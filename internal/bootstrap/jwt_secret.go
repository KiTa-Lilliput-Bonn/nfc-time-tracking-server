package bootstrap

import (
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const jwtSecretFileName = "jwt.secret"

// Older examples used these strings; treat like “unset” so we still persist jwt.secret.
var jwtLegacyPlaceholder = map[string]struct{}{
	"auto-generated-on-first-start": {},
	"change-me-or-use-NFC_AUTH_JWT_SECRET": {},
}

// ResolveJWTSecret returns the JWT signing secret.
// Priority: explicit non-empty configuredSecret (YAML / NFC_AUTH_JWT_SECRET), else contents of jwt.secret
// beside the database file, else a newly generated secret written to that file (0644 parent dir must exist).
func ResolveJWTSecret(configuredSecret, databasePath string) (string, error) {
	s := strings.TrimSpace(configuredSecret)
	if _, legacy := jwtLegacyPlaceholder[s]; legacy {
		s = ""
	}
	if s != "" {
		return s, nil
	}

	dir := filepath.Dir(databasePath)
	path := filepath.Join(dir, jwtSecretFileName)

	if data, err := os.ReadFile(path); err == nil {
		fromFile := strings.TrimSpace(string(data))
		if len(fromFile) >= 32 {
			return fromFile, nil
		}
		log.Printf("auth: %s ignored (too short); regenerating", path)
	}

	b := make([]byte, 32)
	if _, err := crand.Read(b); err != nil {
		return "", fmt.Errorf("generate jwt secret: %w", err)
	}
	secret := hex.EncodeToString(b)
	if err := os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	log.Printf("auth: wrote %s (persisted across restarts; override with auth.jwt_secret or NFC_AUTH_JWT_SECRET)", path)
	return secret, nil
}
