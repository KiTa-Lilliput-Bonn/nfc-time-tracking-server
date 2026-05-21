-- +goose Up
CREATE TABLE IF NOT EXISTS schedule_week_notes (
    iso_week_year INTEGER NOT NULL,
    iso_week INTEGER NOT NULL CHECK (iso_week >= 1 AND iso_week <= 53),
    notes TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (iso_week_year, iso_week)
);

-- +goose Down
DROP TABLE IF EXISTS schedule_week_notes;
