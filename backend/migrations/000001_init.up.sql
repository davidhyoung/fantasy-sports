CREATE TABLE IF NOT EXISTS leagues (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    sport      TEXT NOT NULL,
    season     TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS teams (
    id        BIGSERIAL PRIMARY KEY,
    league_id BIGINT NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    name      TEXT NOT NULL,
    owner_id  BIGINT
);

CREATE TABLE IF NOT EXISTS players (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    sport       TEXT NOT NULL,
    position    TEXT NOT NULL,
    external_id TEXT
);

CREATE TABLE IF NOT EXISTS rosters (
    team_id   BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    player_id BIGINT NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    slot      TEXT NOT NULL,
    PRIMARY KEY (team_id, player_id)
);
