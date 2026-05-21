package audit_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nfc-time-tracking-server/internal/audit"
)

func TestWriteTipFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tip := &audit.Tip{LastID: 42, EventHash: []byte{1, 2, 3}, WrittenAt: time.Now().UTC()}
	if err := audit.WriteTipFile(dir, tip); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "audit-tip.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		LastID    int64  `json:"last_id"`
		EventHash string `json:"event_hash"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.LastID != 42 || doc.EventHash == "" {
		t.Fatalf("doc: %+v", doc)
	}
}
