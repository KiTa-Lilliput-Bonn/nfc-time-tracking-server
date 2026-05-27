-- +goose Up
ALTER TABLE users ADD COLUMN default_team_meeting_participant BOOLEAN NOT NULL DEFAULT 1;

-- +goose Down
SELECT 1;
