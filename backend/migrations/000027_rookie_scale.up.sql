-- Rookie contract scale: a drafted player's salary/length is derived from
-- his draft slot (see internal/handlers/rookie_scale.go), not typed in by
-- the commissioner. JSONB rather than flat columns, same reasoning as
-- slots/scoring — the shape (a years-by-round map) isn't a fixed set of
-- columns and the whole blob is always read/written together. Empty '{}' is
-- a real, valid value meaning "use the hardcoded defaults" — see
-- rookieScalePctRange/rookieScaleYears, which treat a zero/missing field as
-- unset rather than requiring every league to configure this explicitly.
ALTER TABLE league_settings ADD COLUMN IF NOT EXISTS rookie_scale JSONB NOT NULL DEFAULT '{}'::jsonb;
