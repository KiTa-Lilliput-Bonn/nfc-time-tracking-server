package model

// ApiPairedClient is a device/LAN-API client paired via shared secret (superadmin-managed).
// Secret is stored in plaintext in SQLite for operational retrieval; only expose via superadmin API.
type ApiPairedClient struct {
	ID            string  `json:"id"`
	Label         string  `json:"label"`
	Secret        string  `json:"secret"`
	CreatedAtUTC  string  `json:"created_at_utc"`
	RevokedAtUTC  *string `json:"revoked_at_utc,omitempty"`
}
