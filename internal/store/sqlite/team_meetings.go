package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"nfc-time-tracking-server/internal/model"
)

type TeamMeetingStore struct {
	db *DB
}

func NewTeamMeetingStore(db *DB) *TeamMeetingStore {
	return &TeamMeetingStore{db: db}
}

func (s *TeamMeetingStore) DeleteByWeekAndSource(ctx context.Context, isoYear, isoWeek int, source string) error {
	_, err := s.db.DB.ExecContext(ctx,
		`DELETE FROM team_meetings WHERE iso_week_year = ? AND iso_week = ? AND source = ?`,
		isoYear, isoWeek, source)
	return err
}

func (s *TeamMeetingStore) NextManualSectionIndex(ctx context.Context, isoYear, isoWeek int, kind model.TeamMeetingKind) (int, error) {
	var mx sql.NullInt64
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT MAX(section_index) FROM team_meetings WHERE iso_week_year = ? AND iso_week = ? AND source = 'manual' AND kind = ?`,
		isoYear, isoWeek, string(kind)).Scan(&mx)
	if err != nil {
		return 0, err
	}
	if !mx.Valid {
		return 0, nil
	}
	return int(mx.Int64) + 1, nil
}

func (s *TeamMeetingStore) CreateWithUsers(ctx context.Context, m *model.TeamMeeting) error {
	tx, err := s.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO team_meetings (iso_week_year, iso_week, meeting_date, kind, label, time_start, time_end, source, section_index)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ISOWeekYear, m.ISOWeek, m.MeetingDate, string(m.Kind), m.Label, m.TimeStart, m.TimeEnd, m.Source, m.SectionIndex)
	if err != nil {
		return fmt.Errorf("insert team_meeting: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	m.ID = int(id)
	if err := insertMeetingUsersTx(ctx, tx, m.ID, m.UserIDs); err != nil {
		return err
	}
	return tx.Commit()
}

func insertMeetingUsersTx(ctx context.Context, tx *sql.Tx, meetingID int, userIDs []int) error {
	seen := map[int]struct{}{}
	for _, uid := range userIDs {
		if uid <= 0 {
			continue
		}
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO team_meeting_users (team_meeting_id, user_id) VALUES (?, ?)`,
			meetingID, uid); err != nil {
			return fmt.Errorf("insert team_meeting_user %d: %w", uid, err)
		}
	}
	return nil
}

func (s *TeamMeetingStore) ListByWeek(ctx context.Context, isoYear, isoWeek int) ([]model.TeamMeeting, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT id, iso_week_year, iso_week, meeting_date, kind, label, time_start, time_end, source, section_index
		 FROM team_meetings WHERE iso_week_year = ? AND iso_week = ? ORDER BY section_index, kind`,
		isoYear, isoWeek)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []model.TeamMeeting
	for rows.Next() {
		var m model.TeamMeeting
		var kind string
		if err := rows.Scan(&m.ID, &m.ISOWeekYear, &m.ISOWeek, &m.MeetingDate, &kind, &m.Label, &m.TimeStart, &m.TimeEnd, &m.Source, &m.SectionIndex); err != nil {
			return nil, err
		}
		m.Kind = model.TeamMeetingKind(kind)
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		uids, err := s.listUserIDsForMeeting(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].UserIDs = uids
	}
	return out, nil
}

func (s *TeamMeetingStore) listUserIDsForMeeting(ctx context.Context, meetingID int) ([]int, error) {
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT user_id FROM team_meeting_users WHERE team_meeting_id = ? ORDER BY user_id`, meetingID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListForUserInDateRange returns meetings where meeting_date is in [from, to] (YYYY-MM-DD) and user is assignee.
func (s *TeamMeetingStore) ListForUserInDateRange(ctx context.Context, userID int, from, to string) ([]model.TeamMeeting, error) {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if len(from) < 10 || len(to) < 10 {
		return nil, fmt.Errorf("invalid date range")
	}
	from = from[:10]
	to = to[:10]
	rows, err := s.db.DB.QueryContext(ctx,
		`SELECT tm.id, tm.iso_week_year, tm.iso_week, tm.meeting_date, tm.kind, tm.label, tm.time_start, tm.time_end, tm.source, tm.section_index
		 FROM team_meetings tm
		 INNER JOIN team_meeting_users tmu ON tmu.team_meeting_id = tm.id AND tmu.user_id = ?
		 WHERE date(tm.meeting_date) >= date(?) AND date(tm.meeting_date) <= date(?)
		 ORDER BY tm.meeting_date, tm.section_index, tm.kind`,
		userID, from, to)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []model.TeamMeeting
	for rows.Next() {
		var m model.TeamMeeting
		var kind string
		if err := rows.Scan(&m.ID, &m.ISOWeekYear, &m.ISOWeek, &m.MeetingDate, &kind, &m.Label, &m.TimeStart, &m.TimeEnd, &m.Source, &m.SectionIndex); err != nil {
			return nil, err
		}
		m.Kind = model.TeamMeetingKind(kind)
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		uids, err := s.listUserIDsForMeeting(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].UserIDs = uids
	}
	return out, nil
}

func (s *TeamMeetingStore) GetByID(ctx context.Context, id int) (*model.TeamMeeting, error) {
	var m model.TeamMeeting
	var kind string
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, iso_week_year, iso_week, meeting_date, kind, label, time_start, time_end, source, section_index
		 FROM team_meetings WHERE id = ?`, id).
		Scan(&m.ID, &m.ISOWeekYear, &m.ISOWeek, &m.MeetingDate, &kind, &m.Label, &m.TimeStart, &m.TimeEnd, &m.Source, &m.SectionIndex)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.Kind = model.TeamMeetingKind(kind)
	m.UserIDs, err = s.listUserIDsForMeeting(ctx, m.ID)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ReplaceMeetingAndUsers updates times/source/section and replaces assignees.
func (s *TeamMeetingStore) ReplaceMeetingAndUsers(ctx context.Context, m *model.TeamMeeting) error {
	tx, err := s.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`UPDATE team_meetings SET meeting_date = ?, kind = ?, label = ?, time_start = ?, time_end = ?, source = ?, section_index = ?,
		 iso_week_year = ?, iso_week = ? WHERE id = ?`,
		m.MeetingDate, string(m.Kind), m.Label, m.TimeStart, m.TimeEnd, m.Source, m.SectionIndex,
		m.ISOWeekYear, m.ISOWeek, m.ID); err != nil {
		return fmt.Errorf("update team_meeting: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM team_meeting_users WHERE team_meeting_id = ?`, m.ID); err != nil {
		return err
	}
	if err := insertMeetingUsersTx(ctx, tx, m.ID, m.UserIDs); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *TeamMeetingStore) Delete(ctx context.Context, id int) error {
	_, err := s.db.DB.ExecContext(ctx, `DELETE FROM team_meetings WHERE id = ?`, id)
	return err
}
