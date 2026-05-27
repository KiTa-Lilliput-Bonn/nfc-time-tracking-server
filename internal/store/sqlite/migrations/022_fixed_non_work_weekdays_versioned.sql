-- +goose Up
CREATE TABLE fixed_non_work_weekdays (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    weekdays TEXT NOT NULL DEFAULT '[]',
    valid_from DATE NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:00',
    UNIQUE(user_id, valid_from)
);

INSERT INTO fixed_non_work_weekdays (user_id, weekdays, valid_from, created_at)
SELECT
    u.id,
    u.fixed_non_work_weekdays,
    COALESCE(
        (SELECT MIN(valid_from) FROM weekly_hours wh WHERE wh.user_id = u.id),
        date(u.created_at),
        '2000-01-01'
    ),
    datetime('now')
FROM users u;

ALTER TABLE users DROP COLUMN fixed_non_work_weekdays;

-- +goose Down
ALTER TABLE users ADD COLUMN fixed_non_work_weekdays TEXT NOT NULL DEFAULT '[]';

UPDATE users SET fixed_non_work_weekdays = COALESCE(
    (SELECT f.weekdays FROM fixed_non_work_weekdays f
     WHERE f.user_id = users.id
     ORDER BY f.valid_from DESC LIMIT 1),
    '[]'
);

DROP TABLE fixed_non_work_weekdays;
