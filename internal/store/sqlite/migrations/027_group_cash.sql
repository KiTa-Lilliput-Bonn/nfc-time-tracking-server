-- +goose Up
-- Gruppenkassen: pro Gruppe eine Kasse mit Kassenwarten, monatlichem Anspruch, Buchungen und Belegen.
-- Hinweis: PRAGMA foreign_keys gilt nicht zuverlässig für alle Pool-Verbindungen; Löschungen erfolgen
-- daher explizit im Code statt per ON DELETE CASCADE.
CREATE TABLE cash_keepers (
  group_id INTEGER NOT NULL REFERENCES user_groups(id),
  user_id INTEGER NOT NULL REFERENCES users(id),
  created_at TEXT NOT NULL,
  PRIMARY KEY (group_id, user_id)
);
CREATE INDEX idx_cash_keepers_user ON cash_keepers(user_id);

-- Monatlicher Anspruch (Cent), gültig ab Monat valid_from (YYYY-MM) bis zur nächsten Version.
CREATE TABLE cash_allowances (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id INTEGER NOT NULL REFERENCES user_groups(id),
  valid_from TEXT NOT NULL,
  amount_cents INTEGER NOT NULL CHECK (amount_cents >= 0),
  created_by INTEGER REFERENCES users(id),
  created_at TEXT NOT NULL,
  UNIQUE (group_id, valid_from)
);

-- Buchungen. source nur bei Einnahmen: allowance = Monatsbetrag (for_month), savings = aus Ansparkonto,
-- other = sonstige Einnahme (z. B. Spende).
CREATE TABLE cash_entries (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id INTEGER NOT NULL REFERENCES user_groups(id),
  kind TEXT NOT NULL CHECK (kind IN ('expense', 'income')),
  source TEXT NOT NULL DEFAULT '' CHECK (source IN ('', 'allowance', 'savings', 'other')),
  for_month TEXT NOT NULL DEFAULT '',
  entry_date TEXT NOT NULL,
  amount_cents INTEGER NOT NULL CHECK (amount_cents > 0),
  description TEXT NOT NULL DEFAULT '',
  created_by INTEGER REFERENCES users(id),
  created_at TEXT NOT NULL,
  updated_by INTEGER REFERENCES users(id),
  updated_at TEXT NOT NULL
);
CREATE INDEX idx_cash_entries_group ON cash_entries(group_id, entry_date);

-- Belege (PDF/Foto) liegen in der Datenbank, damit das bestehende Backup sie mitsichert.
CREATE TABLE cash_receipts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  entry_id INTEGER NOT NULL REFERENCES cash_entries(id),
  filename TEXT NOT NULL,
  content_type TEXT NOT NULL,
  size_bytes INTEGER NOT NULL,
  data BLOB NOT NULL,
  uploaded_by INTEGER REFERENCES users(id),
  created_at TEXT NOT NULL
);
CREATE INDEX idx_cash_receipts_entry ON cash_receipts(entry_id);

-- +goose Down
DROP TABLE IF EXISTS cash_receipts;
DROP TABLE IF EXISTS cash_entries;
DROP TABLE IF EXISTS cash_allowances;
DROP TABLE IF EXISTS cash_keepers;
