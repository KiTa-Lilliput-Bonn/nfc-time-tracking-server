package sqlite

import (
	"context"
	"testing"

	"nfc-time-tracking-server/internal/model"
)

func TestScheduleBoundStore_ListByUser_NormalizesForDateLookup(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	users := NewUserStore(db)
	u := &model.User{
		Username: "sb1", PasswordHash: "x", DisplayName: "SB", Role: model.RoleUser, Active: true,
	}
	if err := users.Create(ctx, u); err != nil {
		t.Fatal(err)
	}

	sb := NewScheduleBoundStore(db)
	if err := sb.Set(ctx, &model.ScheduleBoundSetting{
		UserID: u.ID, ScheduleBound: false, ValidFrom: "2026-06-02",
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := sb.ListByUser(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if model.ScheduleBoundForDate(rows, "2026-06-02") {
		t.Fatalf("expected bound=false for 2026-06-02, rows=%+v", rows)
	}
}
