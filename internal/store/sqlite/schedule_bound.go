package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"nfc-time-tracking-server/internal/model"
)

type ScheduleBoundStore struct {
	db *DB
}

func NewScheduleBoundStore(db *DB) *ScheduleBoundStore {
	return &ScheduleBoundStore{db: db}
}

func (s *ScheduleBoundStore) Set(ctx context.Context, row *model.ScheduleBoundSetting) error {
	bound := 0
	if row.ScheduleBound {
		bound = 1
	}
	res, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO user_schedule_bound (user_id, schedule_bound, valid_from, created_at) VALUES (?, ?, ?, datetime('now'))`,
		row.UserID, bound, row.ValidFrom)
	if err != nil {
		return fmt.Errorf("set schedule bound: %w", err)
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
		return fmt.Errorf("set schedule bound: row not found after insert")
	}
	*row = *got
	return nil
}

func (s *ScheduleBoundStore) Delete(ctx context.Context, userID int, id int) error {
	res, err := s.db.DB.ExecContext(ctx,
		`DELETE FROM user_schedule_bound WHERE id = ? AND user_id = ?`,
		id, userID)
	if err != nil {
		return fmt.Errorf("delete schedule bound: %w", err)
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

func (s *ScheduleBoundStore) GetByID(ctx context.Context, userID int, id int) (*model.ScheduleBoundSetting, error) {
	row := &model.ScheduleBoundSetting{}
	var bound int
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, user_id, schedule_bound, valid_from, created_at FROM user_schedule_bound
		 WHERE id = ? AND user_id = ?`,
		id, userID).Scan(&row.ID, &row.UserID, &bound, &row.ValidFrom, &row.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row.ScheduleBound = bound != 0
	return row, nil
}

func (s *ScheduleBoundStore) ListByUser(ctx context.Context, userID int) ([]model.ScheduleBoundSetting, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT id, user_id, schedule_bound, valid_from, created_at FROM user_schedule_bound WHERE user_id = ? ORDER BY valid_from`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.ScheduleBoundSetting
	for rows.Next() {
		var row model.ScheduleBoundSetting
		var bound int
		if err := rows.Scan(&row.ID, &row.UserID, &bound, &row.ValidFrom, &row.CreatedAt); err != nil {
			return nil, err
		}
		row.ScheduleBound = bound != 0
		list = append(list, row)
	}
	return list, rows.Err()
}
