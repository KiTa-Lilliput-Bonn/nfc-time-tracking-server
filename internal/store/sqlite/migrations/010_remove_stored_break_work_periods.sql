-- +goose Up
-- Pausen liegen nur noch implizit zwischen Arbeitsblöcken; gespeicherte is_break-Zeilen und deren Korrekturen entfernen.
DELETE FROM time_corrections
WHERE work_period_id IN (SELECT id FROM work_periods WHERE is_break = 1);

DELETE FROM work_periods WHERE is_break = 1;

-- +goose Down
SELECT 1;
