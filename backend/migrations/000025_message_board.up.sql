-- League message board (native leagues only). Author is a team, not a user —
-- this app has no concept of individual managers beyond team ownership, so
-- every other native-league surface (Standings, Roster, Trade) already
-- displays team identity, not a separate person identity. Threads are posts
-- with parent_id NULL; replies point at their thread's root post_id
-- directly (one level of nesting only, enforced in the handler, not here).
CREATE TABLE IF NOT EXISTS league_posts (
    id                  BIGSERIAL PRIMARY KEY,
    league_id           BIGINT NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    parent_id           BIGINT REFERENCES league_posts(id) ON DELETE CASCADE,
    author_team_id      BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    title               TEXT,    -- set for threads (parent_id IS NULL), null for replies
    body                TEXT NOT NULL,
    image_url           TEXT,
    attached_gsis_id    TEXT REFERENCES nfl_players(gsis_id),
    is_pinned           BOOLEAN NOT NULL DEFAULT false,
    is_deleted          BOOLEAN NOT NULL DEFAULT false,
    edited_at           TIMESTAMPTZ,
    -- Poll fields — only meaningful when parent_id IS NULL and options exist.
    poll_closes_at      TIMESTAMPTZ,
    poll_votes_visible  TEXT NOT NULL DEFAULT 'after_voting' CHECK (poll_votes_visible IN ('after_voting', 'always')),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_league_posts_league_parent ON league_posts (league_id, parent_id, created_at DESC);
-- At most one pinned thread per league — a partial unique index on the
-- league is what actually enforces "maximum one pinned thread," not
-- application logic alone.
CREATE UNIQUE INDEX IF NOT EXISTS idx_league_posts_one_pin
    ON league_posts (league_id) WHERE is_pinned AND parent_id IS NULL;

CREATE TABLE IF NOT EXISTS league_post_reactions (
    post_id    BIGINT NOT NULL REFERENCES league_posts(id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reaction   TEXT NOT NULL DEFAULT '+1',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (post_id, user_id, reaction)
);

CREATE TABLE IF NOT EXISTS league_poll_options (
    id       BIGSERIAL PRIMARY KEY,
    post_id  BIGINT NOT NULL REFERENCES league_posts(id) ON DELETE CASCADE,
    label    TEXT NOT NULL,
    position INT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_league_poll_options_post ON league_poll_options (post_id, position);

CREATE TABLE IF NOT EXISTS league_poll_votes (
    post_id    BIGINT NOT NULL REFERENCES league_posts(id) ON DELETE CASCADE,
    option_id  BIGINT NOT NULL REFERENCES league_poll_options(id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- One vote per user per poll (not per option) — the primary key is on
    -- (post_id, user_id) precisely so a second vote must overwrite the
    -- first rather than add a second row.
    PRIMARY KEY (post_id, user_id)
);

-- Per-user last-read-at on a thread — meaningful even for the single
-- signed-in user this app has today (same as unread email), and what the
-- unread dot / tab count / Unread filter all read from. Deliberately not
-- derived from a client timestamp, which lies across devices/tabs.
CREATE TABLE IF NOT EXISTS league_thread_reads (
    thread_id     BIGINT NOT NULL REFERENCES league_posts(id) ON DELETE CASCADE,
    user_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    last_read_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (thread_id, user_id)
);
