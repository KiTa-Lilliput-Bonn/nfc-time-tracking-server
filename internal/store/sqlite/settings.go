package sqlite

import (
	"context"
	"database/sql"

	"nfc-time-tracking-server/internal/model"
)

type SettingsStore struct {
	db *DB
}

func NewSettingsStore(db *DB) *SettingsStore {
	return &SettingsStore{db: db}
}

func (s *SettingsStore) Get(ctx context.Context, key string) (string, error) {
	var val string
	err := s.db.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

func (s *SettingsStore) Set(ctx context.Context, key, value string) error {
	_, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
	return err
}

func (s *SettingsStore) GetAll(ctx context.Context) ([]model.Setting, error) {
	rows, err := s.db.DB.QueryContext(ctx, `SELECT key, value FROM settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.Setting
	for rows.Next() {
		var st model.Setting
		if err := rows.Scan(&st.Key, &st.Value); err != nil {
			return nil, err
		}
		list = append(list, st)
	}
	return list, rows.Err()
}
