package sqlite

import (
	"context"
	"fmt"
	"time"

	"nfc-time-tracking-server/internal/model"
)

type ShiftAlertDismissalStore struct {
	db *DB
}

func NewShiftAlertDismissalStore(db *DB) *ShiftAlertDismissalStore {
	return &ShiftAlertDismissalStore{db: db}
}

func (s *ShiftAlertDismissalStore) Create(ctx context.Context, userID int, workDate string, dismissedBy int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.DB.ExecContext(ctx,
		`INSERT OR REPLACE INTO shift_alert_dismissals (user_id, work_date, dismissed_by, created_at) VALUES (?, ?, ?, ?)`,
		userID, workDate, dismissedBy, now)
	if err != nil {
		return fmt.Errorf("create shift alert dismissal: %w", err)
	}
	return nil
}

func (s *ShiftAlertDismissalStore) ListByDateRange(ctx context.Context, from, to string) ([]model.ShiftAlertDismissal, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT user_id, work_date, dismissed_by, created_at FROM shift_alert_dismissals WHERE work_date >= ? AND work_date <= ?`,
		from, to)
	if err != nil {
		return nil, fmt.Errorf("list shift alert dismissals: %w", err)
	}
	defer rows.Close()

	var list []model.ShiftAlertDismissal
	for rows.Next() {
		var d model.ShiftAlertDismissal
		var createdAt string
		if err := rows.Scan(&d.UserID, &d.WorkDate, &d.DismissedBy, &createdAt); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			t, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		}
		d.CreatedAt = t
		list = append(list, d)
	}
	return list, rows.Err()
}
