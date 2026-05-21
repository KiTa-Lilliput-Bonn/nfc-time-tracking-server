-- +goose Up
-- SQLite ALTER ADD COLUMN erlaubt nur konstante DEFAULTs (kein CURRENT_TIMESTAMP).
ALTER TABLE weekly_hours ADD COLUMN created_at TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:00';
ALTER TABLE vacation_entitlements ADD COLUMN created_at TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:00';

UPDATE weekly_hours SET created_at = datetime(valid_from || 'T12:00:00');
UPDATE vacation_entitlements SET created_at = datetime(valid_from || 'T12:00:00');

-- +goose Down
ALTER TABLE weekly_hours DROP COLUMN created_at;
ALTER TABLE vacation_entitlements DROP COLUMN created_at;
