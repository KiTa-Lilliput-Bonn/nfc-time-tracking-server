-- +goose Up
-- +goose NO TRANSACTION

CREATE TABLE compensation_day_claims (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    work_date DATE NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('open', 'used', 'waived')),
    used_absence_id INTEGER REFERENCES absences(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, work_date)
);

PRAGMA foreign_keys=OFF;

CREATE TABLE absences_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    absence_date DATE NOT NULL,
    absence_type TEXT NOT NULL CHECK(absence_type IN ('sick', 'vacation', 'other', 'compensation_day')),
    half_day BOOLEAN NOT NULL DEFAULT 0,
    created_by INTEGER NOT NULL REFERENCES users(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, absence_date)
);

INSERT INTO absences_new (id, user_id, absence_date, absence_type, half_day, created_by, created_at)
SELECT id, user_id, absence_date, absence_type, half_day, created_by, created_at
FROM absences;

DROP TABLE absences;
ALTER TABLE absences_new RENAME TO absences;

PRAGMA foreign_keys=ON;

CREATE INDEX idx_compensation_day_claims_user_status
    ON compensation_day_claims(user_id, status, work_date);
CREATE INDEX idx_compensation_day_claims_used_absence
    ON compensation_day_claims(used_absence_id);

-- +goose Down
-- +goose NO TRANSACTION

PRAGMA foreign_keys=OFF;

DROP TABLE IF EXISTS compensation_day_claims;

CREATE TABLE absences_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    absence_date DATE NOT NULL,
    absence_type TEXT NOT NULL CHECK(absence_type IN ('sick', 'vacation', 'other')),
    half_day BOOLEAN NOT NULL DEFAULT 0,
    created_by INTEGER NOT NULL REFERENCES users(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, absence_date)
);

INSERT INTO absences_old (id, user_id, absence_date, absence_type, half_day, created_by, created_at)
SELECT id, user_id, absence_date, absence_type, half_day, created_by, created_at
FROM absences
WHERE absence_type IN ('sick', 'vacation', 'other');

DROP TABLE absences;
ALTER TABLE absences_old RENAME TO absences;

PRAGMA foreign_keys=ON;
