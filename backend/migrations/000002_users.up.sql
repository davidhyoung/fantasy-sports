CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    yahoo_guid    TEXT        NOT NULL UNIQUE,
    display_name  TEXT        NOT NULL,
    email         TEXT,
    access_token  TEXT        NOT NULL,
    refresh_token TEXT        NOT NULL,
    token_expiry  TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
