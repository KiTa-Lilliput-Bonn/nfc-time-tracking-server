-- +goose Up
CREATE TABLE user_schedule_bound (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    schedule_bound INTEGER NOT NULL,
    valid_from DATE NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT (datetime('now')),
    UNIQUE(user_id, valid_from)
);

-- +goose Down
DROP TABLE user_schedule_bound;
