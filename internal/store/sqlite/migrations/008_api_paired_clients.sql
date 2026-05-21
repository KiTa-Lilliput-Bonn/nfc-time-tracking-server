-- +goose Up
CREATE TABLE api_paired_clients (
    id TEXT PRIMARY KEY,
    label TEXT NOT NULL DEFAULT '',
    secret TEXT NOT NULL DEFAULT '',
    created_at_utc TEXT NOT NULL,
    revoked_at_utc TEXT
);

-- +goose Down
DROP TABLE IF EXISTS api_paired_clients;
