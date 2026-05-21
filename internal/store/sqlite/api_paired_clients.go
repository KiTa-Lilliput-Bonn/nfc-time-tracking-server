package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"nfc-time-tracking-server/internal/model"
)

type ApiPairedClientStore struct {
	db *DB
}

func NewApiPairedClientStore(db *DB) *ApiPairedClientStore {
	return &ApiPairedClientStore{db: db}
}

func (s *ApiPairedClientStore) Insert(ctx context.Context, c *model.ApiPairedClient) error {
	if strings.TrimSpace(c.ID) == "" {
		return errors.New("empty id")
	}
	_, err := s.db.DB.ExecContext(ctx, `
		INSERT INTO api_paired_clients (id, label, secret, created_at_utc, revoked_at_utc)
		VALUES (?, ?, ?, ?, NULL)
	`, c.ID, c.Label, c.Secret, c.CreatedAtUTC)
	return err
}

func (s *ApiPairedClientStore) GetByID(ctx context.Context, id string) (*model.ApiPairedClient, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, sql.ErrNoRows
	}
	var c model.ApiPairedClient
	var revoked sql.NullString
	err := s.db.DB.QueryRowContext(ctx, `
		SELECT id, label, secret, created_at_utc, revoked_at_utc
		FROM api_paired_clients WHERE id = ?
	`, id).Scan(&c.ID, &c.Label, &c.Secret, &c.CreatedAtUTC, &revoked)
	if err != nil {
		return nil, err
	}
	if revoked.Valid && revoked.String != "" {
		v := revoked.String
		c.RevokedAtUTC = &v
	}
	return &c, nil
}

func (s *ApiPairedClientStore) List(ctx context.Context) ([]model.ApiPairedClient, error) {
	rows, err := s.db.DB.QueryContext(ctx, `
		SELECT id, label, secret, created_at_utc, revoked_at_utc
		FROM api_paired_clients
		ORDER BY created_at_utc DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.ApiPairedClient
	for rows.Next() {
		var c model.ApiPairedClient
		var revoked sql.NullString
		if err := rows.Scan(&c.ID, &c.Label, &c.Secret, &c.CreatedAtUTC, &revoked); err != nil {
			return nil, err
		}
		if revoked.Valid && revoked.String != "" {
			v := revoked.String
			c.RevokedAtUTC = &v
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *ApiPairedClientStore) ListAuthorizedSecrets(ctx context.Context) ([]model.ApiPairedClient, error) {
	rows, err := s.db.DB.QueryContext(ctx, `
		SELECT id, label, secret, created_at_utc, revoked_at_utc
		FROM api_paired_clients
		WHERE revoked_at_utc IS NULL AND trim(secret) != ''
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.ApiPairedClient
	for rows.Next() {
		var c model.ApiPairedClient
		var revoked sql.NullString
		if err := rows.Scan(&c.ID, &c.Label, &c.Secret, &c.CreatedAtUTC, &revoked); err != nil {
			return nil, err
		}
		if revoked.Valid && revoked.String != "" {
			v := revoked.String
			c.RevokedAtUTC = &v
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *ApiPairedClientStore) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return sql.ErrNoRows
	}
	res, err := s.db.DB.ExecContext(ctx, `DELETE FROM api_paired_clients WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
