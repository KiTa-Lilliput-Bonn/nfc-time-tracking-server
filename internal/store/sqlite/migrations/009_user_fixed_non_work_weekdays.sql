-- +goose Up
ALTER TABLE users ADD COLUMN fixed_non_work_weekdays TEXT NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE users DROP COLUMN fixed_non_work_weekdays;
