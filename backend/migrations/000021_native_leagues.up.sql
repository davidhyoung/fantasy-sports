-- A native league has no Yahoo league behind it: no yahoo_key, no sync, no
-- live scoring/standings today. source distinguishes "where does fantasy
-- context come from" (see internal/services/leaguesettings); format selects
-- the season-rollover strategy once that lands (dynasty/keeper/redraft carry
-- roster state forward differently).
ALTER TABLE leagues
    ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'yahoo'
        CHECK (source IN ('yahoo', 'native')),
    ADD COLUMN IF NOT EXISTS format TEXT NOT NULL DEFAULT 'redraft'
        CHECK (format IN ('redraft', 'keeper', 'dynasty'));

-- Rows created by the old public CreateLeague endpoint have no yahoo_key and
-- no Yahoo league behind them — they're native by definition, not merely
-- unsynced Yahoo leagues.
UPDATE leagues SET source = 'native' WHERE yahoo_key IS NULL;

-- Canonical league settings, in the same vocabulary the draft-values
-- ?slots=&scoring= override params already use (leaguesettings.SlotToPosition /
-- ScoringEditableStats). slots/scoring are JSONB rather than columns because
-- that vocabulary is already validated in Go, the blob is always read and
-- written whole, and adding a slot type shouldn't need a migration.
CREATE TABLE IF NOT EXISTS league_settings (
    league_id  BIGINT PRIMARY KEY REFERENCES leagues(id) ON DELETE CASCADE,
    num_teams  INT    NOT NULL,
    budget     INT    NOT NULL DEFAULT 200,
    slots      JSONB  NOT NULL,
    scoring    JSONB  NOT NULL,
    taxi_slots INT    NOT NULL DEFAULT 0,
    ir_slots   INT    NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
