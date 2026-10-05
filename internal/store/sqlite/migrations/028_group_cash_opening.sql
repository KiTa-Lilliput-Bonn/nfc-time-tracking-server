-- +goose Up
-- Anfangsbestand einer Gruppenkasse: Kassenbestand und Ansparkonto zum Stichtag opening_date.
-- Buchungen müssen am oder nach dem Stichtag liegen.
CREATE TABLE cash_openings (
  group_id INTEGER PRIMARY KEY REFERENCES user_groups(id),
  opening_date TEXT NOT NULL,
  cash_cents INTEGER NOT NULL,
  savings_cents INTEGER NOT NULL,
  updated_by INTEGER REFERENCES users(id),
  updated_at TEXT NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS cash_openings;
