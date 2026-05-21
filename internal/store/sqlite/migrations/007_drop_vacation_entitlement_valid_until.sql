-- +goose Up
-- Urlaubsblöcke enden implizit mit dem nächsten Eintrag (valid_from).
ALTER TABLE vacation_entitlements DROP COLUMN valid_until;

-- +goose Down
ALTER TABLE vacation_entitlements ADD COLUMN valid_until DATE;
