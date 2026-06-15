-- +goose Up
CREATE TABLE shift_alert_dismissals (
  user_id INTEGER NOT NULL REFERENCES users(id),
  work_date TEXT NOT NULL,
  dismissed_by INTEGER NOT NULL REFERENCES users(id),
  created_at TEXT NOT NULL,
  PRIMARY KEY (user_id, work_date)
);

INSERT OR IGNORE INTO settings (key, value) VALUES ('shift_alert_max_hours', '11');
INSERT OR IGNORE INTO settings (key, value) VALUES ('shift_alert_late_end', '21:00');

-- +goose Down
DROP TABLE IF EXISTS shift_alert_dismissals;
