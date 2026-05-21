-- +goose Up
CREATE TABLE audit_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at TIMESTAMP NOT NULL,
    actor_user_id INTEGER REFERENCES users(id),
    actor_role TEXT NOT NULL DEFAULT 'system',
    action TEXT NOT NULL CHECK(action IN ('create', 'update', 'delete')),
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL DEFAULT '',
    target_user_id INTEGER REFERENCES users(id),
    summary TEXT NOT NULL DEFAULT '{}',
    prev_hash BLOB NOT NULL,
    event_hash BLOB NOT NULL
);
CREATE INDEX idx_audit_events_created_at ON audit_events(created_at);
CREATE INDEX idx_audit_events_entity ON audit_events(entity_type, entity_id);
CREATE INDEX idx_audit_events_actor ON audit_events(actor_user_id);
CREATE INDEX idx_audit_events_target ON audit_events(target_user_id);

CREATE TABLE audit_anchors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    anchored_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_deleted_id INTEGER NOT NULL,
    last_event_hash BLOB NOT NULL
);

CREATE TABLE audit_retention_guard (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    allow_purge INTEGER NOT NULL DEFAULT 0
);
INSERT INTO audit_retention_guard (id, allow_purge) VALUES (1, 0);

-- +goose StatementBegin
CREATE TRIGGER audit_events_deny_update
BEFORE UPDATE ON audit_events
BEGIN
    SELECT RAISE(ABORT, 'audit_events are append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER audit_events_deny_delete
BEFORE DELETE ON audit_events
WHEN COALESCE((SELECT allow_purge FROM audit_retention_guard WHERE id = 1), 0) = 0
BEGIN
    SELECT RAISE(ABORT, 'audit_events are append-only');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS audit_events_deny_delete;
DROP TRIGGER IF EXISTS audit_events_deny_update;
DROP TABLE IF EXISTS audit_retention_guard;
DROP TABLE IF EXISTS audit_anchors;
DROP INDEX IF EXISTS idx_audit_events_target;
DROP INDEX IF EXISTS idx_audit_events_actor;
DROP INDEX IF EXISTS idx_audit_events_entity;
DROP INDEX IF EXISTS idx_audit_events_created_at;
DROP TABLE IF EXISTS audit_events;
