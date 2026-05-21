-- +goose Up
CREATE TABLE api_pairing_sessions (
    token_hash TEXT PRIMARY KEY,
    client_id TEXT NOT NULL,
    expires_at_utc TEXT NOT NULL,
    consumed_at_utc TEXT,
    created_at_utc TEXT NOT NULL
);

CREATE INDEX idx_api_pairing_sessions_client_id ON api_pairing_sessions(client_id);

-- +goose Down
DROP TABLE IF EXISTS api_pairing_sessions;
