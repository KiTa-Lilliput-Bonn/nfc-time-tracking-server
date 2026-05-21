-- +goose Up
CREATE TABLE user_groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE users ADD COLUMN group_id INTEGER REFERENCES user_groups(id);

CREATE INDEX idx_users_group_id ON users(group_id);

-- +goose Down
SELECT 1;
