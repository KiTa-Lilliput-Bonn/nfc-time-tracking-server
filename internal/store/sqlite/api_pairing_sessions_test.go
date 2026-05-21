package sqlite

import (
	"context"
	"testing"
	"time"

	"nfc-time-tracking-server/internal/service/apipairing"
	"nfc-time-tracking-server/internal/store"
)

func TestApiPairingSessionStore_CreateAndConsume(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	sessions := NewApiPairingSessionStore(db)
	svc := apipairing.NewSessionService(sessions)
	ctx := context.Background()

	token, err := svc.CreatePairingSession(ctx, "client1234567890123456789012345678")
	if err != nil {
		t.Fatal(err)
	}

	clientID, err := svc.ConsumePairingToken(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if clientID != "client1234567890123456789012345678" {
		t.Fatalf("client id: got %q", clientID)
	}

	_, err = svc.ConsumePairingToken(ctx, token)
	if err != apipairing.ErrPairingTokenUsed {
		t.Fatalf("second consume: want ErrPairingTokenUsed, got %v", err)
	}
}

func TestApiPairingSessionStore_Expired(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	sessions := NewApiPairingSessionStore(db)
	ctx := context.Background()
	token, _ := apipairing.NewPairingToken()
	hash := apipairing.HashToken(token)
	expired := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	created := time.Now().UTC().Format(time.RFC3339)
	if err := sessions.CreateSession(ctx, "c1", hash, expired, created); err != nil {
		t.Fatal(err)
	}
	svc := apipairing.NewSessionService(sessions)
	_, err = svc.ConsumePairingToken(ctx, token)
	if err != apipairing.ErrPairingTokenExpired {
		t.Fatalf("want expired, got %v", err)
	}
}

func TestApiPairingSessionStore_NotFound(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	sessions := NewApiPairingSessionStore(db)
	_, err = sessions.ConsumeSession(context.Background(), "deadbeef")
	if err != store.ErrPairingSessionNotFound {
		t.Fatalf("want not found, got %v", err)
	}
}
