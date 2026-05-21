package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/model"
)

type AbsenceStore struct {
	db *DB
}

func NewAbsenceStore(db *DB) *AbsenceStore {
	return &AbsenceStore{db: db}
}

func (s *AbsenceStore) Create(ctx context.Context, a *model.Absence) error {
	res, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO absences (user_id, absence_date, absence_type, half_day, created_by) VALUES (?, ?, ?, ?, ?)`,
		a.UserID, a.AbsenceDate, a.AbsenceType, a.HalfDay, a.CreatedBy)
	if err != nil {
		return fmt.Errorf("create absence: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	a.ID = int(id)
	a.CreatedAt = time.Now().UTC()
	return nil
}

func (s *AbsenceStore) Delete(ctx context.Context, id int) error {
	_, err := s.db.DB.ExecContext(ctx, `DELETE FROM absences WHERE id = ?`, id)
	return err
}

func (s *AbsenceStore) GetByID(ctx context.Context, id int) (*model.Absence, error) {
	a := &model.Absence{}
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, user_id, absence_date, absence_type, half_day, created_by, created_at FROM absences WHERE id = ?`, id).
		Scan(&a.ID, &a.UserID, &a.AbsenceDate, &a.AbsenceType, &a.HalfDay, &a.CreatedBy, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (s *AbsenceStore) GetForUserDate(ctx context.Context, userID int, date string) (*model.Absence, error) {
	a := &model.Absence{}
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, user_id, absence_date, absence_type, half_day, created_by, created_at FROM absences WHERE user_id = ? AND absence_date = ?`,
		userID, date).Scan(&a.ID, &a.UserID, &a.AbsenceDate, &a.AbsenceType, &a.HalfDay, &a.CreatedBy, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (s *AbsenceStore) ListByUserDateRange(ctx context.Context, userID int, from, to string) ([]model.Absence, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT id, user_id, absence_date, absence_type, half_day, created_by, created_at FROM absences
		 WHERE user_id = ? AND absence_date >= ? AND absence_date <= ? ORDER BY absence_date`,
		userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.Absence
	for rows.Next() {
		var a model.Absence
		if err := rows.Scan(&a.ID, &a.UserID, &a.AbsenceDate, &a.AbsenceType, &a.HalfDay, &a.CreatedBy, &a.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

func (s *AbsenceStore) ListByDateRangeTypes(ctx context.Context, from, to string, types []model.AbsenceType) ([]model.Absence, error) {
	if len(types) == 0 {
		return nil, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(types)), ",")
	args := []interface{}{from, to}
	for _, t := range types {
		args = append(args, string(t))
	}
	q := fmt.Sprintf(
		`SELECT id, user_id, absence_date, absence_type, half_day, created_by, created_at FROM absences
		 WHERE absence_date >= ? AND absence_date <= ? AND absence_type IN (%s) ORDER BY absence_date, user_id`,
		ph)
	rows, err := s.db.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.Absence
	for rows.Next() {
		var a model.Absence
		if err := rows.Scan(&a.ID, &a.UserID, &a.AbsenceDate, &a.AbsenceType, &a.HalfDay, &a.CreatedBy, &a.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}
