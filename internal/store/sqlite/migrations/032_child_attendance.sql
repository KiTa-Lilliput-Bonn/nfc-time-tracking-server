-- +goose Up
-- Anwesenheitsliste der Kinder je Gruppe: Kinder mit Namen, Kommen/Gehen je Tag, vorab gemeldetes
-- Fehlen (auch nur teilweise: später bringen, früher abholen) und Gruppenaccounts für ein Gerät im Flur.
-- Löschungen erfolgen explizit im Code (siehe 027_group_cash.sql).
CREATE TABLE children (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id INTEGER NOT NULL REFERENCES user_groups(id),
  first_name TEXT NOT NULL,
  last_name TEXT NOT NULL DEFAULT '',
  active INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX idx_children_group ON children(group_id);

-- Kommen und Gehen als Uhrzeit HH:MM (Ortszeit) je Kind und Tag.
CREATE TABLE child_attendance (
  child_id INTEGER NOT NULL REFERENCES children(id),
  day TEXT NOT NULL,
  arrived_at TEXT,
  left_at TEXT,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (child_id, day)
);
CREATE INDEX idx_child_attendance_day ON child_attendance(day);

-- Vorab gemeldet: ohne Uhrzeiten fehlt das Kind ganz; mit arrive_from kommt es später,
-- mit leave_at wird es früher abgeholt.
CREATE TABLE child_notices (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  child_id INTEGER NOT NULL REFERENCES children(id),
  date_from TEXT NOT NULL,
  date_to TEXT NOT NULL,
  reason TEXT NOT NULL CHECK (reason IN ('vacation', 'sick', 'other')),
  arrive_from TEXT,
  leave_at TEXT,
  note TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX idx_child_notices_child ON child_notices(child_id, date_to);

-- Ein Konto je Gruppe, das nur die Anwesenheitsliste dieser Gruppe sieht. session_version
-- steigt beim Zurücksetzen des Passworts, damit angemeldete Geräte abgemeldet werden.
CREATE TABLE group_accounts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id INTEGER NOT NULL UNIQUE REFERENCES user_groups(id),
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  active INTEGER NOT NULL DEFAULT 1,
  session_version INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

-- +goose Down
DROP TABLE group_accounts;
DROP TABLE child_notices;
DROP TABLE child_attendance;
DROP TABLE children;
