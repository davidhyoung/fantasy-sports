// Package nflstats loads aggregated season-level NFL stats from nfl_player_stats
// and exposes them in the canonical stat vocabulary used by the ranking and
// scoring layers. This replaces Yahoo as the stats source for NFL rankings.
package nflstats

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/davidyoung/fantasy-sports/backend/internal/services/scoring"
)

// PlayerSeason aggregates a single player's regular-season stats for a season.
type PlayerSeason struct {
	GsisID      string
	Season      int
	GamesPlayed int
	Values      map[scoring.CanonicalStat]float64
}

// LoadSeasonStats aggregates nfl_player_stats → season totals (REG only) for
// the given gsis_ids. Returns a map keyed by gsis_id. Players without any rows
// for the season are absent from the result.
//
// The returned Values map uses canonical stat IDs; FG-by-distance buckets are
// distributed from total fg_made using scoring.FGDistribution.
func LoadSeasonStats(ctx context.Context, db *pgxpool.Pool, season int, gsisIDs []string) (map[string]PlayerSeason, error) {
	out := map[string]PlayerSeason{}
	if len(gsisIDs) == 0 {
		return out, nil
	}

	rows, err := db.Query(ctx, `
		SELECT
			gsis_id,
			COUNT(DISTINCT week) AS games,
			COALESCE(SUM(pass_attempts), 0)        AS pass_att,
			COALESCE(SUM(completions), 0)          AS pass_comp,
			COALESCE(SUM(passing_yards), 0)        AS pass_yds,
			COALESCE(SUM(passing_tds), 0)          AS pass_td,
			COALESCE(SUM(interceptions), 0)        AS pass_int,
			COALESCE(SUM(sacks), 0)                AS sacks,
			COALESCE(SUM(carries), 0)              AS rush_att,
			COALESCE(SUM(rushing_yards), 0)        AS rush_yds,
			COALESCE(SUM(rushing_tds), 0)          AS rush_td,
			COALESCE(SUM(receptions), 0)           AS rec,
			COALESCE(SUM(targets), 0)              AS targets,
			COALESCE(SUM(receiving_yards), 0)      AS rec_yds,
			COALESCE(SUM(receiving_tds), 0)        AS rec_td,
			COALESCE(SUM(special_teams_tds), 0)    AS return_td,
			COALESCE(SUM(passing_2pt + rushing_2pt + receiving_2pt), 0) AS two_pt,
			COALESCE(SUM(rushing_fumbles + receiving_fumbles), 0)       AS fumbles,
			COALESCE(SUM(rushing_fumbles_lost + receiving_fumbles_lost), 0) AS fumbles_lost,
			COALESCE(SUM(fg_made), 0)              AS fg_made,
			COALESCE(SUM(pat_made), 0)             AS pat_made
		FROM nfl_player_stats
		WHERE gsis_id = ANY($1)
		  AND season = $2
		  AND season_type = 'REG'
		GROUP BY gsis_id
	`, gsisIDs, season)
	if err != nil {
		return nil, fmt.Errorf("load season stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			ps       PlayerSeason
			passAtt, passComp, passYds,
			passTD, passInt, sacks,
			rushAtt, rushYds, rushTD,
			rec, targets, recYds, recTD,
			returnTD, twoPt,
			fumbles, fumblesLost,
			fgMade, patMade float64
		)
		if err := rows.Scan(
			&ps.GsisID, &ps.GamesPlayed,
			&passAtt, &passComp, &passYds,
			&passTD, &passInt, &sacks,
			&rushAtt, &rushYds, &rushTD,
			&rec, &targets, &recYds, &recTD,
			&returnTD, &twoPt,
			&fumbles, &fumblesLost,
			&fgMade, &patMade,
		); err != nil {
			return nil, fmt.Errorf("scan season stats: %w", err)
		}
		ps.Season = season
		ps.Values = map[scoring.CanonicalStat]float64{
			scoring.StatPassAtt:     passAtt,
			scoring.StatPassComp:    passComp,
			scoring.StatPassInc:     passAtt - passComp,
			scoring.StatPassYds:     passYds,
			scoring.StatPassTD:      passTD,
			scoring.StatPassInt:     passInt,
			scoring.StatSacks:       sacks,
			scoring.StatRushAtt:     rushAtt,
			scoring.StatRushYds:     rushYds,
			scoring.StatRushTD:      rushTD,
			scoring.StatRec:         rec,
			scoring.StatTargets:     targets,
			scoring.StatRecYds:      recYds,
			scoring.StatRecTD:       recTD,
			scoring.StatReturnTD:    returnTD,
			scoring.StatTwoPt:       twoPt,
			scoring.StatFumbles:     fumbles,
			scoring.StatFumblesLost: fumblesLost,
			scoring.StatFGMade:      fgMade,
			scoring.StatPATMade:     patMade,
		}
		// Distribute fg_made across distance buckets (Yahoo scores by distance).
		for bucket, share := range scoring.FGDistribution {
			ps.Values[bucket] = fgMade * share
		}
		out[ps.GsisID] = ps
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
