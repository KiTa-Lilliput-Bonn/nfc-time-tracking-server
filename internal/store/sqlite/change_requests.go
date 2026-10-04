package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store"
)

type ChangeRequestStore struct {
	db *DB
}

func NewChangeRequestStore(db *DB) *ChangeRequestStore {
	return &ChangeRequestStore{db: db}
}

const changeRequestColumns = `id, user_id, kind, status, work_period_id, work_date, original_in, original_out,
	punch_in, punch_out, date_from, date_to, half_day, reason, decided_by, decided_at, decision_comment, created_at`

func formatOptTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseOptTime(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, ns.String)
	if err != nil {
		return nil
	}
	return &t
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanChangeRequest(row rowScanner) (*model.ChangeRequest, error) {
	var c model.ChangeRequest
	var wpID, decidedBy sql.NullInt64
	var origIn, origOut, pin, pout, decidedAt sql.NullString
	var createdAt string
	var half int
	if err := row.Scan(&c.ID, &c.UserID, &c.Kind, &c.Status, &wpID, &c.WorkDate, &origIn, &origOut,
		&pin, &pout, &c.DateFrom, &c.DateTo, &half, &c.Reason, &decidedBy, &decidedAt, &c.DecisionComment, &createdAt); err != nil {
		return nil, err
	}
	if wpID.Valid {
		v := int(wpID.Int64)
		c.WorkPeriodID = &v
	}
	if decidedBy.Valid {
		v := int(decidedBy.Int64)
		c.DecidedBy = &v
	}
	c.OriginalIn = parseOptTime(origIn)
	c.OriginalOut = parseOptTime(origOut)
	c.PunchIn = parseOptTime(pin)
	c.PunchOut = parseOptTime(pout)
	c.DecidedAt = parseOptTime(decidedAt)
	if t := parseOptTime(sql.NullString{String: createdAt, Valid: true}); t != nil {
		c.CreatedAt = *t
	}
	c.HalfDay = half != 0
	return &c, nil
}

func (s *ChangeRequestStore) Create(ctx context.Context, c *model.ChangeRequest) error {
	c.Status = model.RequestPending
	c.CreatedAt = time.Now().UTC()
	half := 0
	if c.HalfDay {
		half = 1
	}
	var wpID interface{}
	if c.WorkPeriodID != nil {
		wpID = *c.WorkPeriodID
	}
	res, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO change_requests (user_id, kind, status, work_period_id, work_date, original_in, original_out,
			punch_in, punch_out, date_from, date_to, half_day, reason, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.UserID, c.Kind, c.Status, wpID, c.WorkDate, formatOptTime(c.OriginalIn), formatOptTime(c.OriginalOut),
		formatOptTime(c.PunchIn), formatOptTime(c.PunchOut), c.DateFrom, c.DateTo, half, c.Reason,
		c.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("create change request: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	c.ID = int(id)
	return nil
}

func (s *ChangeRequestStore) GetByID(ctx context.Context, id int) (*model.ChangeRequest, error) {
	row := s.db.DB.QueryRowContext(ctx, `SELECT `+changeRequestColumns+` FROM change_requests WHERE id = ?`, id)
	c, err := scanChangeRequest(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

func (s *ChangeRequestStore) List(ctx context.Context, f store.ChangeRequestFilter) ([]model.ChangeRequest, error) {
	var where []string
	var args []interface{}
	if f.UserID != nil {
		where = append(where, "user_id = ?")
		args = append(args, *f.UserID)
	}
	if len(f.Statuses) > 0 {
		ph := make([]string, len(f.Statuses))
		for i, st := range f.Statuses {
			ph[i] = "?"
			args = append(args, st)
		}
		where = append(where, "status IN ("+strings.Join(ph, ",")+")")
	}
	if f.Kind != "" {
		where = append(where, "kind = ?")
		args = append(args, f.Kind)
	}
	q := `SELECT ` + changeRequestColumns + ` FROM change_requests`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY created_at DESC, id DESC"
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	rows, err := s.db.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.ChangeRequest
	for rows.Next() {
		c, err := scanChangeRequest(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *c)
	}
	return list, rows.Err()
}

func (s *ChangeRequestStore) CountPending(ctx context.Context) (int, error) {
	var n int
	err := s.db.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM change_requests WHERE status = 'pending'`).Scan(&n)
	return n, err
}

// SetDecision changes a pending request to status. It returns false when the request was no longer pending,
// so concurrent decisions cannot both apply.
func (s *ChangeRequestStore) SetDecision(ctx context.Context, id int, status model.ChangeRequestStatus, by int, comment string) (bool, error) {
	res, err := s.db.DB.ExecContext(ctx,
		`UPDATE change_requests SET status = ?, decided_by = ?, decided_at = ?, decision_comment = ?
		 WHERE id = ? AND status = 'pending'`,
		status, by, time.Now().UTC().Format(time.RFC3339Nano), comment, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// Withdraw marks a pending request of userID as withdrawn.
func (s *ChangeRequestStore) Withdraw(ctx context.Context, id, userID int) (bool, error) {
	res, err := s.db.DB.ExecContext(ctx,
		`UPDATE change_requests SET status = 'withdrawn', decided_at = ? WHERE id = ? AND user_id = ? AND status = 'pending'`,
		time.Now().UTC().Format(time.RFC3339Nano), id, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// Reopen puts a request back to pending (used when applying an approval failed).
func (s *ChangeRequestStore) Reopen(ctx context.Context, id int) error {
	_, err := s.db.DB.ExecContext(ctx,
		`UPDATE change_requests SET status = 'pending', decided_by = NULL, decided_at = NULL, decision_comment = '' WHERE id = ?`, id)
	return err
}
