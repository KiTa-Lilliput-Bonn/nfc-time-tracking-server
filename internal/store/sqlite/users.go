package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"nfc-time-tracking-server/internal/model"
)

type UserStore struct {
	db *DB
}

func NewUserStore(db *DB) *UserStore {
	return &UserStore{db: db}
}

func marshalFixedNonWorkWeekdays(w []int) string {
	if len(w) == 0 {
		return "[]"
	}
	b, err := json.Marshal(w)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func unmarshalFixedNonWorkWeekdays(s string, dest *[]int) error {
	s = strings.TrimSpace(s)
	if s == "" {
		*dest = nil
		return nil
	}
	var out []int
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return err
	}
	*dest = out
	return nil
}

func (s *UserStore) Create(ctx context.Context, u *model.User) error {
	res, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, display_name, role, active, must_change_password) VALUES (?, ?, ?, ?, ?, ?)`,
		u.Username, u.PasswordHash, u.DisplayName, u.Role, u.Active, u.MustChangePassword)
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	u.ID = int(id)
	return nil
}

func scanGroupID(dest **int, gid sql.NullInt64) {
	if gid.Valid {
		v := int(gid.Int64)
		*dest = &v
	} else {
		*dest = nil
	}
}

func scanSSOSubject(u *model.User, sso sql.NullString) {
	u.SSOSubject = ""
	if sso.Valid {
		u.SSOSubject = sso.String
	}
	u.SSOLinked = u.SSOSubject != ""
}

func (s *UserStore) GetByID(ctx context.Context, id int) (*model.User, error) {
	u := &model.User{}
	var gid sql.NullInt64
	var sso sql.NullString
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, username, password_hash, display_name, group_id, role, active, must_change_password, default_team_meeting_participant, opening_hours_balance, opening_vacation_days, created_at, updated_at, sso_subject FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &gid, &u.Role, &u.Active, &u.MustChangePassword, &u.DefaultTeamMeetingParticipant, &u.OpeningHoursBalance, &u.OpeningVacationDays, &u.CreatedAt, &u.UpdatedAt, &sso)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found: %d", id)
	}
	if err != nil {
		return nil, err
	}
	scanGroupID(&u.GroupID, gid)
	scanSSOSubject(u, sso)
	return u, nil
}

func (s *UserStore) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	u := &model.User{}
	var gid sql.NullInt64
	var sso sql.NullString
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, username, password_hash, display_name, group_id, role, active, must_change_password, default_team_meeting_participant, opening_hours_balance, opening_vacation_days, created_at, updated_at, sso_subject FROM users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &gid, &u.Role, &u.Active, &u.MustChangePassword, &u.DefaultTeamMeetingParticipant, &u.OpeningHoursBalance, &u.OpeningVacationDays, &u.CreatedAt, &u.UpdatedAt, &sso)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found: %s", username)
	}
	if err != nil {
		return nil, err
	}
	scanGroupID(&u.GroupID, gid)
	scanSSOSubject(u, sso)
	return u, nil
}

func (s *UserStore) List(ctx context.Context, activeOnly bool) ([]model.User, error) {
	query := `SELECT id, username, password_hash, display_name, group_id, role, active, must_change_password, default_team_meeting_participant, opening_hours_balance, opening_vacation_days, created_at, updated_at, sso_subject FROM users`
	if activeOnly {
		query += ` WHERE active = 1`
	}
	query += ` ORDER BY display_name`

	rows, err := s.db.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []model.User
	for rows.Next() {
		var u model.User
		var gid sql.NullInt64
		var sso sql.NullString
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &gid, &u.Role, &u.Active, &u.MustChangePassword, &u.DefaultTeamMeetingParticipant, &u.OpeningHoursBalance, &u.OpeningVacationDays, &u.CreatedAt, &u.UpdatedAt, &sso); err != nil {
			return nil, err
		}
		scanGroupID(&u.GroupID, gid)
		scanSSOSubject(&u, sso)
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

func (s *UserStore) Update(ctx context.Context, u *model.User) error {
	var gid interface{}
	if u.GroupID != nil {
		gid = *u.GroupID
	}
	_, err := s.db.DB.ExecContext(ctx,
		`UPDATE users SET display_name = ?, group_id = ?, role = ?, active = ?, default_team_meeting_participant = ?, opening_hours_balance = ?, opening_vacation_days = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		u.DisplayName, gid, u.Role, u.Active, u.DefaultTeamMeetingParticipant, u.OpeningHoursBalance, u.OpeningVacationDays, u.ID)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}

func (s *UserStore) SetPassword(ctx context.Context, userID int, passwordHash string, mustChangePassword bool) error {
	_, err := s.db.DB.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, must_change_password = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		passwordHash, mustChangePassword, userID)
	if err != nil {
		return fmt.Errorf("set password: %w", err)
	}
	return nil
}

func (s *UserStore) Count(ctx context.Context) (int, error) {
	var n int
	err := s.db.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// SetCreatedAt setzt das Anlagedatum (nur für Test-Seeding, NFC_TEST_MODE).
func (s *UserStore) SetCreatedAt(ctx context.Context, userID int, createdAt string) error {
	_, err := s.db.DB.ExecContext(ctx, `UPDATE users SET created_at = ? WHERE id = ?`, createdAt, userID)
	return err
}

// GetBySSOSubject liefert den Benutzer, der mit dieser SSO-Kennung ("sub") verknüpft ist.
func (s *UserStore) GetBySSOSubject(ctx context.Context, subject string) (*model.User, error) {
	var id int
	err := s.db.DB.QueryRowContext(ctx, `SELECT id FROM users WHERE sso_subject = ?`, subject).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found for sso subject")
	}
	if err != nil {
		return nil, err
	}
	return s.GetByID(ctx, id)
}

// FindByUsernameFold sucht Benutzer ohne Beachtung der Groß-/Kleinschreibung (ASCII).
// Mehrere Treffer sind möglich, wenn sich Benutzernamen nur in der Schreibweise unterscheiden.
func (s *UserStore) FindByUsernameFold(ctx context.Context, username string) ([]model.User, error) {
	rows, err := s.db.DB.QueryContext(ctx, `SELECT id FROM users WHERE username = ? COLLATE NOCASE ORDER BY id`, username)
	if err != nil {
		return nil, err
	}
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]model.User, 0, len(ids))
	for _, id := range ids {
		u, err := s.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, nil
}

// SetSSOSubject verknüpft den Benutzer mit einer SSO-Kennung; "" löst die Verknüpfung.
func (s *UserStore) SetSSOSubject(ctx context.Context, userID int, subject string) error {
	var v interface{}
	if subject != "" {
		v = subject
	}
	_, err := s.db.DB.ExecContext(ctx,
		`UPDATE users SET sso_subject = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, v, userID)
	if err != nil {
		return fmt.Errorf("set sso subject: %w", err)
	}
	return nil
}
