-- +goose Up
CREATE TABLE IF NOT EXISTS team_meetings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    iso_week_year INTEGER NOT NULL,
    iso_week INTEGER NOT NULL,
    meeting_date TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('kt', 'gt')),
    time_start TEXT NOT NULL,
    time_end TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT 'excel' CHECK (source IN ('excel', 'manual')),
    section_index INTEGER NOT NULL DEFAULT 0,
    UNIQUE (iso_week_year, iso_week, kind, section_index, source)
);

CREATE TABLE IF NOT EXISTS team_meeting_users (
    team_meeting_id INTEGER NOT NULL REFERENCES team_meetings(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (team_meeting_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_team_meetings_week ON team_meetings(iso_week_year, iso_week);
CREATE INDEX IF NOT EXISTS idx_team_meeting_users_user ON team_meeting_users(user_id);

-- +goose Down
DROP INDEX IF EXISTS idx_team_meeting_users_user;
DROP INDEX IF EXISTS idx_team_meetings_week;
DROP TABLE IF EXISTS team_meeting_users;
DROP TABLE IF EXISTS team_meetings;
