package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"nfc-time-tracking-server/internal/model"
)

type GroupCashStore struct {
	db *DB
}

func NewGroupCashStore(db *DB) *GroupCashStore {
	return &GroupCashStore{db: db}
}

func nowText() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func parseText(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

func optInt(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

func intArg(p *int) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

func (s *GroupCashStore) ListKeepers(ctx context.Context, groupID int) ([]int, error) {
	return s.queryInts(ctx, `SELECT user_id FROM cash_keepers WHERE group_id = ? ORDER BY user_id`, groupID)
}

func (s *GroupCashStore) ListKeeperGroups(ctx context.Context, userID int) ([]int, error) {
	return s.queryInts(ctx, `SELECT group_id FROM cash_keepers WHERE user_id = ? ORDER BY group_id`, userID)
}

func (s *GroupCashStore) queryInts(ctx context.Context, q string, args ...any) ([]int, error) {
	rows, err := s.db.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *GroupCashStore) IsKeeper(ctx context.Context, groupID, userID int) (bool, error) {
	var n int
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM cash_keepers WHERE group_id = ? AND user_id = ?`, groupID, userID).Scan(&n)
	return n > 0, err
}

func (s *GroupCashStore) SetKeepers(ctx context.Context, groupID int, userIDs []int) error {
	tx, err := s.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM cash_keepers WHERE group_id = ?`, groupID); err != nil {
		return fmt.Errorf("clear keepers: %w", err)
	}
	now := nowText()
	for _, uid := range userIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO cash_keepers (group_id, user_id, created_at) VALUES (?, ?, ?)`, groupID, uid, now); err != nil {
			return fmt.Errorf("insert keeper: %w", err)
		}
	}
	return tx.Commit()
}

func (s *GroupCashStore) GetOpening(ctx context.Context, groupID int) (*model.CashOpening, error) {
	var o model.CashOpening
	var by sql.NullInt64
	var updated string
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT group_id, opening_date, cash_cents, savings_cents, updated_by, updated_at FROM cash_openings WHERE group_id = ?`,
		groupID).Scan(&o.GroupID, &o.Date, &o.CashCents, &o.SavingsCents, &by, &updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	o.UpdatedBy = optInt(by)
	o.UpdatedAt = parseText(updated)
	return &o, nil
}

func (s *GroupCashStore) SetOpening(ctx context.Context, o *model.CashOpening) error {
	o.UpdatedAt = time.Now().UTC()
	_, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO cash_openings (group_id, opening_date, cash_cents, savings_cents, updated_by, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (group_id) DO UPDATE SET opening_date = excluded.opening_date, cash_cents = excluded.cash_cents,
		   savings_cents = excluded.savings_cents, updated_by = excluded.updated_by, updated_at = excluded.updated_at`,
		o.GroupID, o.Date, o.CashCents, o.SavingsCents, intArg(o.UpdatedBy), o.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("set opening: %w", err)
	}
	return nil
}

func (s *GroupCashStore) DeleteOpening(ctx context.Context, groupID int) error {
	_, err := s.db.DB.ExecContext(ctx, `DELETE FROM cash_openings WHERE group_id = ?`, groupID)
	return err
}

func (s *GroupCashStore) ListAllowances(ctx context.Context, groupID int) ([]model.CashAllowance, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT id, group_id, valid_from, amount_cents, created_by, created_at FROM cash_allowances
		 WHERE group_id = ? ORDER BY valid_from ASC`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CashAllowance
	for rows.Next() {
		var a model.CashAllowance
		var by sql.NullInt64
		var created string
		if err := rows.Scan(&a.ID, &a.GroupID, &a.ValidFrom, &a.AmountCents, &by, &created); err != nil {
			return nil, err
		}
		a.CreatedBy = optInt(by)
		a.CreatedAt = parseText(created)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *GroupCashStore) UpsertAllowance(ctx context.Context, a *model.CashAllowance) error {
	a.CreatedAt = time.Now().UTC()
	_, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO cash_allowances (group_id, valid_from, amount_cents, created_by, created_at) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (group_id, valid_from) DO UPDATE SET amount_cents = excluded.amount_cents,
		   created_by = excluded.created_by, created_at = excluded.created_at`,
		a.GroupID, a.ValidFrom, a.AmountCents, intArg(a.CreatedBy), a.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("upsert allowance: %w", err)
	}
	return s.db.DB.QueryRowContext(ctx,
		`SELECT id FROM cash_allowances WHERE group_id = ? AND valid_from = ?`, a.GroupID, a.ValidFrom).Scan(&a.ID)
}

func (s *GroupCashStore) DeleteAllowance(ctx context.Context, groupID, id int) (bool, error) {
	res, err := s.db.DB.ExecContext(ctx, `DELETE FROM cash_allowances WHERE id = ? AND group_id = ?`, id, groupID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

const cashEntryColumns = `id, group_id, kind, source, for_month, entry_date, amount_cents, description,
	created_by, created_at, updated_by, updated_at`

func scanCashEntry(row rowScanner) (*model.CashEntry, error) {
	var e model.CashEntry
	var cby, uby sql.NullInt64
	var cat, uat string
	if err := row.Scan(&e.ID, &e.GroupID, &e.Kind, &e.Source, &e.ForMonth, &e.EntryDate, &e.AmountCents,
		&e.Description, &cby, &cat, &uby, &uat); err != nil {
		return nil, err
	}
	e.CreatedBy = optInt(cby)
	e.UpdatedBy = optInt(uby)
	e.CreatedAt = parseText(cat)
	e.UpdatedAt = parseText(uat)
	return &e, nil
}

func (s *GroupCashStore) ListEntries(ctx context.Context, groupID int) ([]model.CashEntry, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT `+cashEntryColumns+` FROM cash_entries WHERE group_id = ? ORDER BY entry_date ASC, id ASC`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CashEntry
	for rows.Next() {
		e, err := scanCashEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (s *GroupCashStore) GetEntry(ctx context.Context, id int) (*model.CashEntry, error) {
	e, err := scanCashEntry(s.db.DB.QueryRowContext(ctx, `SELECT `+cashEntryColumns+` FROM cash_entries WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return e, err
}

func (s *GroupCashStore) CreateEntry(ctx context.Context, e *model.CashEntry) error {
	now := time.Now().UTC()
	e.CreatedAt, e.UpdatedAt = now, now
	e.UpdatedBy = e.CreatedBy
	ts := now.Format(time.RFC3339Nano)
	res, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO cash_entries (group_id, kind, source, for_month, entry_date, amount_cents, description,
			created_by, created_at, updated_by, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.GroupID, e.Kind, e.Source, e.ForMonth, e.EntryDate, e.AmountCents, e.Description,
		intArg(e.CreatedBy), ts, intArg(e.UpdatedBy), ts)
	if err != nil {
		return fmt.Errorf("create cash entry: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	e.ID = int(id)
	return nil
}

func (s *GroupCashStore) UpdateEntry(ctx context.Context, e *model.CashEntry) error {
	e.UpdatedAt = time.Now().UTC()
	_, err := s.db.DB.ExecContext(ctx,
		`UPDATE cash_entries SET kind = ?, source = ?, for_month = ?, entry_date = ?, amount_cents = ?, description = ?,
			updated_by = ?, updated_at = ? WHERE id = ?`,
		e.Kind, e.Source, e.ForMonth, e.EntryDate, e.AmountCents, e.Description,
		intArg(e.UpdatedBy), e.UpdatedAt.Format(time.RFC3339Nano), e.ID)
	if err != nil {
		return fmt.Errorf("update cash entry: %w", err)
	}
	return nil
}

func (s *GroupCashStore) DeleteEntry(ctx context.Context, id int) error {
	tx, err := s.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM cash_receipts WHERE entry_id = ?`, id); err != nil {
		return fmt.Errorf("delete receipts: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM cash_entries WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete cash entry: %w", err)
	}
	return tx.Commit()
}

const cashReceiptColumns = `r.id, r.entry_id, r.filename, r.content_type, r.size_bytes, r.uploaded_by, r.created_at`

func scanCashReceipt(row rowScanner) (*model.CashReceipt, error) {
	var c model.CashReceipt
	var by sql.NullInt64
	var created string
	if err := row.Scan(&c.ID, &c.EntryID, &c.Filename, &c.ContentType, &c.SizeBytes, &by, &created); err != nil {
		return nil, err
	}
	c.UploadedBy = optInt(by)
	c.CreatedAt = parseText(created)
	return &c, nil
}

func (s *GroupCashStore) ListReceipts(ctx context.Context, groupID int) ([]model.CashReceipt, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT `+cashReceiptColumns+` FROM cash_receipts r JOIN cash_entries e ON e.id = r.entry_id
		 WHERE e.group_id = ? ORDER BY r.id ASC`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CashReceipt
	for rows.Next() {
		c, err := scanCashReceipt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (s *GroupCashStore) GetReceipt(ctx context.Context, id int) (*model.CashReceipt, error) {
	c, err := scanCashReceipt(s.db.DB.QueryRowContext(ctx, `SELECT `+cashReceiptColumns+` FROM cash_receipts r WHERE r.id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

func (s *GroupCashStore) ReceiptData(ctx context.Context, id int) ([]byte, error) {
	var data []byte
	err := s.db.DB.QueryRowContext(ctx, `SELECT data FROM cash_receipts WHERE id = ?`, id).Scan(&data)
	return data, err
}

func (s *GroupCashStore) CreateReceipt(ctx context.Context, r *model.CashReceipt, data []byte) error {
	r.CreatedAt = time.Now().UTC()
	r.SizeBytes = int64(len(data))
	res, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO cash_receipts (entry_id, filename, content_type, size_bytes, data, uploaded_by, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.EntryID, r.Filename, r.ContentType, r.SizeBytes, data, intArg(r.UploadedBy), r.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("create receipt: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	r.ID = int(id)
	return nil
}

func (s *GroupCashStore) DeleteReceipt(ctx context.Context, id int) error {
	_, err := s.db.DB.ExecContext(ctx, `DELETE FROM cash_receipts WHERE id = ?`, id)
	return err
}
