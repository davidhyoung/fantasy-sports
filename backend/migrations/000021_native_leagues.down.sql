DROP TABLE IF EXISTS league_settings;

ALTER TABLE leagues
    DROP COLUMN IF EXISTS source,
    DROP COLUMN IF EXISTS format;
