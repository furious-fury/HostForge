package database

import (
	"context"
	"path/filepath"
	"testing"
)

// The DSN in OpenSQLite must be written in the syntax the driver actually
// parses. modernc.org/sqlite reads `_pragma=name(value)`; the `_busy_timeout=`
// and `_journal_mode=` forms belong to mattn/go-sqlite3 and are ignored here,
// which silently leaves the database in its default rollback-journal mode.
func TestOpenSQLiteAppliesPragmas(t *testing.T) {
	db, err := OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "pragma.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, tc := range []struct {
		pragma string
		want   string
	}{
		{"journal_mode", "wal"},
		{"busy_timeout", "5000"},
		{"foreign_keys", "1"},
		{"synchronous", "1"}, // NORMAL, the standard companion to WAL
	} {
		var got string
		if err := db.QueryRowContext(context.Background(), "PRAGMA "+tc.pragma).Scan(&got); err != nil {
			t.Fatalf("read PRAGMA %s: %v", tc.pragma, err)
		}
		if got != tc.want {
			t.Errorf("PRAGMA %s = %q, want %q", tc.pragma, got, tc.want)
		}
	}
}
