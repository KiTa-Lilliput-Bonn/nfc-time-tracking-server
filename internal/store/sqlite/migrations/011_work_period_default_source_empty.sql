-- +goose Up
-- +goose NO TRANSACTION
-- Stempel-/Import-Perioden: source leer (Standard), nur manuelle Einträge 'manual'.
-- PRAGMA foreign_keys wirkt in SQLite nicht innerhalb einer Transaktion; ohne NO TRANSACTION schlägt DROP TABLE work_periods fehl (time_corrections FK).
PRAGMA foreign_keys=OFF;

CREATE TABLE work_periods_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    work_date DATE NOT NULL,
    punch_in TIMESTAMP NOT NULL,
    punch_out TIMESTAMP,
    is_break BOOLEAN NOT NULL DEFAULT 0,
    source TEXT NOT NULL DEFAULT '' CHECK(source IN ('', 'manual'))
);

INSERT INTO work_periods_new (id, user_id, work_date, punch_in, punch_out, is_break, source)
SELECT id, user_id, work_date, punch_in, punch_out, is_break,
  CASE WHEN source = 'imported' THEN '' ELSE source END
FROM work_periods;

DROP TABLE work_periods;

ALTER TABLE work_periods_new RENAME TO work_periods;

CREATE INDEX idx_work_periods_user_date ON work_periods(user_id, work_date);

DELETE FROM sqlite_sequence WHERE name = 'work_periods';
INSERT INTO sqlite_sequence (name, seq)
SELECT 'work_periods', COALESCE((SELECT MAX(id) FROM work_periods), 0);

PRAGMA foreign_keys=ON;

-- +goose Down
-- +goose NO TRANSACTION
PRAGMA foreign_keys=OFF;

CREATE TABLE work_periods_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    work_date DATE NOT NULL,
    punch_in TIMESTAMP NOT NULL,
    punch_out TIMESTAMP,
    is_break BOOLEAN NOT NULL DEFAULT 0,
    source TEXT NOT NULL DEFAULT 'imported' CHECK(source IN ('imported', 'manual'))
);

INSERT INTO work_periods_old (id, user_id, work_date, punch_in, punch_out, is_break, source)
SELECT id, user_id, work_date, punch_in, punch_out, is_break,
  CASE WHEN source = '' THEN 'imported' ELSE source END
FROM work_periods;

DROP TABLE work_periods;

ALTER TABLE work_periods_old RENAME TO work_periods;

CREATE INDEX idx_work_periods_user_date ON work_periods(user_id, work_date);

DELETE FROM sqlite_sequence WHERE name = 'work_periods';
INSERT INTO sqlite_sequence (name, seq)
SELECT 'work_periods', COALESCE((SELECT MAX(id) FROM work_periods), 0);

PRAGMA foreign_keys=ON;
