package audit

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type tipFile struct {
	LastID    int64  `json:"last_id"`
	EventHash string `json:"event_hash"`
	WrittenAt string `json:"written_at"`
}

// WriteTipFile writes audit-tip.json into dir for offline chain-head comparison.
func WriteTipFile(dir string, tip *Tip) error {
	if tip == nil {
		return nil
	}
	payload := tipFile{
		LastID:    tip.LastID,
		EventHash: hex.EncodeToString(tip.EventHash),
		WrittenAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "audit-tip.json"), b, 0o644)
}
