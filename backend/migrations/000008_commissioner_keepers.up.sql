-- Track whether a synced user is the commissioner of their team's league.
ALTER TABLE teams ADD COLUMN IF NOT EXISTS is_commissioner BOOLEAN NOT NULL DEFAULT FALSE;

-- Record when a team owner explicitly submits their keeper selections.
CREATE TABLE IF NOT EXISTS keeper_submissions (
    team_id           BIGINT PRIMARY KEY REFERENCES teams(id) ON DELETE CASCADE,
    submitted_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    submitter_user_id BIGINT REFERENCES users(id)
);
