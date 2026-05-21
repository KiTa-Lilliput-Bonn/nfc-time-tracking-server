-- +goose Up
-- Wochenstunden-Blöcke enden implizit mit dem nächsten Eintrag (valid_from).
ALTER TABLE weekly_hours DROP COLUMN valid_until;

-- +goose Down
ALTER TABLE weekly_hours ADD COLUMN valid_until DATE;
