-- +goose Up
DELETE FROM settings WHERE key LIKE 'ftp_%';

-- +goose Down
-- FTP settings were intentionally removed; no restore.
