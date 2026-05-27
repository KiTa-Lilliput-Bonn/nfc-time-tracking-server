-- +goose Up
ALTER TABLE holidays ADD COLUMN kind TEXT NOT NULL DEFAULT 'feiertag'
  CHECK(kind IN ('feiertag', 'brauchtum'));
UPDATE holidays SET kind = 'brauchtum'
  WHERE name IN ('Heiligabend', 'Silvester', 'Rosenmontag');

-- +goose Down
ALTER TABLE holidays DROP COLUMN kind;
