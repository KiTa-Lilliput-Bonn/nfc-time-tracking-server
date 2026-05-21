package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"nfc-time-tracking-server/internal/model"
)

type ClosureDayStore struct {
	db *DB
}

func NewClosureDayStore(db *DB) *ClosureDayStore {
	return &ClosureDayStore{db: db}
}

func (s *ClosureDayStore) Create(ctx context.Context, c *model.ClosureDay) error {
	res, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO closure_days (closure_date, name, created_by) VALUES (?, ?, ?)`,
		c.ClosureDate, c.Name, c.CreatedBy)
	if err != nil {
		return fmt.Errorf("create closure day: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	c.ID = int(id)
	return nil
}

func (s *ClosureDayStore) Delete(ctx context.Context, id int) error {
	_, err := s.db.DB.ExecContext(ctx, `DELETE FROM closure_days WHERE id = ?`, id)
	return err
}

func (s *ClosureDayStore) List(ctx context.Context) ([]model.ClosureDay, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT id, closure_date, name, created_by FROM closure_days ORDER BY closure_date`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.ClosureDay
	for rows.Next() {
		var c model.ClosureDay
		if err := rows.Scan(&c.ID, &c.ClosureDate, &c.Name, &c.CreatedBy); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *ClosureDayStore) GetForDate(ctx context.Context, date string) (*model.ClosureDay, error) {
	c := &model.ClosureDay{}
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, closure_date, name, created_by FROM closure_days WHERE closure_date = ?`, date).
		Scan(&c.ID, &c.ClosureDate, &c.Name, &c.CreatedBy)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}
