package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"nfc-time-tracking-server/internal/audit"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func openTestAuditDB(t *testing.T) (*sqlite.DB, *sqlite.AuditStore) {
	t.Helper()
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, sqlite.NewAuditStore(db)
}

func TestAuditAppendAndVerify(t *testing.T) {
	_, store := openTestAuditDB(t)
	ctx := context.Background()
	id, err := store.Append(ctx, audit.Entry{
		ActorRole:  "superadmin",
		Action:     audit.ActionCreate,
		EntityType: "setting",
		EntityID:   "foo",
		Summary:    `{"key":"foo"}`,
	})
	if err != nil || id != 1 {
		t.Fatalf("append: id=%d err=%v", id, err)
	}
	res := store.Verify(ctx)
	if !res.OK || res.Checked != 1 {
		t.Fatalf("verify: %+v", res)
	}
}

func TestAuditTriggersBlockUpdateDelete(t *testing.T) {
	db, store := openTestAuditDB(t)
	ctx := context.Background()
	if _, err := store.Append(ctx, audit.Entry{
		ActorRole: "system", Action: audit.ActionCreate, EntityType: "test", EntityID: "1",
		Summary: "{}",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.ExecContext(ctx, `UPDATE audit_events SET summary = 'x' WHERE id = 1`); err == nil {
		t.Fatal("expected update to fail")
	}
	if _, err := db.DB.ExecContext(ctx, `DELETE FROM audit_events WHERE id = 1`); err == nil {
		t.Fatal("expected delete to fail")
	}
}

func TestAuditPurgeWithAnchor(t *testing.T) {
	db, store := openTestAuditDB(t)
	ctx := context.Background()
	old := time.Now().UTC().AddDate(0, 0, -90)
	_, err := db.DB.ExecContext(ctx, `
		INSERT INTO audit_events (
			id, created_at, actor_user_id, actor_role, action, entity_type, entity_id,
			target_user_id, summary, prev_hash, event_hash
		) VALUES (1, ?, NULL, 'system', 'create', 'test', '1', NULL, '{}', ?, ?)`,
		old.Format(time.RFC3339Nano), audit.GenesisHash[:], audit.GenesisHash[:],
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PurgeOlderThan(ctx, time.Now().UTC().AddDate(0, 0, -60)); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("events after purge: %d", n)
	}
	var anchors int
	if err := db.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_anchors`).Scan(&anchors); err != nil {
		t.Fatal(err)
	}
	if anchors != 1 {
		t.Fatalf("anchors: %d", anchors)
	}
}
