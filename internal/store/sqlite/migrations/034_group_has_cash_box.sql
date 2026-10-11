-- +goose Up
-- Die Leitung legt pro Gruppe fest, ob sie eine Gruppenkasse hat. Gruppen, deren Kasse schon genutzt wird
-- (Kassenwarte, Anspruch, Anfangsbestand oder Buchungen), behalten ihre Kasse; alle anderen starten ohne.
ALTER TABLE user_groups ADD COLUMN has_cash_box INTEGER NOT NULL DEFAULT 0;

UPDATE user_groups SET has_cash_box = 1 WHERE id IN (
  SELECT group_id FROM cash_keepers
  UNION SELECT group_id FROM cash_allowances
  UNION SELECT group_id FROM cash_openings
  UNION SELECT group_id FROM cash_entries
);

-- +goose Down
SELECT 1;
