package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store"
)

// AttendanceStore speichert Kinder, ihre Anwesenheit, vorab gemeldetes Fehlen und Gruppenaccounts.
type AttendanceStore struct {
	db *DB
}

func NewAttendanceStore(db *DB) *AttendanceStore {
	return &AttendanceStore{db: db}
}

func nullClock(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func clockPtr(v sql.NullString) *string {
	if !v.Valid || v.String == "" {
		return nil
	}
	s := v.String
	return &s
}

// ListChildren liefert die Kinder sortiert nach Vorname, Nachname.
func (s *AttendanceStore) ListChildren(ctx context.Context, includeInactive bool) ([]model.Child, error) {
	q := `SELECT ` + childCols + ` FROM children`
	if !includeInactive {
		q += ` WHERE active = 1`
	}
	q += ` ORDER BY first_name COLLATE NOCASE, last_name COLLATE NOCASE, id`
	rows, err := s.db.DB.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Child{}
	for rows.Next() {
		c, err := scanChild(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (s *AttendanceStore) GetChild(ctx context.Context, id int) (*model.Child, error) {
	c, err := scanChild(s.db.DB.QueryRowContext(ctx, `SELECT `+childCols+` FROM children WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	return c, err
}

const childCols = `id, group_id, first_name, last_name, active, deactivated_at, birth_month`

func scanChild(row interface{ Scan(...any) error }) (*model.Child, error) {
	var c model.Child
	var deact, birth sql.NullString
	if err := row.Scan(&c.ID, &c.GroupID, &c.FirstName, &c.LastName, &c.Active, &deact, &birth); err != nil {
		return nil, err
	}
	c.BirthMonth = clockPtr(birth)
	if deact.Valid && deact.String != "" {
		v := deact.String
		c.DeactivatedAt = &v
	}
	return &c, nil
}

func (s *AttendanceStore) CreateChild(ctx context.Context, c *model.Child) error {
	now := nowText()
	res, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO children (group_id, first_name, last_name, active, birth_month, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.GroupID, c.FirstName, c.LastName, c.Active, nullClock(c.BirthMonth), now, now)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	c.ID = int(id)
	return nil
}

// UpdateChild speichert das Kind; beim Abmelden wird der Zeitpunkt gemerkt (Beginn der Löschfrist),
// beim Wieder-Anmelden entfernt.
func (s *AttendanceStore) UpdateChild(ctx context.Context, c *model.Child) error {
	now := nowText()
	res, err := s.db.DB.ExecContext(ctx,
		`UPDATE children SET group_id = ?, first_name = ?, last_name = ?, active = ?, birth_month = ?, updated_at = ?,
		 deactivated_at = CASE WHEN ? THEN NULL ELSE COALESCE(deactivated_at, ?) END
		 WHERE id = ?`,
		c.GroupID, c.FirstName, c.LastName, c.Active, nullClock(c.BirthMonth), now, c.Active, now, c.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	if c.Active {
		c.DeactivatedAt = nil
	} else if c.DeactivatedAt == nil {
		c.DeactivatedAt = &now
	}
	return nil
}

// DeleteChild löscht ein Kind samt Anwesenheit und Meldungen.
func (s *AttendanceStore) DeleteChild(ctx context.Context, id int) error {
	tx, err := s.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		`DELETE FROM child_attendance WHERE child_id = ?`,
		`DELETE FROM child_notices WHERE child_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM children WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return tx.Commit()
}

// ListAttendance liefert die Einträge eines Tages je Kind.
func (s *AttendanceStore) ListAttendance(ctx context.Context, date string) (map[int]model.ChildAttendance, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT child_id, day, arrived_at, left_at FROM child_attendance WHERE day = ?`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]model.ChildAttendance{}
	for rows.Next() {
		var a model.ChildAttendance
		var in, outT sql.NullString
		if err := rows.Scan(&a.ChildID, &a.Date, &in, &outT); err != nil {
			return nil, err
		}
		a.ArrivedAt, a.LeftAt = clockPtr(in), clockPtr(outT)
		out[a.ChildID] = a
	}
	return out, rows.Err()
}

// PutAttendance speichert Kommen/Gehen; ohne beide Zeiten wird der Eintrag entfernt.
func (s *AttendanceStore) PutAttendance(ctx context.Context, a model.ChildAttendance) error {
	if a.ArrivedAt == nil && a.LeftAt == nil {
		_, err := s.db.DB.ExecContext(ctx, `DELETE FROM child_attendance WHERE child_id = ? AND day = ?`, a.ChildID, a.Date)
		return err
	}
	_, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO child_attendance (child_id, day, arrived_at, left_at, updated_at) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(child_id, day) DO UPDATE SET arrived_at = excluded.arrived_at, left_at = excluded.left_at, updated_at = excluded.updated_at`,
		a.ChildID, a.Date, nullClock(a.ArrivedAt), nullClock(a.LeftAt), nowText())
	return err
}

const noticeCols = `id, child_id, date_from, date_to, reason, arrive_from, leave_at, note`

func scanNotices(rows *sql.Rows) ([]model.ChildNotice, error) {
	defer rows.Close()
	out := []model.ChildNotice{}
	for rows.Next() {
		var n model.ChildNotice
		var reason string
		var af, la sql.NullString
		if err := rows.Scan(&n.ID, &n.ChildID, &n.DateFrom, &n.DateTo, &reason, &af, &la, &n.Note); err != nil {
			return nil, err
		}
		n.Reason = model.ChildNoticeReason(reason)
		n.ArriveFrom, n.LeaveAt = clockPtr(af), clockPtr(la)
		out = append(out, n)
	}
	return out, rows.Err()
}

// ListNoticesForDate liefert alle Meldungen, die date einschließen.
func (s *AttendanceStore) ListNoticesForDate(ctx context.Context, date string) ([]model.ChildNotice, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT `+noticeCols+` FROM child_notices WHERE date_from <= ? AND date_to >= ? ORDER BY date_from, id`, date, date)
	if err != nil {
		return nil, err
	}
	return scanNotices(rows)
}

// ListNoticesEndingFrom liefert alle Meldungen, die am oder nach from enden.
func (s *AttendanceStore) ListNoticesEndingFrom(ctx context.Context, from string) ([]model.ChildNotice, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT `+noticeCols+` FROM child_notices WHERE date_to >= ? ORDER BY date_from, id`, from)
	if err != nil {
		return nil, err
	}
	return scanNotices(rows)
}

// ListNoticesForChild liefert die Meldungen eines Kindes, die am oder nach from enden.
func (s *AttendanceStore) ListNoticesForChild(ctx context.Context, childID int, from string) ([]model.ChildNotice, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT `+noticeCols+` FROM child_notices WHERE child_id = ? AND date_to >= ? ORDER BY date_from, id`, childID, from)
	if err != nil {
		return nil, err
	}
	return scanNotices(rows)
}

func (s *AttendanceStore) GetNotice(ctx context.Context, id int) (*model.ChildNotice, error) {
	rows, err := s.db.DB.QueryContext(ctx, `SELECT `+noticeCols+` FROM child_notices WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	list, err := scanNotices(rows)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, store.ErrNotFound
	}
	return &list[0], nil
}

func (s *AttendanceStore) CreateNotice(ctx context.Context, n *model.ChildNotice) error {
	now := nowText()
	res, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO child_notices (child_id, date_from, date_to, reason, arrive_from, leave_at, note, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ChildID, n.DateFrom, n.DateTo, string(n.Reason), nullClock(n.ArriveFrom), nullClock(n.LeaveAt), n.Note, now, now)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	n.ID = int(id)
	return nil
}

func (s *AttendanceStore) UpdateNotice(ctx context.Context, n *model.ChildNotice) error {
	res, err := s.db.DB.ExecContext(ctx,
		`UPDATE child_notices SET date_from = ?, date_to = ?, reason = ?, arrive_from = ?, leave_at = ?, note = ?, updated_at = ?
		 WHERE id = ?`,
		n.DateFrom, n.DateTo, string(n.Reason), nullClock(n.ArriveFrom), nullClock(n.LeaveAt), n.Note, nowText(), n.ID)
	if err != nil {
		return err
	}
	if c, _ := res.RowsAffected(); c == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *AttendanceStore) DeleteNotice(ctx context.Context, id int) error {
	res, err := s.db.DB.ExecContext(ctx, `DELETE FROM child_notices WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if c, _ := res.RowsAffected(); c == 0 {
		return store.ErrNotFound
	}
	return nil
}

// ListPresentStaff liefert die IDs aktiver Benutzer, die an date eingestempelt und noch nicht
// ausgestempelt sind. Blöcke mit Korrektur (immer mit Ende) oder deaktivierter Korrektur zählen nicht.
func (s *AttendanceStore) ListPresentStaff(ctx context.Context, date string) ([]int, error) {
	rows, err := s.db.DB.QueryContext(ctx, `
SELECT DISTINCT wp.user_id
FROM work_periods wp
JOIN users u ON u.id = wp.user_id AND u.active = 1
WHERE wp.work_date = ? AND wp.punch_out IS NULL AND wp.is_break = 0
  AND NOT EXISTS (SELECT 1 FROM time_corrections tc WHERE tc.work_period_id = wp.id)
ORDER BY wp.user_id`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

const groupAccountCols = `id, group_id, username, password_hash, active, session_version`

func (s *AttendanceStore) scanGroupAccount(row *sql.Row) (*model.GroupAccount, error) {
	var a model.GroupAccount
	err := row.Scan(&a.ID, &a.GroupID, &a.Username, &a.PasswordHash, &a.Active, &a.SessionVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *AttendanceStore) ListGroupAccounts(ctx context.Context) ([]model.GroupAccount, error) {
	rows, err := s.db.DB.QueryContext(ctx, `SELECT `+groupAccountCols+` FROM group_accounts ORDER BY group_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.GroupAccount{}
	for rows.Next() {
		var a model.GroupAccount
		if err := rows.Scan(&a.ID, &a.GroupID, &a.Username, &a.PasswordHash, &a.Active, &a.SessionVersion); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *AttendanceStore) GetGroupAccount(ctx context.Context, id int) (*model.GroupAccount, error) {
	return s.scanGroupAccount(s.db.DB.QueryRowContext(ctx, `SELECT `+groupAccountCols+` FROM group_accounts WHERE id = ?`, id))
}

func (s *AttendanceStore) GetGroupAccountByGroup(ctx context.Context, groupID int) (*model.GroupAccount, error) {
	return s.scanGroupAccount(s.db.DB.QueryRowContext(ctx, `SELECT `+groupAccountCols+` FROM group_accounts WHERE group_id = ?`, groupID))
}

// GetGroupAccountByUsername sucht ohne Beachtung der Groß-/Kleinschreibung.
func (s *AttendanceStore) GetGroupAccountByUsername(ctx context.Context, username string) (*model.GroupAccount, error) {
	return s.scanGroupAccount(s.db.DB.QueryRowContext(ctx,
		`SELECT `+groupAccountCols+` FROM group_accounts WHERE username = ? COLLATE NOCASE`, strings.TrimSpace(username)))
}

func (s *AttendanceStore) CreateGroupAccount(ctx context.Context, a *model.GroupAccount) error {
	now := nowText()
	res, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO group_accounts (group_id, username, password_hash, active, session_version, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 1, ?, ?)`,
		a.GroupID, a.Username, a.PasswordHash, a.Active, now, now)
	if err != nil {
		return fmt.Errorf("create group account: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	a.ID = int(id)
	a.SessionVersion = 1
	return nil
}

// UpdateGroupAccount ändert Benutzername und Aktiv-Status.
func (s *AttendanceStore) UpdateGroupAccount(ctx context.Context, a *model.GroupAccount) error {
	_, err := s.db.DB.ExecContext(ctx,
		`UPDATE group_accounts SET username = ?, active = ?, updated_at = ? WHERE id = ?`,
		a.Username, a.Active, nowText(), a.ID)
	return err
}

// SetGroupAccountPassword setzt das Passwort und meldet alle angemeldeten Geräte ab.
func (s *AttendanceStore) SetGroupAccountPassword(ctx context.Context, id int, hash string) error {
	_, err := s.db.DB.ExecContext(ctx,
		`UPDATE group_accounts SET password_hash = ?, session_version = session_version + 1, updated_at = ? WHERE id = ?`,
		hash, nowText(), id)
	return err
}

func (s *AttendanceStore) DeleteGroupAccount(ctx context.Context, id int) error {
	_, err := s.db.DB.ExecContext(ctx, `DELETE FROM group_accounts WHERE id = ?`, id)
	return err
}

// PurgeChildData löscht Kinderdaten nach den Löschfristen. Kommen/Gehen wird vorher anonym gezählt
// (Kinder je Gruppe, Tag und halber Stunde) und zu child_attendance_stats addiert. Da Zählen und Löschen
// in einer Transaktion geschehen, wird jeder Eintrag genau einmal gezählt.
func (s *AttendanceStore) PurgeChildData(ctx context.Context, p store.ChildPurge) (store.ChildPurgeResult, error) {
	var res store.ChildPurgeResult
	tx, err := s.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	// Abgemeldete Kinder nach Ablauf der Frist.
	var gone []int
	rows, err := tx.QueryContext(ctx, `SELECT id, deactivated_at FROM children WHERE active = 0 AND deactivated_at IS NOT NULL`)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var id int
		var at string
		if err := rows.Scan(&id, &at); err != nil {
			rows.Close()
			return res, err
		}
		if t := parseText(at); !t.IsZero() && t.Before(p.InactiveBefore) {
			gone = append(gone, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}
	goneSet := map[int]bool{}
	for _, id := range gone {
		goneSet[id] = true
	}

	// Kommen/Gehen: alte Tage und alle Tage der zu löschenden Kinder.
	type key struct {
		group int
		day   string
		slot  string
	}
	counts := map[key]int{}
	type rowKey struct {
		child int
		day   string
	}
	var drop []rowKey
	rows, err = tx.QueryContext(ctx,
		`SELECT a.child_id, c.group_id, a.day, a.arrived_at, a.left_at, c.active
		 FROM child_attendance a JOIN children c ON c.id = a.child_id
		 WHERE a.day < ? OR c.active = 0`, p.TimesBefore)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var child, group int
		var day string
		var in, out sql.NullString
		var active bool
		if err := rows.Scan(&child, &group, &day, &in, &out, &active); err != nil {
			rows.Close()
			return res, err
		}
		if day >= p.TimesBefore && !goneSet[child] {
			continue
		}
		drop = append(drop, rowKey{child, day})
		if !in.Valid || in.String == "" {
			continue
		}
		counts[key{group, day, ""}]++
		for _, slot := range attendanceSlots(in.String, out.String) {
			counts[key{group, day, slot}]++
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}
	for k, n := range counts {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO child_attendance_stats (group_id, day, slot, children) VALUES (?, ?, ?, ?)
			 ON CONFLICT(group_id, day, slot) DO UPDATE SET children = children + excluded.children`,
			k.group, k.day, k.slot, n); err != nil {
			return res, err
		}
	}
	for _, d := range drop {
		if _, err := tx.ExecContext(ctx, `DELETE FROM child_attendance WHERE child_id = ? AND day = ?`, d.child, d.day); err != nil {
			return res, err
		}
	}
	res.AttendanceDays = len(drop)

	r, err := tx.ExecContext(ctx, `DELETE FROM child_notices WHERE date_to < ?`, p.NoticesBefore)
	if err != nil {
		return res, err
	}
	n, _ := r.RowsAffected()
	res.Notices = int(n)
	for _, id := range gone {
		r, err := tx.ExecContext(ctx, `DELETE FROM child_notices WHERE child_id = ?`, id)
		if err != nil {
			return res, err
		}
		n, _ := r.RowsAffected()
		res.Notices += int(n)
		if _, err := tx.ExecContext(ctx, `DELETE FROM children WHERE id = ?`, id); err != nil {
			return res, err
		}
	}
	res.Children = len(gone)
	return res, tx.Commit()
}

// attendanceSlots liefert die halben Stunden (Beginn HH:MM), in denen ein Kind von in bis out da war.
// Ohne Gehen-Zeit zählt nur die halbe Stunde des Kommens.
func attendanceSlots(in, out string) []string {
	start, ok := clockMinutes(in)
	if !ok {
		return nil
	}
	end, ok := clockMinutes(out)
	if !ok || end <= start {
		end = start + 1
	}
	var slots []string
	for m := start - start%30; m < end; m += 30 {
		slots = append(slots, fmt.Sprintf("%02d:%02d", m/60, m%60))
	}
	return slots
}

func clockMinutes(s string) (int, bool) {
	var h, m int
	if len(s) != 5 {
		return 0, false
	}
	if _, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}
