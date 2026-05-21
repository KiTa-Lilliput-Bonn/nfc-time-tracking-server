-- +goose Up
-- NFC-Zuweisungen enden implizit mit dem nächsten Eintrag (assigned_from) bzw. Neuzuweisung.
ALTER TABLE nfc_tags DROP COLUMN assigned_until;

-- +goose Down
ALTER TABLE nfc_tags ADD COLUMN assigned_until DATE;
