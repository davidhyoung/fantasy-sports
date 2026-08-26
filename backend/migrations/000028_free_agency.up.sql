-- Reservation floor for free-agent offers: a fraction of the league's own
-- real auction value for that player (GetDraftValues' board), below which no
-- offer signs him regardless of whether it's the only one. Same reasoning as
-- taxi_cap_pct/ir_cap_pct — a plain column, not a nested blob, since it's one
-- scalar knob.
ALTER TABLE league_settings ADD COLUMN IF NOT EXISTS fa_reservation_pct DOUBLE PRECISION NOT NULL DEFAULT 0.5 CHECK (fa_reservation_pct >= 0 AND fa_reservation_pct <= 1);

-- A free-agency offer window: teams submit/edit offers while it's open,
-- the commissioner resolves it once (an explicit action, never a cron —
-- matching ScoreLeagueWeek, since nothing in this app's data pipeline is
-- genuinely live). kind is descriptive only (offseason vs. in-season
-- churn) — the resolution mechanism is identical either way. Only one
-- window may be open at a time per league: the partial unique index below
-- is what enforces that, not application logic alone.
CREATE TABLE IF NOT EXISTS league_fa_windows (
    id          BIGSERIAL PRIMARY KEY,
    league_id   BIGINT NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    season      INT    NOT NULL,
    kind        TEXT   NOT NULL CHECK (kind IN ('offseason', 'weekly')),
    week        INT,
    opened_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_league_fa_windows_one_open
    ON league_fa_windows (league_id) WHERE resolved_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_league_fa_windows_league_season
    ON league_fa_windows (league_id, season);

-- One team's offer on one player within one window. priority is that
-- team's own ranking of this offer against their other pending offers in
-- the same window (1 = most wanted) — resolution reserves cap for a team's
-- better-priority pending offers before considering a worse-priority one,
-- so a team's stated preference survives even when the market doesn't
-- resolve players in that team's preferred order. status starts 'pending'
-- and is written once, at resolution, to 'won' or 'lost' — offers are
-- otherwise mutable (re-POST updates salary/years/priority) or removable
-- (withdraw) up until then.
CREATE TABLE IF NOT EXISTS league_fa_offers (
    id         BIGSERIAL PRIMARY KEY,
    league_id  BIGINT NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    window_id  BIGINT NOT NULL REFERENCES league_fa_windows(id) ON DELETE CASCADE,
    gsis_id    TEXT   NOT NULL REFERENCES nfl_players(gsis_id),
    team_id    BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    salary     INT    NOT NULL CHECK (salary >= 0),
    years      INT    NOT NULL CHECK (years >= 1),
    priority   INT    NOT NULL DEFAULT 1,
    status     TEXT   NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'won', 'lost')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- One offer per team per player per window — re-offering on the same
    -- player updates this row rather than stacking a second one.
    UNIQUE (window_id, gsis_id, team_id)
);
CREATE INDEX IF NOT EXISTS idx_league_fa_offers_window ON league_fa_offers (window_id);
CREATE INDEX IF NOT EXISTS idx_league_fa_offers_team ON league_fa_offers (window_id, team_id);

-- A free-agent signing is its own transaction kind, distinct from 'add'
-- (which covers the plain, uncontested assignLeagueRoster path).
ALTER TABLE league_transactions DROP CONSTRAINT league_transactions_kind_check;
ALTER TABLE league_transactions ADD CONSTRAINT league_transactions_kind_check
    CHECK (kind IN ('draft', 'auction', 'trade', 'add', 'drop', 'keeper', 'rollover', 'sign'));
