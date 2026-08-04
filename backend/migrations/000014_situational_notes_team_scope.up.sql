-- Team-scoped situational notes: some news (QB competitions, scheme installs,
-- coaching changes) is really about a team's context, not one player's own
-- value. Explicit scope lets a teammate's review pull it in without the system
-- guessing which direction/magnitude it applies in — see
-- docs/stats/consensus-ensemble.md.
ALTER TABLE nfl_player_situational_notes
    ADD COLUMN IF NOT EXISTS team TEXT,
    ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT 'player'; -- 'player' | 'team'

CREATE INDEX IF NOT EXISTS idx_situational_notes_team_scope
    ON nfl_player_situational_notes (team, season)
    WHERE scope = 'team';

-- Re-running an import (routine as research gets refreshed through camp) had no
-- dedup key and would silently pile up duplicate rows. Match the upsert pattern
-- already used for nfl_consensus_rankings.
ALTER TABLE nfl_player_situational_notes
    ADD CONSTRAINT nfl_player_situational_notes_dedup
    UNIQUE (player_name_raw, season, category, source, reported_date);
