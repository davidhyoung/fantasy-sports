-- Distinguishes a my_value the user typed themselves from one the client
-- auto-filled by interpolating neighbours on a board/tier move, so a move
-- never silently overwrites a number the user set on purpose. NULL whenever
-- my_value itself is NULL — the source of no opinion isn't meaningful.
ALTER TABLE draft_prep_players
    ADD COLUMN IF NOT EXISTS my_value_source TEXT
    CHECK (my_value_source IS NULL OR my_value_source IN ('user', 'derived'));
