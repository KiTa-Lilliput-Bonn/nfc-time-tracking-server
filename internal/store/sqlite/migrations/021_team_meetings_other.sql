-- +goose Up
-- +goose NO TRANSACTION
PRAGMA foreign_keys=OFF;

CREATE TABLE team_meetings_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    iso_week_year INTEGER NOT NULL,
    iso_week INTEGER NOT NULL,
    meeting_date TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('kt', 'gt', 'other')),
    label TEXT NOT NULL DEFAULT '',
    time_start TEXT NOT NULL,
    time_end TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT 'excel' CHECK (source IN ('excel', 'manual')),
    section_index INTEGER NOT NULL DEFAULT 0,
    UNIQUE (iso_week_year, iso_week, kind, section_index, source)
);

INSERT INTO team_meetings_new (
    id, iso_week_year, iso_week, meeting_date, kind, label,
    time_start, time_end, source, section_index
)
SELECT
    id, iso_week_year, iso_week, meeting_date, kind, '',
    time_start, time_end, source, section_index
FROM team_meetings;

DROP TABLE team_meetings;

ALTER TABLE team_meetings_new RENAME TO team_meetings;

CREATE INDEX IF NOT EXISTS idx_team_meetings_week ON team_meetings(iso_week_year, iso_week);

DELETE FROM sqlite_sequence WHERE name = 'team_meetings';
INSERT INTO sqlite_sequence (name, seq)
SELECT 'team_meetings', COALESCE((SELECT MAX(id) FROM team_meetings), 0);

PRAGMA foreign_keys=ON;

-- +goose Down
-- +goose NO TRANSACTION
PRAGMA foreign_keys=OFF;

CREATE TABLE team_meetings_old (
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

INSERT INTO team_meetings_old (
    id, iso_week_year, iso_week, meeting_date, kind,
    time_start, time_end, source, section_index
)
SELECT
    id, iso_week_year, iso_week, meeting_date, kind,
    time_start, time_end, source, section_index
FROM team_meetings
WHERE kind IN ('kt', 'gt');

DROP TABLE team_meetings;

ALTER TABLE team_meetings_old RENAME TO team_meetings;

CREATE INDEX IF NOT EXISTS idx_team_meetings_week ON team_meetings(iso_week_year, iso_week);

DELETE FROM sqlite_sequence WHERE name = 'team_meetings';
INSERT INTO sqlite_sequence (name, seq)
SELECT 'team_meetings', COALESCE((SELECT MAX(id) FROM team_meetings), 0);

PRAGMA foreign_keys=ON;
