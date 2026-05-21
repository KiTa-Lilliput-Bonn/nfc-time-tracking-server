package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/model"
)

// normalizeWorkDateColumn trims SQLite DATE that may scan as full RFC3339 to YYYY-MM-DD for stable APIs and SQL filters.
func normalizeWorkDateColumn(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, 'T'); i >= 0 {
		return s[:i]
	}
	if i := strings.IndexByte(s, ' '); i >= 0 {
		return s[:i]
	}
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

type WorkPeriodStore struct {
	db *DB
}

func NewWorkPeriodStore(db *DB) *WorkPeriodStore {
	return &WorkPeriodStore{db: db}
}

type effectiveWorkPeriodInterval struct {
	ID  int
	In  time.Time
	Out *time.Time
}

func parseRFC3339Any(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil && !t.IsZero() {
		return t, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}

func intervalsOverlap(aStart time.Time, aEnd *time.Time, bStart time.Time, bEnd *time.Time) bool {
	// Treat nil end as "open-ended" (infinite).
	// Overlap condition: aStart < bEnd && bStart < aEnd.
	var aEndVal time.Time
	aHasEnd := aEnd != nil
	if aHasEnd {
		aEndVal = *aEnd
	}
	var bEndVal time.Time
	bHasEnd := bEnd != nil
	if bHasEnd {
		bEndVal = *bEnd
	}

	if bHasEnd {
		if !aStart.Before(bEndVal) {
			return false
		}
	}
	if aHasEnd {
		if !bStart.Before(aEndVal) {
			return false
		}
	}
	// If either side is open-ended, and the other start is before the open end (always),
	// then overlap is determined by the other inequality above.
	return true
}

func (s *WorkPeriodStore) listEffectiveIntervalsForUserDate(ctx context.Context, userID int, workDate string) ([]effectiveWorkPeriodInterval, error) {
	// Effective interval = latest correction (if any) else original punches.
	// We use MAX(id) as "latest" because corrections are inserted monotonically.
	rows, err := s.db.DB.QueryContext(ctx, `
SELECT
  wp.id,
  COALESCE(tc.corrected_in, wp.punch_in) AS effective_in,
  COALESCE(tc.corrected_out, wp.punch_out) AS effective_out
FROM work_periods wp
LEFT JOIN (
  SELECT t1.work_period_id, t1.corrected_in, t1.corrected_out
  FROM time_corrections t1
  INNER JOIN (
    SELECT work_period_id, MAX(id) AS max_id
    FROM time_corrections
    GROUP BY work_period_id
  ) tmax ON tmax.work_period_id = t1.work_period_id AND tmax.max_id = t1.id
) tc ON tc.work_period_id = wp.id
WHERE wp.user_id = ? AND wp.work_date = ?
ORDER BY effective_in
`, userID, workDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []effectiveWorkPeriodInterval
	for rows.Next() {
		var id int
		var inStr string
		var outStr sql.NullString
		if err := rows.Scan(&id, &inStr, &outStr); err != nil {
			return nil, err
		}
		inT, err := parseRFC3339Any(inStr)
		if err != nil {
			return nil, err
		}
		var outT *time.Time
		if outStr.Valid && outStr.String != "" {
			t, err := parseRFC3339Any(outStr.String)
			if err != nil {
				return nil, err
			}
			outT = &t
		}
		out = append(out, effectiveWorkPeriodInterval{ID: id, In: inT, Out: outT})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *WorkPeriodStore) ReplaceForUserDate(ctx context.Context, userID int, date string, periods []model.WorkPeriod) error {
	tx, err := s.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`DELETE FROM work_periods WHERE user_id = ? AND work_date = ? AND source != 'manual'`,
		userID, date)
	if err != nil {
		return err
	}

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO work_periods (user_id, work_date, punch_in, punch_out, is_break, source) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, wp := range periods {
		wp.Source = ""
		var out interface{}
		if wp.PunchOut != nil {
			out = wp.PunchOut.UTC().Format(time.RFC3339Nano)
		}
		_, err = stmt.ExecContext(ctx, userID, date, wp.PunchIn.UTC().Format(time.RFC3339Nano), out, wp.IsBreak, wp.Source)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *WorkPeriodStore) ListByUserDateRange(ctx context.Context, userID int, from, to string) ([]model.WorkPeriod, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT id, user_id, work_date, punch_in, punch_out, is_break, source FROM work_periods
		 WHERE user_id = ? AND work_date >= ? AND work_date <= ? ORDER BY work_date, punch_in`,
		userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanWorkPeriods(rows)
}

func populateWorkPeriodTimes(wp *model.WorkPeriod, pin string, pout sql.NullString) {
	wp.PunchIn, _ = time.Parse(time.RFC3339Nano, pin)
	if wp.PunchIn.IsZero() {
		wp.PunchIn, _ = time.Parse(time.RFC3339, pin)
	}
	if pout.Valid && pout.String != "" {
		t, _ := time.Parse(time.RFC3339Nano, pout.String)
		if t.IsZero() {
			t, _ = time.Parse(time.RFC3339, pout.String)
		}
		wp.PunchOut = &t
	}
}

func (s *WorkPeriodStore) GetByID(ctx context.Context, id int) (*model.WorkPeriod, error) {
	var wp model.WorkPeriod
	var pin string
	var pout sql.NullString
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, user_id, work_date, punch_in, punch_out, is_break, source FROM work_periods WHERE id = ?`,
		id).Scan(&wp.ID, &wp.UserID, &wp.WorkDate, &pin, &pout, &wp.IsBreak, &wp.Source)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	wp.WorkDate = normalizeWorkDateColumn(wp.WorkDate)
	populateWorkPeriodTimes(&wp, pin, pout)
	return &wp, nil
}

func scanWorkPeriods(rows *sql.Rows) ([]model.WorkPeriod, error) {
	var list []model.WorkPeriod
	for rows.Next() {
		var wp model.WorkPeriod
		var pin string
		var pout sql.NullString
		if err := rows.Scan(&wp.ID, &wp.UserID, &wp.WorkDate, &pin, &pout, &wp.IsBreak, &wp.Source); err != nil {
			return nil, err
		}
		wp.WorkDate = normalizeWorkDateColumn(wp.WorkDate)
		populateWorkPeriodTimes(&wp, pin, pout)
		list = append(list, wp)
	}
	return list, rows.Err()
}

func (s *WorkPeriodStore) CreateManual(ctx context.Context, wp *model.WorkPeriod) error {
	wp.Source = "manual"
	if wp.WorkDate == "" {
		return fmt.Errorf("work_date required")
	}
	if wp.PunchIn.IsZero() {
		return fmt.Errorf("punch_in required")
	}
	if wp.PunchOut == nil {
		return fmt.Errorf("punch_out required")
	}
	if !wp.PunchOut.After(wp.PunchIn) {
		return fmt.Errorf("punch_out must be after punch_in")
	}

	existing, err := s.listEffectiveIntervalsForUserDate(ctx, wp.UserID, wp.WorkDate)
	if err != nil {
		return fmt.Errorf("check overlaps: %w", err)
	}
	newStart := wp.PunchIn.UTC()
	newEnd := wp.PunchOut.UTC()
	for _, ex := range existing {
		// Sanity: ignore any existing open-ended/invalid intervals that don't make sense.
		if ex.Out != nil && !ex.Out.After(ex.In) {
			continue
		}
		if intervalsOverlap(ex.In.UTC(), ptrTimeOrNilUTC(ex.Out), newStart, &newEnd) {
			return fmt.Errorf("der Zeitraum überschneidet sich mit einem vorhandenen Eintrag an diesem Tag")
		}
	}

	var out interface{}
	if wp.PunchOut != nil {
		out = wp.PunchOut.UTC().Format(time.RFC3339Nano)
	}
	res, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO work_periods (user_id, work_date, punch_in, punch_out, is_break, source) VALUES (?, ?, ?, ?, ?, 'manual')`,
		wp.UserID, wp.WorkDate, wp.PunchIn.UTC().Format(time.RFC3339Nano), out, wp.IsBreak)
	if err != nil {
		return fmt.Errorf("create manual work period: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	wp.ID = int(id)
	wp.Source = "manual"
	return nil
}

func ptrTimeOrNilUTC(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func (s *WorkPeriodStore) DeleteManual(ctx context.Context, id int) error {
	res, err := s.db.DB.ExecContext(ctx, `DELETE FROM work_periods WHERE id = ? AND source = 'manual'`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("no manual work period with id %d", id)
	}
	return nil
}
