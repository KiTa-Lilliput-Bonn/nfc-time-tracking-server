-- +goose Up
-- KiBiz-Rechnung im Dienstplan: Qualifikation je Person, KiBiz-Tabelle, Kinder-Wochenmuster je Gruppe
-- und Abweichungen pro Tag. Löschungen erfolgen explizit im Code (siehe 027_group_cash.sql).
CREATE TABLE staff_qualifications (
  user_id INTEGER PRIMARY KEY REFERENCES users(id),
  qualification TEXT NOT NULL CHECK (qualification IN ('fachkraft', 'ergaenzungskraft', 'sonstige')),
  updated_at TEXT NOT NULL
);

-- Anlage zu § 33 KiBiz je Gruppenform und Betreuungszeit, pro Gruppe und Woche: Kinderzahl,
-- Leitungsstunden, Gesamtpersonalkraftstunden und davon Mindest-Fachkraftstunden.
CREATE TABLE kibiz_rates (
  group_form TEXT NOT NULL CHECK (group_form IN ('I', 'II', 'III')),
  care_hours INTEGER NOT NULL CHECK (care_hours IN (25, 35, 45)),
  children REAL NOT NULL CHECK (children > 0),
  leitung_hours REAL NOT NULL CHECK (leitung_hours >= 0),
  total_hours REAL NOT NULL CHECK (total_hours >= 0),
  fachkraft_min_hours REAL NOT NULL CHECK (fachkraft_min_hours >= 0),
  PRIMARY KEY (group_form, care_hours)
);

-- Werte der Anlage zu § 33 KiBiz in der Fassung ab 01.08.2020 (gültig bis 31.07.2027),
-- https://recht.nrw.de/system/files/BA/41629-46546-sgv_216_20191203_1_anlage.htm
INSERT INTO kibiz_rates (group_form, care_hours, children, leitung_hours, total_hours, fachkraft_min_hours) VALUES
  ('I',   25, 20, 5,  71.5, 55.0),
  ('I',   35, 20, 7,  99.5, 77.0),
  ('I',   45, 20, 9, 128.0, 99.0),
  ('II',  25, 10, 5,  76.5, 55.0),
  ('II',  35, 10, 7, 107.0, 77.0),
  ('II',  45, 10, 9, 137.5, 99.0),
  ('III', 25, 25, 5,  71.0, 27.5),
  ('III', 35, 25, 7,  99.0, 38.5),
  ('III', 45, 20, 9, 114.0, 49.5);

-- Wochenmuster: counts = JSON [{"group_form":"II","care_hours":35,"count":6}, ...], gilt ab valid_from.
CREATE TABLE child_patterns (
  group_id INTEGER NOT NULL REFERENCES user_groups(id),
  valid_from TEXT NOT NULL,
  counts TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (group_id, valid_from)
);

-- Abweichung vom Muster an einem Tag (gleiches JSON-Format).
CREATE TABLE child_count_days (
  group_id INTEGER NOT NULL REFERENCES user_groups(id),
  day TEXT NOT NULL,
  counts TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (group_id, day)
);
CREATE INDEX idx_child_count_days_day ON child_count_days(day);

-- +goose Down
DROP TABLE child_count_days;
DROP TABLE child_patterns;
DROP TABLE kibiz_rates;
DROP TABLE staff_qualifications;
