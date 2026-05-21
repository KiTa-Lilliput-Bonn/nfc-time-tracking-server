package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"nfc-time-tracking-server/internal/model"
)

type WeeklyHoursStore struct {
	db *DB
}

func NewWeeklyHoursStore(db *DB) *WeeklyHoursStore {
	return &WeeklyHoursStore{db: db}
}

func (s *WeeklyHoursStore) Set(ctx context.Context, wh *model.WeeklyHours) error {
	res, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO weekly_hours (user_id, hours_per_week, valid_from, created_at) VALUES (?, ?, ?, datetime('now'))`,
		wh.UserID, wh.HoursPerWeek, wh.ValidFrom)
	if err != nil {
		return fmt.Errorf("set weekly hours: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	got, err := s.GetByID(ctx, wh.UserID, int(id))
	if err != nil {
		return err
	}
	if got == nil {
		return fmt.Errorf("set weekly hours: row not found after insert")
	}
	*wh = *got
	return nil
}

func (s *WeeklyHoursStore) Delete(ctx context.Context, userID int, id int) error {
	res, err := s.db.DB.ExecContext(ctx,
		`DELETE FROM weekly_hours WHERE id = ? AND user_id = ?`,
		id, userID)
	if err != nil {
		return fmt.Errorf("delete weekly hours: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *WeeklyHoursStore) GetByID(ctx context.Context, userID int, id int) (*model.WeeklyHours, error) {
	wh := &model.WeeklyHours{}
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, user_id, hours_per_week, valid_from, created_at FROM weekly_hours
		 WHERE id = ? AND user_id = ?`,
		id, userID).Scan(&wh.ID, &wh.UserID, &wh.HoursPerWeek, &wh.ValidFrom, &wh.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return wh, nil
}

// GetForDate liefert den Eintrag mit dem größten valid_from, das noch ≤ date ist
// (ein späterer Eintrag überschreibt den vorherigen Block ab seinem Gültig-ab-Datum).
func (s *WeeklyHoursStore) GetForDate(ctx context.Context, userID int, date string) (*model.WeeklyHours, error) {
	wh := &model.WeeklyHours{}
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, user_id, hours_per_week, valid_from, created_at FROM weekly_hours
		 WHERE user_id = ? AND valid_from <= ?
		 ORDER BY valid_from DESC LIMIT 1`,
		userID, date).Scan(&wh.ID, &wh.UserID, &wh.HoursPerWeek, &wh.ValidFrom, &wh.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return wh, nil
}

func (s *WeeklyHoursStore) ListByUser(ctx context.Context, userID int) ([]model.WeeklyHours, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT id, user_id, hours_per_week, valid_from, created_at FROM weekly_hours WHERE user_id = ? ORDER BY valid_from`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.WeeklyHours
	for rows.Next() {
		var wh model.WeeklyHours
		if err := rows.Scan(&wh.ID, &wh.UserID, &wh.HoursPerWeek, &wh.ValidFrom, &wh.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, wh)
	}
	return list, rows.Err()
}
