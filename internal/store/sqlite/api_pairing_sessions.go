package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/store"
)

type ApiPairingSessionStore struct {
	db *DB
}

func NewApiPairingSessionStore(db *DB) *ApiPairingSessionStore {
	return &ApiPairingSessionStore{db: db}
}

func (s *ApiPairingSessionStore) CreateSession(ctx context.Context, clientID, tokenHash, expiresAtUTC, createdAtUTC string) error {
	clientID = strings.TrimSpace(clientID)
	tokenHash = strings.TrimSpace(tokenHash)
	if clientID == "" || tokenHash == "" {
		return errors.New("empty client id or token hash")
	}
	_, err := s.db.DB.ExecContext(ctx, `
		INSERT INTO api_pairing_sessions (token_hash, client_id, expires_at_utc, consumed_at_utc, created_at_utc)
		VALUES (?, ?, ?, NULL, ?)
	`, tokenHash, clientID, expiresAtUTC, createdAtUTC)
	return err
}

// ConsumeSession marks a valid session as consumed and returns the client_id.
func (s *ApiPairingSessionStore) ConsumeSession(ctx context.Context, tokenHash string) (string, error) {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return "", store.ErrPairingSessionNotFound
	}

	var clientID, expiresAt, consumed sql.NullString
	err := s.db.DB.QueryRowContext(ctx, `
		SELECT client_id, expires_at_utc, consumed_at_utc
		FROM api_pairing_sessions WHERE token_hash = ?
	`, tokenHash).Scan(&clientID, &expiresAt, &consumed)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", store.ErrPairingSessionNotFound
		}
		return "", err
	}
	if consumed.Valid && strings.TrimSpace(consumed.String) != "" {
		return "", store.ErrPairingSessionConsumed
	}
	exp, err := time.Parse(time.RFC3339, strings.TrimSpace(expiresAt.String))
	if err != nil {
		return "", err
	}
	if time.Now().UTC().After(exp) {
		return "", store.ErrPairingSessionExpired
	}

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.DB.ExecContext(ctx, `
		UPDATE api_pairing_sessions
		SET consumed_at_utc = ?
		WHERE token_hash = ? AND consumed_at_utc IS NULL
	`, now, tokenHash)
	if err != nil {
		return "", err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", store.ErrPairingSessionConsumed
	}
	return strings.TrimSpace(clientID.String), nil
}
