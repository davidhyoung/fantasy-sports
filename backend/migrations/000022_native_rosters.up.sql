-- A native league's actual roster state: who's on which team, and what they
-- signed for. Yahoo leagues get this from Yahoo; a native league has no other
-- source, so this table is authoritative the moment a player is assigned.
CREATE TABLE IF NOT EXISTS league_rosters (
    league_id    BIGINT NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    team_id      BIGINT NOT NULL REFERENCES teams(id)   ON DELETE CASCADE,
    gsis_id      TEXT   NOT NULL REFERENCES nfl_players(gsis_id),
    slot         TEXT   NOT NULL DEFAULT 'BN'
        CHECK (slot IN ('QB','RB','WR','TE','K','DEF','FLEX','SFLEX','BN','TAXI','IR')),
    acquired_via TEXT   NOT NULL
        CHECK (acquired_via IN ('draft','auction','trade','waiver','fa','keeper')),
    acquired_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- A player belongs to at most one team per league. The free-agent pool is
    -- derived from this — anyone with no row here — so it's enforced here
    -- rather than trusted to application code.
    PRIMARY KEY (league_id, gsis_id)
);
CREATE INDEX IF NOT EXISTS idx_league_rosters_team ON league_rosters (team_id);

-- What a rostered player signed for. Every roster row gets a contract row —
-- even a $0 undrafted pickup — since cap space is derived as
-- budget - SUM(salary) and that math needs every rostered player accounted for.
CREATE TABLE IF NOT EXISTS league_contracts (
    league_id      BIGINT NOT NULL,
    gsis_id        TEXT   NOT NULL,
    salary         INT    NOT NULL DEFAULT 0 CHECK (salary >= 0),
    signed_season  INT    NOT NULL,
    years_total    INT,   -- NULL = year-to-year, escalated by keeper_rules at rollover
    years_used     INT    NOT NULL DEFAULT 1,
    PRIMARY KEY (league_id, gsis_id),
    FOREIGN KEY (league_id, gsis_id)
        REFERENCES league_rosters (league_id, gsis_id) ON DELETE CASCADE
);
