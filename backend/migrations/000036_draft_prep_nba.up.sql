-- Personal NBA draft-prep board: your own interest flag and note for a
-- player, scoped to (user, league, season) — same sparse-row convention as
-- draft_prep_players (an untouched player has no row at all). Starts at the
-- "mature" NFL shape directly (binary interest, a note) rather than
-- replaying that table's whole iterative history (tag enum -> 6-level scale
-- -> binary, custom_rank/tier, planned_cost/my_value) — those extra columns
-- exist because NFL draft prep has auction dollars and a tiering system to
-- plan against; native NBA leagues have neither yet, so there's nothing for
-- a cost or tier column to mean here.
CREATE TABLE IF NOT EXISTS draft_prep_players_nba (
    user_id    BIGINT NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
    league_id  BIGINT NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    season     INT    NOT NULL,
    espn_id    TEXT   NOT NULL,
    -- +1 target, -1 avoid; NULL = no opinion. 0 is disallowed so there's
    -- exactly one way to say "no opinion" (NULL, i.e. no row at all).
    interest   SMALLINT CHECK (interest IS NULL OR interest IN (-1, 1)),
    note       TEXT   NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, league_id, season, espn_id)
);

CREATE INDEX IF NOT EXISTS idx_draft_prep_nba_board
    ON draft_prep_players_nba (user_id, league_id, season);
