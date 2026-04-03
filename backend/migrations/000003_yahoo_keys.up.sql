-- Add Yahoo integration columns to leagues so we can track which Yahoo league
-- each row came from and which user synced it.
ALTER TABLE leagues
    ADD COLUMN yahoo_key TEXT UNIQUE,
    ADD COLUMN user_id   BIGINT REFERENCES users(id);

-- Add Yahoo integration columns to teams.
-- Also add user_id so we know which local user owns each team.
ALTER TABLE teams
    ADD COLUMN yahoo_key TEXT UNIQUE,
    ADD COLUMN user_id   BIGINT REFERENCES users(id);
