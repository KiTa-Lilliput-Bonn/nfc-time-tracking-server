package sqlite

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

type DB struct {
	DB *sql.DB
}

func Open(dsn string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Enable WAL mode and foreign keys
	if _, err := sqlDB.Exec("PRAGMA journal_mode=WAL"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	if _, err := sqlDB.Exec("PRAGMA foreign_keys=ON"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	goose.SetBaseFS(migrations)
	if err := gooseUp(sqlDB); err != nil {
		sqlDB.Close()
		return nil, err
	}

	return &DB{DB: sqlDB}, nil
}

func gooseUp(sqlDB *sql.DB) error {
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.Up(sqlDB, "migrations"); err != nil {
		if errD := goose.SetDialect("sqlite"); errD != nil {
			return fmt.Errorf("run migrations (sqlite3: %w); set sqlite dialect: %w", err, errD)
		}
		if err2 := goose.Up(sqlDB, "migrations"); err2 != nil {
			return fmt.Errorf("run migrations: sqlite3: %w; sqlite: %w", err, err2)
		}
	}
	return nil
}

func (d *DB) Close() error {
	return d.DB.Close()
}
