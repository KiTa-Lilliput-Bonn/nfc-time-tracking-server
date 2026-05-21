package apipairing

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/model"
)

// StoreSubset is the subset of ApiPairedClientStore needed for bearer checks (tests may stub).
type StoreSubset interface {
	ListAuthorizedSecrets(ctx context.Context) ([]model.ApiPairedClient, error)
}

// Service validates Bearer tokens against stored client secrets.
type Service struct {
	Store StoreSubset
}

func New(s StoreSubset) *Service {
	return &Service{Store: s}
}

// NewID returns a 32-character hex id (16 random bytes), matching the Flutter LAN-API client id style.
func NewID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// NewSecret returns a URL-safe random secret (32 bytes, base64url ohne Padding) für QR-Payload Feld "s".
func NewSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// IsAuthorizedBearer returns true if token matches any active stored secret (constant-time per candidate).
func (s *Service) IsAuthorizedBearer(ctx context.Context, bearerSecret string) bool {
	token := strings.TrimSpace(bearerSecret)
	if token == "" {
		return false
	}
	clients, err := s.Store.ListAuthorizedSecrets(ctx)
	if err != nil || len(clients) == 0 {
		return false
	}
	tb := []byte(token)
	for _, c := range clients {
		sb := []byte(strings.TrimSpace(c.Secret))
		if len(tb) != len(sb) {
			continue
		}
		if subtle.ConstantTimeCompare(tb, sb) == 1 {
			return true
		}
	}
	return false
}

// BuildClient prepares a new client row with timestamps (caller sets Label and Secret).
func BuildClient(id, label, secret string) (*model.ApiPairedClient, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("empty id")
	}
	if strings.TrimSpace(secret) == "" {
		return nil, fmt.Errorf("empty secret")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return &model.ApiPairedClient{
		ID:           id,
		Label:        strings.TrimSpace(label),
		Secret:       strings.TrimSpace(secret),
		CreatedAtUTC: now,
	}, nil
}
