package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/model"
)

// SQLite DATE / modernc driver may scan as full RFC3339; the API and UI expect YYYY-MM-DD only.
func normalizeScheduleDate(s string) string {
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

type ScheduleStore struct {
	db *DB
}

func NewScheduleStore(db *DB) *ScheduleStore {
	return &ScheduleStore{db: db}
}

func (s *ScheduleStore) Set(ctx context.Context, sch *model.Schedule) error {
	_, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO schedules (user_id, schedule_date, shift_start, shift_end) VALUES (?, ?, ?, ?)
		 ON CONFLICT(user_id, schedule_date) DO UPDATE SET shift_start = excluded.shift_start, shift_end = excluded.shift_end`,
		sch.UserID, sch.ScheduleDate, sch.ShiftStart, sch.ShiftEnd)
	if err != nil {
		return fmt.Errorf("set schedule: %w", err)
	}
	err = s.db.DB.QueryRowContext(ctx,
		`SELECT id FROM schedules WHERE user_id = ? AND date(schedule_date) = date(?)`,
		sch.UserID, sch.ScheduleDate).Scan(&sch.ID)
	if err != nil {
		return fmt.Errorf("schedule id: %w", err)
	}
	return nil
}

func (s *ScheduleStore) GetByID(ctx context.Context, id int) (*model.Schedule, error) {
	sch := &model.Schedule{}
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, user_id, schedule_date, shift_start, shift_end FROM schedules WHERE id = ?`, id).
		Scan(&sch.ID, &sch.UserID, &sch.ScheduleDate, &sch.ShiftStart, &sch.ShiftEnd)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sch.ScheduleDate = normalizeScheduleDate(sch.ScheduleDate)
	return sch, nil
}

func (s *ScheduleStore) GetForUserDate(ctx context.Context, userID int, date string) (*model.Schedule, error) {
	sch := &model.Schedule{}
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, user_id, schedule_date, shift_start, shift_end FROM schedules WHERE user_id = ? AND date(schedule_date) = date(?)`,
		userID, date).Scan(&sch.ID, &sch.UserID, &sch.ScheduleDate, &sch.ShiftStart, &sch.ShiftEnd)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sch.ScheduleDate = normalizeScheduleDate(sch.ScheduleDate)
	return sch, nil
}

// ListByUserDateRange returns schedules for user_id with schedule_date in [from, to] (YYYY-MM-DD, inclusive).
func (s *ScheduleStore) ListByUserDateRange(ctx context.Context, userID int, from, to string) ([]model.Schedule, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT id, user_id, schedule_date, shift_start, shift_end FROM schedules
		 WHERE user_id = ? AND date(schedule_date) >= date(?) AND date(schedule_date) <= date(?)
		 ORDER BY date(schedule_date)`,
		userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.Schedule
	for rows.Next() {
		var sch model.Schedule
		if err := rows.Scan(&sch.ID, &sch.UserID, &sch.ScheduleDate, &sch.ShiftStart, &sch.ShiftEnd); err != nil {
			return nil, err
		}
		sch.ScheduleDate = normalizeScheduleDate(sch.ScheduleDate)
		list = append(list, sch)
	}
	return list, rows.Err()
}

func (s *ScheduleStore) ListByWeek(ctx context.Context, year, week int) ([]model.Schedule, error) {
	from, to, err := isoWeekRange(year, week)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT id, user_id, schedule_date, shift_start, shift_end FROM schedules
		 WHERE date(schedule_date) >= date(?) AND date(schedule_date) <= date(?) ORDER BY date(schedule_date), user_id`,
		from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.Schedule
	for rows.Next() {
		var sch model.Schedule
		if err := rows.Scan(&sch.ID, &sch.UserID, &sch.ScheduleDate, &sch.ShiftStart, &sch.ShiftEnd); err != nil {
			return nil, err
		}
		sch.ScheduleDate = normalizeScheduleDate(sch.ScheduleDate)
		list = append(list, sch)
	}
	return list, rows.Err()
}

func (s *ScheduleStore) Delete(ctx context.Context, id int) error {
	_, err := s.db.DB.ExecContext(ctx, `DELETE FROM schedules WHERE id = ?`, id)
	return err
}

func (s *ScheduleStore) GetWeekNotes(ctx context.Context, isoYear, isoWeek int) (string, error) {
	if isoWeek < 1 || isoWeek > 53 {
		return "", fmt.Errorf("invalid ISO week %d", isoWeek)
	}
	var notes string
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT notes FROM schedule_week_notes WHERE iso_week_year = ? AND iso_week = ?`,
		isoYear, isoWeek).Scan(&notes)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return notes, nil
}

// HTML aus dem Rich-Text-Editor (Quill) kann länger sein als reiner Text
const maxScheduleWeekNotesRunes = 48000

// LastISOWeekWithShift returns the ISO week-year and week of the most recent schedule entry.
func (s *ScheduleStore) LastISOWeekWithShift(ctx context.Context) (year, week int, ok bool, err error) {
	var dateStr string
	err = s.db.DB.QueryRowContext(ctx,
		`SELECT schedule_date FROM schedules ORDER BY date(schedule_date) DESC LIMIT 1`,
	).Scan(&dateStr)
	if err == sql.ErrNoRows {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	dateStr = normalizeScheduleDate(dateStr)
	t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return 0, 0, false, fmt.Errorf("last schedule date: %w", err)
	}
	y, w := t.ISOWeek()
	return y, w, true, nil
}

func (s *ScheduleStore) PutWeekNotes(ctx context.Context, isoYear, isoWeek int, notes string) error {
	if isoWeek < 1 || isoWeek > 53 {
		return fmt.Errorf("invalid ISO week %d", isoWeek)
	}
	runes := []rune(notes)
	if len(runes) > maxScheduleWeekNotesRunes {
		return fmt.Errorf("notes too long (max %d characters)", maxScheduleWeekNotesRunes)
	}
	_, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO schedule_week_notes (iso_week_year, iso_week, notes) VALUES (?, ?, ?)
		 ON CONFLICT(iso_week_year, iso_week) DO UPDATE SET notes = excluded.notes`,
		isoYear, isoWeek, notes)
	return err
}

// isoWeekRange returns Monday–Sunday dates (YYYY-MM-DD) for ISO week-year `year` and ISO week `week`
// in the local timezone. Uses the same “Jan 4” rule as the web UI (ISO 8601), not a scan from Jan 1
// (week 1’s Monday often lies in the previous calendar year).
func isoWeekRange(year, week int) (from, to string, err error) {
	if week < 1 || week > 53 {
		return "", "", fmt.Errorf("invalid ISO week %d", week)
	}
	loc := time.Local
	jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, loc)
	// Distance from jan4 back to Monday of the same ISO week (Sunday=0 … Saturday=6 in Go, same as JS getDay).
	daysBack := (int(jan4.Weekday()) + 6) % 7
	week1Mon := jan4.AddDate(0, 0, -daysBack)
	targetMon := week1Mon.AddDate(0, 0, (week-1)*7)
	y, w := targetMon.ISOWeek()
	if y != year || w != week {
		return "", "", fmt.Errorf("no ISO week %d for ISO year %d", week, year)
	}
	return targetMon.Format("2006-01-02"), targetMon.AddDate(0, 0, 6).Format("2006-01-02"), nil
}

// ValidateScheduleWeekday validates meetingDate (YYYY-MM-DD) is Mo–Fr within the ISO schedule week.
func ValidateScheduleWeekday(year, week int, meetingDate string) error {
	meetingDate = strings.TrimSpace(meetingDate)
	if len(meetingDate) < 10 {
		return fmt.Errorf("meeting_date required")
	}
	meetingDate = meetingDate[:10]
	mon, fri, err := ISOWeekMondayFriday(year, week)
	if err != nil {
		return err
	}
	if meetingDate < mon || meetingDate > fri {
		return fmt.Errorf("meeting_date must be a weekday (Mon–Fri) in ISO week %d/%d", week, year)
	}
	return nil
}

// ISOWeekMondayFriday returns Monday and Friday (YYYY-MM-DD, local) for the ISO week shown in the
// schedule grid (Mo–Fr columns). Uses the same rules as isoWeekRange.
func ISOWeekMondayFriday(year, week int) (monday, friday string, err error) {
	from, _, err := isoWeekRange(year, week)
	if err != nil {
		return "", "", err
	}
	t, err := time.ParseInLocation("2006-01-02", from, time.Local)
	if err != nil {
		return "", "", err
	}
	return from, t.AddDate(0, 0, 4).Format("2006-01-02"), nil
}
