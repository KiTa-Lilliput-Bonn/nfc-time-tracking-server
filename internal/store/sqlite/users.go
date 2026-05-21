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
		`INSERT INTO users (username, password_hash, display_name, role, active, must_change_password, fixed_non_work_weekdays) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		u.Username, u.PasswordHash, u.DisplayName, u.Role, u.Active, u.MustChangePassword, marshalFixedNonWorkWeekdays(u.FixedNonWorkWeekdays))
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

func (s *UserStore) GetByID(ctx context.Context, id int) (*model.User, error) {
	u := &model.User{}
	var gid sql.NullInt64
	var fnw string
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, username, password_hash, display_name, group_id, role, active, must_change_password, opening_hours_balance, opening_vacation_days, fixed_non_work_weekdays, created_at, updated_at FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &gid, &u.Role, &u.Active, &u.MustChangePassword, &u.OpeningHoursBalance, &u.OpeningVacationDays, &fnw, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found: %d", id)
	}
	if err != nil {
		return nil, err
	}
	scanGroupID(&u.GroupID, gid)
	if err := unmarshalFixedNonWorkWeekdays(fnw, &u.FixedNonWorkWeekdays); err != nil {
		return nil, fmt.Errorf("user %d fixed_non_work_weekdays: %w", id, err)
	}
	return u, nil
}

func (s *UserStore) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	u := &model.User{}
	var gid sql.NullInt64
	var fnw string
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, username, password_hash, display_name, group_id, role, active, must_change_password, opening_hours_balance, opening_vacation_days, fixed_non_work_weekdays, created_at, updated_at FROM users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &gid, &u.Role, &u.Active, &u.MustChangePassword, &u.OpeningHoursBalance, &u.OpeningVacationDays, &fnw, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found: %s", username)
	}
	if err != nil {
		return nil, err
	}
	scanGroupID(&u.GroupID, gid)
	if err := unmarshalFixedNonWorkWeekdays(fnw, &u.FixedNonWorkWeekdays); err != nil {
		return nil, fmt.Errorf("user %s fixed_non_work_weekdays: %w", username, err)
	}
	return u, nil
}

func (s *UserStore) List(ctx context.Context, activeOnly bool) ([]model.User, error) {
	query := `SELECT id, username, password_hash, display_name, group_id, role, active, must_change_password, opening_hours_balance, opening_vacation_days, fixed_non_work_weekdays, created_at, updated_at FROM users`
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
		var fnw string
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &gid, &u.Role, &u.Active, &u.MustChangePassword, &u.OpeningHoursBalance, &u.OpeningVacationDays, &fnw, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		scanGroupID(&u.GroupID, gid)
		if err := unmarshalFixedNonWorkWeekdays(fnw, &u.FixedNonWorkWeekdays); err != nil {
			return nil, fmt.Errorf("user %d fixed_non_work_weekdays: %w", u.ID, err)
		}
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
		`UPDATE users SET display_name = ?, group_id = ?, role = ?, active = ?, opening_hours_balance = ?, opening_vacation_days = ?, fixed_non_work_weekdays = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		u.DisplayName, gid, u.Role, u.Active, u.OpeningHoursBalance, u.OpeningVacationDays, marshalFixedNonWorkWeekdays(u.FixedNonWorkWeekdays), u.ID)
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
