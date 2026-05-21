-- +goose Up
ALTER TABLE users ADD COLUMN opening_hours_balance REAL NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN opening_vacation_days REAL NOT NULL DEFAULT 0;

-- +goose Down
-- DROP COLUMN not portable on older SQLite; new installs only — reverse via backup if needed
SELECT 1;
