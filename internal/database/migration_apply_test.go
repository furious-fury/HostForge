package database

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestApplyMigrationsIncludesCertColumns(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "t.db")
	dsn := fmt.Sprintf("file:%s?_busy_timeout=5000", filepath.ToSlash(dbPath))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	var n int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('domains') WHERE name IN ('last_cert_message', 'cert_checked_at')`).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expected cert columns on domains, got count=%d", n)
	}
}
