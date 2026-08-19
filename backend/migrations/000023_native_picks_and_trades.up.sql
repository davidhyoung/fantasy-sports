-- Draft-class size is per-league config: a dynasty league runs a short
-- rookie draft (3-5 rounds), a redraft league regenerating its whole draft
-- at rollover needs many more. Same shape as taxi_slots/ir_slots — a simple
-- per-league int, no reason for a second settings table.
ALTER TABLE league_settings ADD COLUMN IF NOT EXISTS draft_rounds INT NOT NULL DEFAULT 4;

-- Tradable future draft picks. original_team_id and current_team_id start
-- equal; a trade only ever moves current_team_id. Native leagues track no
-- standings (no weekly scoring exists yet), so overall_pick stays NULL until
-- something else sets a draft order — picks are otherwise unordered within a
-- round.
CREATE TABLE IF NOT EXISTS league_draft_picks (
    id               BIGSERIAL PRIMARY KEY,
    league_id        BIGINT NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    season           INT    NOT NULL,
    round            INT    NOT NULL CHECK (round > 0),
    original_team_id BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    current_team_id  BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    overall_pick     INT,
    used_on_gsis_id  TEXT REFERENCES nfl_players(gsis_id),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (league_id, season, round, original_team_id)
);
CREATE INDEX IF NOT EXISTS idx_league_draft_picks_current_team ON league_draft_picks (current_team_id);
CREATE INDEX IF NOT EXISTS idx_league_draft_picks_league_season ON league_draft_picks (league_id, season);

-- Append-only history. league_rosters/league_contracts/league_draft_picks
-- hold current state; this holds why it changed. created_by records the
-- acting user even though today that's always the single commissioner — a
-- future multi-user accept-flow can layer on without a schema change.
CREATE TABLE IF NOT EXISTS league_transactions (
    id         BIGSERIAL PRIMARY KEY,
    league_id  BIGINT NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    season     INT    NOT NULL,
    kind       TEXT   NOT NULL
        CHECK (kind IN ('draft','auction','trade','add','drop','keeper','rollover')),
    payload    JSONB  NOT NULL,
    created_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_league_transactions_league_season ON league_transactions (league_id, season);
