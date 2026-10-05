package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type Claims struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

type Service struct {
	jwtSecret   []byte
	expiryHours int
	lookup      UserLookup
}

// UserLookup liefert den aktuellen Stand eines Benutzers (aktiv, Rolle) aus der Datenbank.
type UserLookup func(ctx context.Context, userID int) (active bool, role string, err error)

// SetUserLookup aktiviert die Prüfung jedes Tokens gegen den aktuellen Benutzerstand:
// deaktivierte oder gelöschte Konten werden sofort abgewiesen, Rollenänderungen
// greifen sofort statt erst nach Ablauf des Tokens.
func (s *Service) SetUserLookup(f UserLookup) {
	s.lookup = f
}

// ErrUserInactive: Token gültig, aber das Konto ist deaktiviert oder existiert nicht mehr.
var ErrUserInactive = errors.New("user inactive")

// CurrentClaims gleicht die Claims mit dem aktuellen Benutzerstand ab (falls ein
// UserLookup gesetzt ist) und übernimmt die aktuelle Rolle.
func (s *Service) CurrentClaims(ctx context.Context, c *Claims) (*Claims, error) {
	if s.lookup == nil {
		return c, nil
	}
	active, role, err := s.lookup(ctx, c.UserID)
	if err != nil || !active {
		return nil, ErrUserInactive
	}
	out := *c
	out.Role = role
	return &out, nil
}

func New(jwtSecret string, expiryHours int) *Service {
	return &Service{
		jwtSecret:   []byte(jwtSecret),
		expiryHours: expiryHours,
	}
}

// ExpirySeconds returns access token lifetime in seconds.
func (s *Service) ExpirySeconds() int {
	if s.expiryHours <= 0 {
		return 8 * 3600
	}
	return s.expiryHours * 3600
}

func (s *Service) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func (s *Service) CheckPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (s *Service) IssueToken(userID int, username, role string) (string, error) {
	claims := &Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(s.expiryHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func (s *Service) VerifyToken(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

func GenerateRandomPassword(length int) string {
	const charset = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	result := make([]byte, length)
	for i := range result {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		result[i] = charset[n.Int64()]
	}
	return string(result)
}
