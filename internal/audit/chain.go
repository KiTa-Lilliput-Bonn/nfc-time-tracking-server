package audit

import (
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const genesisSeed = "nfc-audit-genesis"

// GenesisHash is the prev_hash for the first event in the chain.
var GenesisHash = sha256.Sum256([]byte(genesisSeed))

// Canonical builds the stable payload hashed into each event.
func Canonical(e Event) []byte {
	actor := ""
	if e.ActorUserID != nil {
		actor = strconv.Itoa(*e.ActorUserID)
	}
	target := ""
	if e.TargetUserID != nil {
		target = strconv.Itoa(*e.TargetUserID)
	}
	summaryDigest := sha256.Sum256([]byte(e.Summary))
	parts := []string{
		strconv.FormatInt(e.ID, 10),
		e.CreatedAt.UTC().Format(time.RFC3339Nano),
		actor,
		e.ActorRole,
		e.Action,
		e.EntityType,
		e.EntityID,
		target,
		fmt.Sprintf("%x", summaryDigest),
	}
	return []byte(strings.Join(parts, "|"))
}

// ComputeEventHash returns SHA256(prevHash || canonical).
func ComputeEventHash(prevHash []byte, canonical []byte) [32]byte {
	h := sha256.New()
	h.Write(prevHash)
	h.Write(canonical)
	return sha256.Sum256(h.Sum(nil))
}

// VerifyChain checks prev_hash linkage and event_hash for each event in order.
func VerifyChain(events []Event) VerifyResult {
	if len(events) == 0 {
		return VerifyResult{OK: true, Checked: 0, GenesisOK: true}
	}
	expectedPrev := GenesisHash[:]
	for _, e := range events {
		if len(e.PrevHash) != 32 {
			id := e.ID
			return VerifyResult{OK: false, Checked: 0, BrokenID: &id, GenesisOK: false}
		}
		if !equalBytes(expectedPrev, e.PrevHash) {
			id := e.ID
			return VerifyResult{OK: false, Checked: int(e.ID - events[0].ID), BrokenID: &id, GenesisOK: e.ID == events[0].ID}
		}
		canonical := Canonical(e)
		want := ComputeEventHash(e.PrevHash, canonical)
		if !equalBytes(want[:], e.EventHash) {
			id := e.ID
			return VerifyResult{OK: false, Checked: int(e.ID - events[0].ID), BrokenID: &id, GenesisOK: true}
		}
		expectedPrev = e.EventHash
	}
	genesisOK := equalBytes(events[0].PrevHash, GenesisHash[:])
	return VerifyResult{OK: true, Checked: len(events), GenesisOK: genesisOK}
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
