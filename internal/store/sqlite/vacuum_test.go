package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestVacuumInto_consistentCopy(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "src.db")
	db, err := Open(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS t (id INTEGER PRIMARY KEY, v TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.ExecContext(ctx, `INSERT INTO t (v) VALUES ('hello')`); err != nil {
		t.Fatal(err)
	}

	dstPath := filepath.Join(dir, "dst.db")
	if err := VacuumInto(ctx, db.DB, dstPath); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(dstPath)

	db2, err := Open(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()

	var v string
	if err := db2.DB.QueryRowContext(ctx, `SELECT v FROM t WHERE id=1`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != "hello" {
		t.Fatalf("got %q want hello", v)
	}
}
