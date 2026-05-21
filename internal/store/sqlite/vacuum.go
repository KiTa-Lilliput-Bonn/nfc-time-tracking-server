package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

const vacuumBusyTimeoutMs = 120000

func escapeSQLStringLiteral(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// VacuumInto writes a consistent standalone copy of the open database to destAbs
// using SQLite VACUUM INTO. destAbs must not exist yet (remove stale files first).
// Uses a dedicated connection with busy_timeout for isolation from other pool settings.
func VacuumInto(ctx context.Context, db *sql.DB, destAbs string) error {
	abs, err := filepath.Abs(destAbs)
	if err != nil {
		return fmt.Errorf("vacuum into: abs dest: %w", err)
	}
	lit := filepath.ToSlash(abs)
	lit = escapeSQLStringLiteral(lit)

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("vacuum into: conn: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "PRAGMA busy_timeout="+strconv.Itoa(vacuumBusyTimeoutMs)); err != nil {
		return fmt.Errorf("vacuum into: busy_timeout: %w", err)
	}
	q := "VACUUM INTO '" + lit + "'"
	if _, err := conn.ExecContext(ctx, q); err != nil {
		return fmt.Errorf("vacuum into: %w", err)
	}
	return nil
}
