package sqlite

import (
	"context"
	"testing"

	"nfc-time-tracking-server/internal/model"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestUserStore_CreateAndGet(t *testing.T) {
	db := setupTestDB(t)
	s := NewUserStore(db)
	ctx := context.Background()

	u := &model.User{
		Username:     "testuser",
		PasswordHash: "$2a$10$fakehash",
		DisplayName:  "Test User",
		Role:         model.RoleUser,
		Active:       true,
	}
	if err := s.Create(ctx, u); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if u.ID == 0 {
		t.Error("expected ID to be set after create")
	}

	got, err := s.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Username != "testuser" {
		t.Errorf("expected username testuser, got %s", got.Username)
	}

	got2, err := s.GetByUsername(ctx, "testuser")
	if err != nil {
		t.Fatalf("GetByUsername failed: %v", err)
	}
	if got2.ID != u.ID {
		t.Errorf("expected ID %d, got %d", u.ID, got2.ID)
	}
}

func TestUserStore_List(t *testing.T) {
	db := setupTestDB(t)
	s := NewUserStore(db)
	ctx := context.Background()

	if err := s.Create(ctx, &model.User{Username: "active", PasswordHash: "x", DisplayName: "A", Role: model.RoleUser, Active: true}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.Create(ctx, &model.User{Username: "inactive", PasswordHash: "x", DisplayName: "B", Role: model.RoleUser, Active: false}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	all, err := s.List(ctx, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 users, got %d", len(all))
	}

	active, err := s.List(ctx, true)
	if err != nil {
		t.Fatalf("List active: %v", err)
	}
	if len(active) != 1 {
		t.Errorf("expected 1 active user, got %d", len(active))
	}
}

func TestUserStore_Update(t *testing.T) {
	db := setupTestDB(t)
	s := NewUserStore(db)
	ctx := context.Background()

	u := &model.User{Username: "updatable", PasswordHash: "x", DisplayName: "Before", Role: model.RoleUser, Active: true}
	if err := s.Create(ctx, u); err != nil {
		t.Fatalf("Create: %v", err)
	}

	u.DisplayName = "After"
	u.Active = false
	if err := s.Update(ctx, u); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, err := s.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.DisplayName != "After" {
		t.Errorf("expected After, got %s", got.DisplayName)
	}
	if got.Active {
		t.Error("expected inactive")
	}
}

func TestUserStore_GetByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	s := NewUserStore(db)
	ctx := context.Background()

	got, err := s.GetByID(ctx, 99999)
	if err == nil {
		t.Fatal("expected error")
	}
	if got != nil {
		t.Errorf("expected nil user, got %+v", got)
	}
}

func TestUserStore_GetByUsername_NotFound(t *testing.T) {
	db := setupTestDB(t)
	s := NewUserStore(db)
	ctx := context.Background()

	got, err := s.GetByUsername(ctx, "nobody")
	if err == nil {
		t.Fatal("expected error")
	}
	if got != nil {
		t.Errorf("expected nil user, got %+v", got)
	}
}
