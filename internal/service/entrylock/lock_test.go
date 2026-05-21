package entrylock

import (
	"testing"
	"time"
)

func TestIsMutable_withinWindow(t *testing.T) {
	created := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	now := created.Add(23 * time.Hour)
	if !IsMutable(created, now) {
		t.Fatal("expected mutable within 24h")
	}
}

func TestIsMutable_exactly24h(t *testing.T) {
	created := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	now := created.Add(MutableWindow)
	if !IsMutable(created, now) {
		t.Fatal("expected mutable at exactly 24h boundary")
	}
}

func TestIsMutable_after24h(t *testing.T) {
	created := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	now := created.Add(MutableWindow + time.Second)
	if IsMutable(created, now) {
		t.Fatal("expected locked after 24h")
	}
}
