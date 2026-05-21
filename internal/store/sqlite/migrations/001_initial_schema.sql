-- +goose Up
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name TEXT NOT NULL,
    role TEXT NOT NULL CHECK(role IN ('user', 'leitung', 'superadmin')),
    active BOOLEAN NOT NULL DEFAULT 1,
    must_change_password BOOLEAN NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE nfc_tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tag_uid TEXT NOT NULL,
    user_id INTEGER NOT NULL REFERENCES users(id),
    assigned_from DATE NOT NULL,
    assigned_until DATE,
    UNIQUE(tag_uid, assigned_from)
);

CREATE TABLE raw_punches (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    punch_time TIMESTAMP NOT NULL,
    nfc_tag_uid TEXT NOT NULL,
    source_file TEXT NOT NULL,
    device_name TEXT NOT NULL DEFAULT '',
    imported_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(punch_time, nfc_tag_uid)
);

CREATE TABLE work_periods (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    work_date DATE NOT NULL,
    punch_in TIMESTAMP NOT NULL,
    punch_out TIMESTAMP,
    is_break BOOLEAN NOT NULL DEFAULT 0,
    source TEXT NOT NULL DEFAULT 'imported' CHECK(source IN ('imported', 'manual'))
);
CREATE INDEX idx_work_periods_user_date ON work_periods(user_id, work_date);

CREATE TABLE time_corrections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    work_period_id INTEGER NOT NULL REFERENCES work_periods(id),
    corrected_in TIMESTAMP NOT NULL,
    corrected_out TIMESTAMP NOT NULL,
    reason TEXT NOT NULL,
    corrected_by INTEGER NOT NULL REFERENCES users(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE weekly_hours (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    hours_per_week REAL NOT NULL,
    valid_from DATE NOT NULL,
    valid_until DATE
);
CREATE INDEX idx_weekly_hours_user ON weekly_hours(user_id);

CREATE TABLE vacation_entitlements (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    days_per_year REAL NOT NULL,
    valid_from DATE NOT NULL,
    valid_until DATE
);
CREATE INDEX idx_vacation_ent_user ON vacation_entitlements(user_id);

CREATE TABLE schedules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    schedule_date DATE NOT NULL,
    shift_start TEXT NOT NULL,
    shift_end TEXT NOT NULL,
    UNIQUE(user_id, schedule_date)
);

CREATE TABLE absences (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    absence_date DATE NOT NULL,
    absence_type TEXT NOT NULL CHECK(absence_type IN ('sick', 'vacation', 'other')),
    half_day BOOLEAN NOT NULL DEFAULT 0,
    created_by INTEGER NOT NULL REFERENCES users(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, absence_date)
);

CREATE TABLE holidays (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    holiday_date DATE NOT NULL UNIQUE,
    name TEXT NOT NULL,
    auto_generated BOOLEAN NOT NULL DEFAULT 0
);

CREATE TABLE closure_days (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    closure_date DATE NOT NULL UNIQUE,
    name TEXT NOT NULL,
    created_by INTEGER NOT NULL REFERENCES users(id)
);

CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Seed default settings
INSERT INTO settings (key, value) VALUES ('rounding_minutes', '15');
INSERT INTO settings (key, value) VALUES ('break_rules', '[{"min_work_hours":6.0,"break_minutes":30},{"min_work_hours":9.0,"break_minutes":45}]');
INSERT INTO settings (key, value) VALUES ('csv_delimiter', ';');

-- +goose Down
DROP TABLE IF EXISTS settings;
DROP TABLE IF EXISTS closure_days;
DROP TABLE IF EXISTS holidays;
DROP TABLE IF EXISTS absences;
DROP TABLE IF EXISTS schedules;
DROP TABLE IF EXISTS vacation_entitlements;
DROP TABLE IF EXISTS weekly_hours;
DROP TABLE IF EXISTS time_corrections;
DROP TABLE IF EXISTS work_periods;
DROP TABLE IF EXISTS raw_punches;
DROP TABLE IF EXISTS nfc_tags;
DROP TABLE IF EXISTS users;
