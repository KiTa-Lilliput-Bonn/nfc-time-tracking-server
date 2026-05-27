package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"nfc-time-tracking-server/internal/model"
)

type FixedNonWorkWeekdaysStore struct {
	db *DB
}

func NewFixedNonWorkWeekdaysStore(db *DB) *FixedNonWorkWeekdaysStore {
	return &FixedNonWorkWeekdaysStore{db: db}
}

func (s *FixedNonWorkWeekdaysStore) Set(ctx context.Context, row *model.FixedNonWorkWeekdays) error {
	res, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO fixed_non_work_weekdays (user_id, weekdays, valid_from, created_at) VALUES (?, ?, ?, datetime('now'))`,
		row.UserID, marshalFixedNonWorkWeekdays(row.Weekdays), row.ValidFrom)
	if err != nil {
		return fmt.Errorf("set fixed non-work weekdays: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	got, err := s.GetByID(ctx, row.UserID, int(id))
	if err != nil {
		return err
	}
	if got == nil {
		return fmt.Errorf("set fixed non-work weekdays: row not found after insert")
	}
	*row = *got
	return nil
}

func (s *FixedNonWorkWeekdaysStore) Delete(ctx context.Context, userID int, id int) error {
	res, err := s.db.DB.ExecContext(ctx,
		`DELETE FROM fixed_non_work_weekdays WHERE id = ? AND user_id = ?`,
		id, userID)
	if err != nil {
		return fmt.Errorf("delete fixed non-work weekdays: %w", err)
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

func (s *FixedNonWorkWeekdaysStore) GetByID(ctx context.Context, userID int, id int) (*model.FixedNonWorkWeekdays, error) {
	row := &model.FixedNonWorkWeekdays{}
	var wdays string
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, user_id, weekdays, valid_from, created_at FROM fixed_non_work_weekdays
		 WHERE id = ? AND user_id = ?`,
		id, userID).Scan(&row.ID, &row.UserID, &wdays, &row.ValidFrom, &row.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := unmarshalFixedNonWorkWeekdays(wdays, &row.Weekdays); err != nil {
		return nil, fmt.Errorf("fixed non-work weekdays %d: %w", id, err)
	}
	return row, nil
}

func (s *FixedNonWorkWeekdaysStore) GetForDate(ctx context.Context, userID int, date string) (*model.FixedNonWorkWeekdays, error) {
	row := &model.FixedNonWorkWeekdays{}
	var wdays string
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, user_id, weekdays, valid_from, created_at FROM fixed_non_work_weekdays
		 WHERE user_id = ? AND valid_from <= ?
		 ORDER BY valid_from DESC LIMIT 1`,
		userID, date).Scan(&row.ID, &row.UserID, &wdays, &row.ValidFrom, &row.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := unmarshalFixedNonWorkWeekdays(wdays, &row.Weekdays); err != nil {
		return nil, fmt.Errorf("fixed non-work weekdays user %d: %w", userID, err)
	}
	return row, nil
}

func (s *FixedNonWorkWeekdaysStore) ListByUser(ctx context.Context, userID int) ([]model.FixedNonWorkWeekdays, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT id, user_id, weekdays, valid_from, created_at FROM fixed_non_work_weekdays WHERE user_id = ? ORDER BY valid_from`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.FixedNonWorkWeekdays
	for rows.Next() {
		var row model.FixedNonWorkWeekdays
		var wdays string
		if err := rows.Scan(&row.ID, &row.UserID, &wdays, &row.ValidFrom, &row.CreatedAt); err != nil {
			return nil, err
		}
		if err := unmarshalFixedNonWorkWeekdays(wdays, &row.Weekdays); err != nil {
			return nil, fmt.Errorf("fixed non-work weekdays user %d: %w", userID, err)
		}
		list = append(list, row)
	}
	return list, rows.Err()
}
