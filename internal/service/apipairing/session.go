package apipairing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"nfc-time-tracking-server/internal/store"
)

// PairingSessionTTL is how long a QR pairing token remains valid.
const PairingSessionTTL = 15 * time.Minute

var (
	ErrPairingTokenInvalid = errors.New("invalid pairing token")
	ErrPairingTokenUsed    = errors.New("pairing token already used")
	ErrPairingTokenExpired = errors.New("pairing token expired")
)

// SessionService creates and consumes short-lived pairing sessions.
type SessionService struct {
	Sessions store.ApiPairingSessionStore
}

func NewSessionService(s store.ApiPairingSessionStore) *SessionService {
	return &SessionService{Sessions: s}
}

// CreatePairingSession generates a token, stores its hash, and returns the plaintext token.
func (s *SessionService) CreatePairingSession(ctx context.Context, clientID string) (token string, err error) {
	token, err = NewPairingToken()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	expires := now.Add(PairingSessionTTL).Format(time.RFC3339)
	created := now.Format(time.RFC3339)
	if err := s.Sessions.CreateSession(ctx, clientID, HashToken(token), expires, created); err != nil {
		return "", err
	}
	return token, nil
}

// ConsumePairingToken validates and consumes a pairing token; returns the bound client_id.
func (s *SessionService) ConsumePairingToken(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", ErrPairingTokenInvalid
	}
	clientID, err := s.Sessions.ConsumeSession(ctx, HashToken(token))
	if err != nil {
		switch {
		case errors.Is(err, store.ErrPairingSessionConsumed):
			return "", ErrPairingTokenUsed
		case errors.Is(err, store.ErrPairingSessionExpired):
			return "", ErrPairingTokenExpired
		default:
			return "", ErrPairingTokenInvalid
		}
	}
	return clientID, nil
}

// NewPairingToken returns a URL-safe random token (32 bytes, base64url).
func NewPairingToken() (string, error) {
	return NewSecret()
}

// HashToken returns the SHA-256 hex digest of a pairing token for storage.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
