// cmd/import-nba syncs NBA player metadata and real game stats from ESPN's
// unofficial site.api.espn.com endpoints (internal/services/espnnba) into
// nba_players / nba_player_stats — the NBA analog of cmd/import, following
// the same manual-batch-import philosophy (explicit, re-runnable, never
// live) since there is no nflverse-equivalent bulk NBA source to pull from
// instead. See migrations/000033_nba_players.up.sql.
//
// Usage:
//
//	go run ./cmd/import-nba -rosters                              # sync nba_players from all 30 teams
//	go run ./cmd/import-nba -stats -from 2026-10-01 -to 2026-10-10 # backfill real game stats for a date range
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/davidyoung/fantasy-sports/backend/internal/services/espnnba"
)

func main() {
	doRosters := flag.Bool("rosters", false, "sync nba_players from ESPN team rosters")
	doStats := flag.Bool("stats", false, "backfill nba_player_stats for completed games in the date range")
	from := flag.String("from", "", "first date to import, YYYY-MM-DD (stats only)")
	to := flag.String("to", "", "last date to import, YYYY-MM-DD (stats only)")
	flag.Parse()

	_ = godotenv.Load()
	_ = godotenv.Load("../.env")

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	if *doRosters {
		log.Println("importing NBA rosters…")
		if err := importRosters(ctx, pool); err != nil {
			log.Fatalf("roster import: %v", err)
		}
	}

	if *doStats {
		if *from == "" || *to == "" {
			log.Fatal("-stats requires -from and -to (YYYY-MM-DD)")
		}
		fromDate, err := time.Parse("2006-01-02", *from)
		if err != nil {
			log.Fatalf("invalid -from: %v", err)
		}
		toDate, err := time.Parse("2006-01-02", *to)
		if err != nil {
			log.Fatalf("invalid -to: %v", err)
		}
		if err := importStats(ctx, pool, fromDate, toDate); err != nil {
			log.Fatalf("stats import: %v", err)
		}
	}

	if !*doRosters && !*doStats {
		log.Fatal("nothing to do — pass -rosters and/or -stats")
	}
	log.Println("done")
}

// nbaSeason reports the season a game date belongs to, using the standard
// NBA convention of naming a season by its starting year (a 2027-01-15 game
// is still part of the "2026" season — Oct-Dec games start a season, Jan-Jun
// games finish the one that started the previous fall).
func nbaSeason(d time.Time) int {
	if d.Month() >= time.October {
		return d.Year()
	}
	return d.Year() - 1
}

func importRosters(ctx context.Context, pool *pgxpool.Pool) error {
	var imported, failed int
	for teamID := 1; teamID <= espnnba.NumTeams; teamID++ {
		roster, err := espnnba.FetchRoster(ctx, teamID)
		if err != nil {
			log.Printf("  team %d: %v", teamID, err)
			failed++
			continue
		}
		for _, a := range roster.Athletes {
			var dob *time.Time
			if a.DateOfBirth != "" {
				if t, err := time.Parse(time.RFC3339, a.DateOfBirth); err == nil {
					dob = &t
				}
			}
			_, err := pool.Exec(ctx, `
				INSERT INTO nba_players
					(espn_id, name, position, team, birth_date, height, weight, college, years_exp, jersey_number, headshot_url, updated_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NOW())
				ON CONFLICT (espn_id) DO UPDATE SET
					name = EXCLUDED.name, position = EXCLUDED.position, team = EXCLUDED.team,
					birth_date = EXCLUDED.birth_date, height = EXCLUDED.height, weight = EXCLUDED.weight,
					college = EXCLUDED.college, years_exp = EXCLUDED.years_exp,
					jersey_number = EXCLUDED.jersey_number, headshot_url = EXCLUDED.headshot_url,
					updated_at = NOW()
			`, a.ID, a.DisplayName, a.Position.Abbreviation, roster.Team.Abbreviation, dob,
				nullableInt(a.Height), nullableInt(a.Weight), a.College.Name, a.Experience.Years,
				nullableJersey(a.Jersey), a.Headshot.Href)
			if err != nil {
				log.Printf("  player %s (%s): %v", a.DisplayName, a.ID, err)
				failed++
				continue
			}
			imported++
		}
	}
	log.Printf("  imported %d players, %d failed", imported, failed)
	return nil
}

func nullableInt(f float64) *int {
	if f == 0 {
		return nil
	}
	v := int(f)
	return &v
}

func nullableJersey(s string) *int {
	if s == "" {
		return nil
	}
	v := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return nil
		}
		v = v*10 + int(c-'0')
	}
	return &v
}

func importStats(ctx context.Context, pool *pgxpool.Pool, from, to time.Time) error {
	var gamesSeen, rowsImported, failed int
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("20060102")
		events, err := espnnba.FetchScoreboard(ctx, dateStr)
		if err != nil {
			log.Printf("  scoreboard %s: %v", dateStr, err)
			continue
		}
		for _, ev := range events {
			if ev.State() != "post" {
				continue // only final box scores are real, countable stats
			}
			gamesSeen++
			teams, err := espnnba.FetchBoxscore(ctx, ev.ID)
			if err != nil {
				log.Printf("  boxscore %s: %v", ev.ID, err)
				failed++
				continue
			}
			if len(teams) != 2 {
				continue
			}
			for i, team := range teams {
				opponent := teams[1-i].Team.Abbreviation
				if len(team.Statistics) == 0 {
					continue
				}
				group := team.Statistics[0]
				for _, a := range group.Athletes {
					if a.Athlete.ID == "" {
						continue
					}
					// A box score can include players not yet on a team's
					// tracked roster (preseason squads run larger than the
					// 15-man regular-season roster -rosters syncs) — stub a
					// minimal row rather than dropping their stats on the
					// FK constraint; a later -rosters run fills in the rest.
					if _, err := pool.Exec(ctx, `
						INSERT INTO nba_players (espn_id, name, team, updated_at)
						VALUES ($1, $2, $3, NOW())
						ON CONFLICT (espn_id) DO NOTHING
					`, a.Athlete.ID, a.Athlete.DisplayName, team.Team.Abbreviation); err != nil {
						log.Printf("  stub player %s (%s): %v", a.Athlete.DisplayName, a.Athlete.ID, err)
						failed++
						continue
					}
					s := espnnba.ParseAthleteStats(group.Keys, a.Stats)
					_, err := pool.Exec(ctx, `
						INSERT INTO nba_player_stats
							(espn_id, espn_event_id, game_date, season, team, opponent,
							 minutes, points, rebounds, offensive_rebounds, defensive_rebounds,
							 assists, steals, blocks, turnovers, fouls,
							 fg_made, fg_att, fg3_made, fg3_att, ft_made, ft_att)
						VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
						ON CONFLICT (espn_id, espn_event_id) DO UPDATE SET
							minutes = EXCLUDED.minutes, points = EXCLUDED.points,
							rebounds = EXCLUDED.rebounds, offensive_rebounds = EXCLUDED.offensive_rebounds,
							defensive_rebounds = EXCLUDED.defensive_rebounds, assists = EXCLUDED.assists,
							steals = EXCLUDED.steals, blocks = EXCLUDED.blocks, turnovers = EXCLUDED.turnovers,
							fouls = EXCLUDED.fouls, fg_made = EXCLUDED.fg_made, fg_att = EXCLUDED.fg_att,
							fg3_made = EXCLUDED.fg3_made, fg3_att = EXCLUDED.fg3_att,
							ft_made = EXCLUDED.ft_made, ft_att = EXCLUDED.ft_att
					`, a.Athlete.ID, ev.ID, d, nbaSeason(d), team.Team.Abbreviation, opponent,
						s.Minutes, s.Points, s.Rebounds, s.OffensiveRebounds, s.DefensiveRebounds,
						s.Assists, s.Steals, s.Blocks, s.Turnovers, s.Fouls,
						s.FGMade, s.FGAtt, s.FG3Made, s.FG3Att, s.FTMade, s.FTAtt)
					if err != nil {
						log.Printf("  stat row %s (%s): %v", a.Athlete.DisplayName, a.Athlete.ID, err)
						failed++
						continue
					}
					rowsImported++
				}
			}
		}
	}
	log.Printf("  %d completed games, %d player-game rows imported, %d failed", gamesSeen, rowsImported, failed)
	return nil
}
