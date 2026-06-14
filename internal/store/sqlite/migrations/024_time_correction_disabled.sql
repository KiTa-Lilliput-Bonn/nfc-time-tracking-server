-- +goose Up
ALTER TABLE time_corrections ADD COLUMN disabled INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite cannot DROP COLUMN in older versions; recreate would be destructive.
-- No down migration for additive column.
