package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"nfc-time-tracking-server/internal/model"
)

type VacationEntitlementStore struct {
	db *DB
}

func NewVacationEntitlementStore(db *DB) *VacationEntitlementStore {
	return &VacationEntitlementStore{db: db}
}

func (s *VacationEntitlementStore) Set(ctx context.Context, ve *model.VacationEntitlement) error {
	res, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO vacation_entitlements (user_id, days_per_year, valid_from, created_at) VALUES (?, ?, ?, datetime('now'))`,
		ve.UserID, ve.DaysPerYear, ve.ValidFrom)
	if err != nil {
		return fmt.Errorf("set vacation entitlement: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	got, err := s.GetByID(ctx, ve.UserID, int(id))
	if err != nil {
		return err
	}
	if got == nil {
		return fmt.Errorf("set vacation entitlement: row not found after insert")
	}
	*ve = *got
	return nil
}

func (s *VacationEntitlementStore) Delete(ctx context.Context, userID int, id int) error {
	res, err := s.db.DB.ExecContext(ctx,
		`DELETE FROM vacation_entitlements WHERE id = ? AND user_id = ?`,
		id, userID)
	if err != nil {
		return fmt.Errorf("delete vacation entitlement: %w", err)
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

func (s *VacationEntitlementStore) GetByID(ctx context.Context, userID int, id int) (*model.VacationEntitlement, error) {
	ve := &model.VacationEntitlement{}
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, user_id, days_per_year, valid_from, created_at FROM vacation_entitlements
		 WHERE id = ? AND user_id = ?`,
		id, userID).Scan(&ve.ID, &ve.UserID, &ve.DaysPerYear, &ve.ValidFrom, &ve.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ve, nil
}

// GetForDate liefert den Eintrag mit dem größten valid_from, das noch ≤ date ist
// (ein späterer Eintrag überschreibt den vorherigen Block ab seinem Gültig-ab-Datum).
func (s *VacationEntitlementStore) GetForDate(ctx context.Context, userID int, date string) (*model.VacationEntitlement, error) {
	ve := &model.VacationEntitlement{}
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, user_id, days_per_year, valid_from, created_at FROM vacation_entitlements
		 WHERE user_id = ? AND valid_from <= ?
		 ORDER BY valid_from DESC LIMIT 1`,
		userID, date).Scan(&ve.ID, &ve.UserID, &ve.DaysPerYear, &ve.ValidFrom, &ve.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ve, nil
}

func (s *VacationEntitlementStore) ListByUser(ctx context.Context, userID int) ([]model.VacationEntitlement, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT id, user_id, days_per_year, valid_from, created_at FROM vacation_entitlements WHERE user_id = ? ORDER BY valid_from`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.VacationEntitlement
	for rows.Next() {
		var ve model.VacationEntitlement
		if err := rows.Scan(&ve.ID, &ve.UserID, &ve.DaysPerYear, &ve.ValidFrom, &ve.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, ve)
	}
	return list, rows.Err()
}
