package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

// OpenSQLite opens the SQLite database at dbPath, pings it, and runs ApplyMigrations.
// The DSN uses WAL and a busy timeout to reduce "database is locked" under concurrent readers.
// MaxOpenConns(1) matches SQLite’s typical single-writer usage on the control plane.
func OpenSQLite(ctx context.Context, dbPath string) (*sql.DB, error) {
	if err := backupBeforeApplicationModelMigration(dbPath); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("file:%s?_busy_timeout=5000&_journal_mode=WAL", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite is happiest with one connection for concurrent-safe writes on a single file.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := ApplyMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func backupBeforeApplicationModelMigration(dbPath string) error {
	info, err := os.Stat(dbPath)
	if errors.Is(err, os.ErrNotExist) || err == nil && info.Size() == 0 {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat sqlite before migration backup: %w", err)
	}
	backupPath := dbPath + ".pre-application-model.bak"
	if _, err := os.Stat(backupPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat sqlite migration backup: %w", err)
	}
	src, err := os.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open sqlite migration source: %w", err)
	}
	defer src.Close()
	dst, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("create sqlite migration backup: %w", err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = os.Remove(backupPath)
		return fmt.Errorf("copy sqlite migration backup: %w", err)
	}
	if err := dst.Sync(); err != nil {
		_ = dst.Close()
		return fmt.Errorf("sync sqlite migration backup: %w", err)
	}
	if err := dst.Close(); err != nil {
		return fmt.Errorf("close sqlite migration backup: %w", err)
	}
	return nil
}
