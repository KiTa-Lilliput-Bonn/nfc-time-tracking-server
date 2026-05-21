package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store"
)

func TestNFCTagStore_TagUIDForUserAt(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	ns := NewNFCTagStore(db)

	u := &model.User{Username: "u2", PasswordHash: "x", DisplayName: "U2", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := ns.Assign(ctx, &model.NFCTag{TagUID: "TOLD", UserID: u.ID, AssignedFrom: "2026-01-01"}); err != nil {
		t.Fatal(err)
	}
	if err := ns.Assign(ctx, &model.NFCTag{TagUID: "TNEW", UserID: u.ID, AssignedFrom: "2026-06-01"}); err != nil {
		t.Fatal(err)
	}

	at := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	tag, err := ns.TagUIDForUserAt(ctx, u.ID, at)
	if err != nil {
		t.Fatal(err)
	}
	if tag != "TOLD" {
		t.Fatalf("March want TOLD, got %q", tag)
	}
	at2 := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	tag2, err := ns.TagUIDForUserAt(ctx, u.ID, at2)
	if err != nil {
		t.Fatal(err)
	}
	if tag2 != "TNEW" {
		t.Fatalf("July want TNEW, got %q", tag2)
	}
}

func TestNFCTagStore_Assign_rejects_active_other_user(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	ns := NewNFCTagStore(db)

	a := &model.User{Username: "nfc_a", PasswordHash: "x", DisplayName: "Anna Aktiv", Role: model.RoleUser, Active: true}
	b := &model.User{Username: "nfc_b", PasswordHash: "x", DisplayName: "Ben Neu", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := us.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	if err := ns.Assign(ctx, &model.NFCTag{TagUID: "SHARED", UserID: a.ID, AssignedFrom: "2026-01-01"}); err != nil {
		t.Fatal(err)
	}
	err := ns.Assign(ctx, &model.NFCTag{TagUID: "SHARED", UserID: b.ID, AssignedFrom: "2026-06-01"})
	var conflict *store.NFCTagAssignedError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected NFCTagAssignedError, got %v", err)
	}
	if conflict.DisplayName != "Anna Aktiv" {
		t.Fatalf("want owner Anna Aktiv, got %q", conflict.DisplayName)
	}
}

func TestNFCTagStore_Assign_allows_after_owner_inactive(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	ns := NewNFCTagStore(db)

	a := &model.User{Username: "nfc_off", PasswordHash: "x", DisplayName: "Alt", Role: model.RoleUser, Active: true}
	b := &model.User{Username: "nfc_on", PasswordHash: "x", DisplayName: "Neu", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := us.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	if err := ns.Assign(ctx, &model.NFCTag{TagUID: "REUSE", UserID: a.ID, AssignedFrom: "2026-01-01"}); err != nil {
		t.Fatal(err)
	}
	a.Active = false
	if err := us.Update(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := ns.Assign(ctx, &model.NFCTag{TagUID: "REUSE", UserID: b.ID, AssignedFrom: "2026-06-01"}); err != nil {
		t.Fatalf("inactive owner should not block: %v", err)
	}
}
