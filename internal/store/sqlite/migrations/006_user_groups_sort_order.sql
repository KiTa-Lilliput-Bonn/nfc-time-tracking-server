-- +goose Up
ALTER TABLE user_groups ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;

UPDATE user_groups SET sort_order = id WHERE id IS NOT NULL;

-- +goose Down
SELECT 1;
