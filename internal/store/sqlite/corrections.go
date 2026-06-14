package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"nfc-time-tracking-server/internal/model"
)

type CorrectionStore struct {
	db *DB
}

func NewCorrectionStore(db *DB) *CorrectionStore {
	return &CorrectionStore{db: db}
}

func scanTimeCorrection(cin, cout string, disabledInt int, c *model.TimeCorrection) {
	c.CorrectedIn, _ = time.Parse(time.RFC3339Nano, cin)
	if c.CorrectedIn.IsZero() {
		c.CorrectedIn, _ = time.Parse(time.RFC3339, cin)
	}
	c.CorrectedOut, _ = time.Parse(time.RFC3339Nano, cout)
	if c.CorrectedOut.IsZero() {
		c.CorrectedOut, _ = time.Parse(time.RFC3339, cout)
	}
	c.Disabled = disabledInt != 0
}

func (s *CorrectionStore) Create(ctx context.Context, c *model.TimeCorrection) error {
	disabled := 0
	if c.Disabled {
		disabled = 1
	}
	res, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO time_corrections (work_period_id, corrected_in, corrected_out, reason, corrected_by, disabled) VALUES (?, ?, ?, ?, ?, ?)`,
		c.WorkPeriodID, c.CorrectedIn.UTC().Format(time.RFC3339Nano), c.CorrectedOut.UTC().Format(time.RFC3339Nano), c.Reason, c.CorrectedBy, disabled)
	if err != nil {
		return fmt.Errorf("create correction: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	c.ID = int(id)
	c.CreatedAt = time.Now().UTC()
	return nil
}

func (s *CorrectionStore) GetLatestForPeriod(ctx context.Context, workPeriodID int) (*model.TimeCorrection, error) {
	c := &model.TimeCorrection{}
	var cin, cout string
	var disabledInt int
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, work_period_id, corrected_in, corrected_out, reason, corrected_by, created_at, disabled FROM time_corrections
		 WHERE work_period_id = ? ORDER BY id DESC LIMIT 1`, workPeriodID).
		Scan(&c.ID, &c.WorkPeriodID, &cin, &cout, &c.Reason, &c.CorrectedBy, &c.CreatedAt, &disabledInt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	scanTimeCorrection(cin, cout, disabledInt, c)
	return c, nil
}

func (s *CorrectionStore) ListByUser(ctx context.Context, userID int, from, to string) ([]model.TimeCorrection, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT tc.id, tc.work_period_id, tc.corrected_in, tc.corrected_out, tc.reason, tc.corrected_by, tc.created_at, tc.disabled
		 FROM time_corrections tc
		 INNER JOIN work_periods wp ON wp.id = tc.work_period_id
		 WHERE wp.user_id = ? AND wp.work_date >= ? AND wp.work_date <= ?
		 ORDER BY tc.created_at DESC`,
		userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.TimeCorrection
	for rows.Next() {
		var c model.TimeCorrection
		var cin, cout string
		var disabledInt int
		if err := rows.Scan(&c.ID, &c.WorkPeriodID, &cin, &cout, &c.Reason, &c.CorrectedBy, &c.CreatedAt, &disabledInt); err != nil {
			return nil, err
		}
		scanTimeCorrection(cin, cout, disabledInt, &c)
		list = append(list, c)
	}
	return list, rows.Err()
}
