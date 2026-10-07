-- +goose Up
-- Kraft je Person unabhängig von der Kontorolle: zusätzlich Leitung und Hauswirtschaft (beide zählen nicht).
-- Der Schalter „Leitung zählen“ entfällt. War er aus, zählten Leitungskonten bisher nie; damit sich daran
-- nichts ändert, bekommen sie dann die Kraft „Leitung“. Wer in der Gruppe arbeitet, wird auf Fachkraft gestellt.
CREATE TABLE staff_qualifications_new (
  user_id INTEGER PRIMARY KEY REFERENCES users(id),
  qualification TEXT NOT NULL CHECK (qualification IN ('fachkraft', 'ergaenzungskraft', 'leitung', 'hauswirtschaft', 'sonstige')),
  updated_at TEXT NOT NULL
);
INSERT INTO staff_qualifications_new (user_id, qualification, updated_at)
  SELECT user_id, qualification, updated_at FROM staff_qualifications;
DROP TABLE staff_qualifications;
ALTER TABLE staff_qualifications_new RENAME TO staff_qualifications;

UPDATE staff_qualifications SET qualification = 'leitung'
  WHERE user_id IN (SELECT id FROM users WHERE role = 'leitung')
    AND NOT EXISTS (SELECT 1 FROM settings WHERE key = 'kibiz_count_leitung' AND value = 'true');
DELETE FROM settings WHERE key = 'kibiz_count_leitung';

-- +goose Down
DELETE FROM staff_qualifications WHERE qualification IN ('leitung', 'hauswirtschaft');
CREATE TABLE staff_qualifications_old (
  user_id INTEGER PRIMARY KEY REFERENCES users(id),
  qualification TEXT NOT NULL CHECK (qualification IN ('fachkraft', 'ergaenzungskraft', 'sonstige')),
  updated_at TEXT NOT NULL
);
INSERT INTO staff_qualifications_old SELECT user_id, qualification, updated_at FROM staff_qualifications;
DROP TABLE staff_qualifications;
ALTER TABLE staff_qualifications_old RENAME TO staff_qualifications;
