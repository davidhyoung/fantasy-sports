-- A native NBA league's roster state — same role as league_rosters, parallel
-- rather than unified since the player identity (espn_id vs gsis_id) and
-- slot vocabulary differ entirely. No contracts/cap table this phase
-- (deferred, see .claude/plans — a redraft-style assign/drop roster is
-- enough for category scoring to mean something).
--
-- Slot vocabulary is only G/F/C, not the more granular PG/SG/SF/PF — ESPN's
-- own NBA position taxonomy (the sole data source here) only ever reports
-- Guard/Forward/Center, verified against real roster data; there is no
-- point-guard-vs-shooting-guard signal to key a narrower slot off of.
CREATE TABLE IF NOT EXISTS league_rosters_nba (
    league_id    BIGINT NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    team_id      BIGINT NOT NULL REFERENCES teams(id)   ON DELETE CASCADE,
    espn_id      TEXT   NOT NULL REFERENCES nba_players(espn_id),
    slot         TEXT   NOT NULL DEFAULT 'BN'
        CHECK (slot IN ('G','F','C','UTIL','BN','IR')),
    acquired_via TEXT   NOT NULL
        CHECK (acquired_via IN ('draft','trade','waiver','fa')),
    acquired_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (league_id, espn_id)
);
CREATE INDEX IF NOT EXISTS idx_league_rosters_nba_team ON league_rosters_nba (team_id);
