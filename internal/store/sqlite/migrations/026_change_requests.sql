-- +goose Up
-- Anträge von Mitarbeitenden (Zeitkorrektur, fehlender Zeiteintrag, Urlaub), die erst nach
-- Freigabe durch die Leitung wirksam werden.
CREATE TABLE change_requests (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id),
  kind TEXT NOT NULL CHECK (kind IN ('time_correction', 'time_entry', 'vacation')),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'withdrawn')),
  work_period_id INTEGER REFERENCES work_periods(id),
  work_date TEXT NOT NULL DEFAULT '',
  original_in TEXT,
  original_out TEXT,
  punch_in TEXT,
  punch_out TEXT,
  date_from TEXT NOT NULL DEFAULT '',
  date_to TEXT NOT NULL DEFAULT '',
  half_day INTEGER NOT NULL DEFAULT 0,
  reason TEXT NOT NULL DEFAULT '',
  decided_by INTEGER REFERENCES users(id),
  decided_at TEXT,
  decision_comment TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX idx_change_requests_status ON change_requests(status, created_at);
CREATE INDEX idx_change_requests_user ON change_requests(user_id, created_at);

-- +goose Down
DROP TABLE IF EXISTS change_requests;
