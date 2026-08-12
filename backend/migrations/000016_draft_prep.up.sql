-- Personal draft-prep board: your own tags and ranking for an upcoming draft.
-- Scoped to (user, league, season) so the same player can be a target in one
-- league and an avoid in another, and last year's board doesn't bleed into this
-- one. Rows exist only for players you've marked — an untouched player has none.
CREATE TABLE IF NOT EXISTS draft_prep_players (
    user_id     BIGINT NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
    league_id   BIGINT NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    season      INT    NOT NULL,
    gsis_id     TEXT   NOT NULL,
    -- 'target' | 'sleeper' | 'avoid'; NULL when only a rank or note is set.
    tag         TEXT,
    -- Position on your personal board. NULL = unranked, falls back to projection order.
    custom_rank INT,
    note        TEXT   NOT NULL DEFAULT '',
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, league_id, season, gsis_id),
    CONSTRAINT draft_prep_tag_valid CHECK (tag IS NULL OR tag IN ('target', 'sleeper', 'avoid'))
);

-- The board is always read whole, for one user/league/season at a time.
CREATE INDEX IF NOT EXISTS idx_draft_prep_board
    ON draft_prep_players (user_id, league_id, season);
