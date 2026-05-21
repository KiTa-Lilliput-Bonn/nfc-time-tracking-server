package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"nfc-time-tracking-server/internal/model"
)

type CompensationDayClaimStore struct {
	db *DB
}

func NewCompensationDayClaimStore(db *DB) *CompensationDayClaimStore {
	return &CompensationDayClaimStore{db: db}
}

func (s *CompensationDayClaimStore) EnsureForWorkDate(ctx context.Context, userID int, workDate string, hasEligibleWork bool) error {
	if hasEligibleWork {
		_, err := s.db.DB.ExecContext(ctx, `
INSERT INTO compensation_day_claims (user_id, work_date, status)
VALUES (?, ?, ?)
ON CONFLICT(user_id, work_date) DO NOTHING
`, userID, workDate, model.CompensationDayClaimOpen)
		return err
	}
	_, err := s.db.DB.ExecContext(ctx, `
DELETE FROM compensation_day_claims
WHERE user_id = ? AND work_date = ? AND status = ?
`, userID, workDate, model.CompensationDayClaimOpen)
	return err
}

func (s *CompensationDayClaimStore) GetOldestOpen(ctx context.Context, userID int) (*model.CompensationDayClaim, error) {
	row := s.db.DB.QueryRowContext(ctx, `
SELECT id, user_id, work_date, status, used_absence_id, created_at, updated_at
FROM compensation_day_claims
WHERE user_id = ? AND status = ?
ORDER BY work_date, id
LIMIT 1
`, userID, model.CompensationDayClaimOpen)
	return scanCompensationDayClaim(row)
}

func (s *CompensationDayClaimStore) MarkUsed(ctx context.Context, claimID int, absenceID int) error {
	res, err := s.db.DB.ExecContext(ctx, `
UPDATE compensation_day_claims
SET status = ?, used_absence_id = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND status = ?
`, model.CompensationDayClaimUsed, absenceID, claimID, model.CompensationDayClaimOpen)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("kein offener Ausgleichstag-Anspruch gefunden")
	}
	return nil
}

func (s *CompensationDayClaimStore) ReopenByAbsenceID(ctx context.Context, absenceID int) error {
	_, err := s.db.DB.ExecContext(ctx, `
UPDATE compensation_day_claims
SET status = ?, used_absence_id = NULL, updated_at = CURRENT_TIMESTAMP
WHERE used_absence_id = ? AND status = ?
`, model.CompensationDayClaimOpen, absenceID, model.CompensationDayClaimUsed)
	return err
}

func (s *CompensationDayClaimStore) Waive(ctx context.Context, userID int, claimID int) error {
	res, err := s.db.DB.ExecContext(ctx, `
UPDATE compensation_day_claims
SET status = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND user_id = ? AND status = ?
`, model.CompensationDayClaimWaived, claimID, userID, model.CompensationDayClaimOpen)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("offener Ausgleichstag-Anspruch nicht gefunden")
	}
	return nil
}

func (s *CompensationDayClaimStore) ListByUser(ctx context.Context, userID int, status *model.CompensationDayClaimStatus) ([]model.CompensationDayClaim, error) {
	query := `
SELECT id, user_id, work_date, status, used_absence_id, created_at, updated_at
FROM compensation_day_claims
WHERE user_id = ?`
	args := []interface{}{userID}
	if status != nil {
		query += ` AND status = ?`
		args = append(args, *status)
	}
	query += ` ORDER BY work_date DESC, id DESC`
	rows, err := s.db.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.CompensationDayClaim
	for rows.Next() {
		c, err := scanCompensationDayClaim(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (s *CompensationDayClaimStore) CountOpen(ctx context.Context, userID int) (int, error) {
	var n int
	err := s.db.DB.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM compensation_day_claims
WHERE user_id = ? AND status = ?
`, userID, model.CompensationDayClaimOpen).Scan(&n)
	return n, err
}

type compensationDayClaimScanner interface {
	Scan(dest ...interface{}) error
}

func scanCompensationDayClaim(row compensationDayClaimScanner) (*model.CompensationDayClaim, error) {
	var c model.CompensationDayClaim
	var used sql.NullInt64
	err := row.Scan(&c.ID, &c.UserID, &c.WorkDate, &c.Status, &used, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if used.Valid {
		id := int(used.Int64)
		c.UsedAbsenceID = &id
	}
	return &c, nil
}
