-- +goose Up
-- Datenschutz: Anwesenheitsdaten der Kinder werden nach Fristen gelöscht (siehe internal/service/childretention).
-- deactivated_at merkt sich die Abmeldung, damit abgemeldete Kinder nach der Frist samt Daten verschwinden.
ALTER TABLE children ADD COLUMN deactivated_at TEXT;
-- Geburtsmonat (YYYY-MM) reicht für das Alter; den genauen Tag speichern wir nicht.
ALTER TABLE children ADD COLUMN birth_month TEXT;
UPDATE children SET deactivated_at = updated_at WHERE active = 0;

-- Datensparsamkeit: vom Nachnamen nur den Anfangsbuchstaben behalten („Finn D.“).
UPDATE children SET last_name = upper(substr(trim(last_name), 1, 1)) WHERE last_name != '';

-- Anonyme Zahlen, die vor dem Löschen von Kommen/Gehen gebildet werden (für die Dienstplanung):
-- Kinder je Gruppe, Tag und halber Stunde (slot = Beginn HH:MM); slot '' = Kinder, die an dem Tag da waren.
CREATE TABLE child_attendance_stats (
  group_id INTEGER NOT NULL REFERENCES user_groups(id),
  day TEXT NOT NULL,
  slot TEXT NOT NULL,
  children INTEGER NOT NULL,
  PRIMARY KEY (group_id, day, slot)
);

-- +goose Down
DROP TABLE child_attendance_stats;
ALTER TABLE children DROP COLUMN birth_month;
ALTER TABLE children DROP COLUMN deactivated_at;
