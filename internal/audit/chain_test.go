package audit_test

import (
	"testing"
	"time"

	"nfc-time-tracking-server/internal/audit"
)

func TestGenesisAndChain(t *testing.T) {
	t.Parallel()
	prev := audit.GenesisHash[:]
	var events []audit.Event
	for i := int64(1); i <= 3; i++ {
		ev := audit.Event{
			ID:         i,
			CreatedAt:  time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Second),
			ActorRole:  "superadmin",
			Action:     audit.ActionCreate,
			EntityType: "user",
			EntityID:   "1",
			Summary:    `{"username":"a"}`,
			PrevHash:   prev,
		}
		canonical := audit.Canonical(ev)
		h := audit.ComputeEventHash(prev, canonical)
		ev.EventHash = h[:]
		events = append(events, ev)
		prev = ev.EventHash
	}
	res := audit.VerifyChain(events)
	if !res.OK || !res.GenesisOK || res.Checked != 3 {
		t.Fatalf("verify: %+v", res)
	}
}

func TestVerifyDetectsTamperedHash(t *testing.T) {
	t.Parallel()
	ev := audit.Event{
		ID:         1,
		CreatedAt:  time.Now().UTC(),
		ActorRole:  "leitung",
		Action:     audit.ActionUpdate,
		EntityType: "schedule",
		EntityID:   "5",
		Summary:    `{}`,
		PrevHash:   audit.GenesisHash[:],
		EventHash:  make([]byte, 32),
	}
	res := audit.VerifyChain([]audit.Event{ev})
	if res.OK {
		t.Fatal("expected tampered chain to fail")
	}
	if res.BrokenID == nil || *res.BrokenID != 1 {
		t.Fatalf("broken_id: %+v", res.BrokenID)
	}
}
