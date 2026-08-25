-- Cap discounting for stashed players: a taxi-squad or injured-reserve
-- player isn't part of a team's active competitive roster, so their salary
-- counts less against the hard cap than an active roster spot's would.
-- Plain columns rather than a settings blob, same reasoning as
-- taxi_slots/ir_slots being flat ints.
ALTER TABLE league_settings
    ADD COLUMN IF NOT EXISTS taxi_cap_pct DOUBLE PRECISION NOT NULL DEFAULT 0.25 CHECK (taxi_cap_pct >= 0 AND taxi_cap_pct <= 1),
    ADD COLUMN IF NOT EXISTS ir_cap_pct   DOUBLE PRECISION NOT NULL DEFAULT 0.50 CHECK (ir_cap_pct >= 0 AND ir_cap_pct <= 1);

-- Per-team, per-season cap ledger. A team's cap for a season is
-- base_budget + banked, where banked is whatever the team finished the
-- previous season under cap by — frozen here at rollover (Phase 8) rather
-- than recomputed from history on every read. A team with no row yet (its
-- league's first season, or any season before Phase 8's rollover
-- integration writes one) falls back to league_settings.budget with
-- banked = 0 — see teamCap in internal/handlers/cap.go.
CREATE TABLE IF NOT EXISTS league_team_seasons (
    league_id    BIGINT NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    team_id      BIGINT NOT NULL REFERENCES teams(id)   ON DELETE CASCADE,
    season       INT    NOT NULL,
    base_budget  INT    NOT NULL,
    banked       INT    NOT NULL DEFAULT 0,
    PRIMARY KEY (league_id, team_id, season)
);

-- Cap hits that outlive the contract they came from. A cut player's roster
-- and contract rows are deleted (league_rosters cascades league_contracts
-- via FK), but under the full-dead-cap rule the salary is still owed for
-- every season left on the deal — this table is what survives that cascade.
-- source_gsis_id is descriptive only, no FK — the whole point of this table
-- is that it outlives the league_rosters row the player came from.
CREATE TABLE IF NOT EXISTS league_dead_money (
    id             BIGSERIAL PRIMARY KEY,
    league_id      BIGINT NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    team_id        BIGINT NOT NULL REFERENCES teams(id)   ON DELETE CASCADE,
    season         INT    NOT NULL,
    amount         INT    NOT NULL CHECK (amount >= 0),
    source_gsis_id TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_league_dead_money_team_season ON league_dead_money (league_id, team_id, season);
