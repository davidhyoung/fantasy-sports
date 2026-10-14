-- Standalone native-NBA keeper designation tool (see .claude/plans and the
-- "Native NBA keepers" entry in CLAUDE.md). Deliberately not wired into
-- rollover.go's dormant `keeper` format — this is a narrower, independent
-- planning tool: pick up to 3 players off last season's roster to carry
-- into the new one, at a cost derived from what they actually cost last
-- season.

-- salary lands directly on league_rosters_nba (already per-season since
-- migration 000037) rather than a separate table — it's a per-(league,
-- season, player) fact at exactly that row's grain. Historical seasons are
-- backfilled via cmd/project-nba -import-nba-salaries; a season's own draft
-- naturally populates it going forward once a real draft/auction flow
-- exists for native NBA (not built yet).
ALTER TABLE league_rosters_nba ADD COLUMN IF NOT EXISTS salary INT;

-- One row per kept player per season — the actual designation. A player can
-- only be kept by one team (UNIQUE), so "The Brian Rule" (another team
-- reclaiming a dropped player) and simple ownership collisions both resolve
-- to a clean 409 at the DB level; the per-team max-of-3 limit is enforced
-- in the handler (a count before insert), not a CHECK constraint, matching
-- how cap.go's budget rules are app-enforced elsewhere in this codebase.
CREATE TABLE IF NOT EXISTS nba_keepers (
    id          BIGSERIAL PRIMARY KEY,
    league_id   BIGINT NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    season      INT NOT NULL,
    team_id     BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    espn_id     TEXT NOT NULL REFERENCES nba_players(espn_id),
    keeper_cost INT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (league_id, season, espn_id)
);

CREATE INDEX idx_nba_keepers_team ON nba_keepers (league_id, season, team_id);
