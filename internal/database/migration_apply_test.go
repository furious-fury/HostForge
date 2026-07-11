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

func TestApplyMigrationsIncludesProjectDeployColumns(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "deploycfg.db")
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
	err = db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM pragma_table_info('projects') WHERE name IN ('deploy_runtime','deploy_install_cmd','deploy_build_cmd','deploy_start_cmd')`,
	).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Fatalf("expected 4 deploy_* columns on projects, got count=%d", n)
	}
}

func TestApplyMigrationsIncludesObservabilityTables(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "obs.db")
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
	for _, tbl := range []string{"deploy_steps", "http_requests"} {
		var name string
		err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&name)
		if err != nil || name != tbl {
			t.Fatalf("missing table %s: %v", tbl, err)
		}
	}
}

func TestApplyMigrationsIncludesGitHubAppTables(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gh-app.db")
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
	for _, tbl := range []string{"github_app", "github_app_installations", "project_ssh_keys"} {
		var name string
		err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&name)
		if err != nil || name != tbl {
			t.Fatalf("missing table %s: %v", tbl, err)
		}
	}
	var n int
	err = db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM pragma_table_info('projects') WHERE name IN ('git_source','github_installation_id')`,
	).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expected git_source + github_installation_id on projects, got count=%d", n)
	}
	err = db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM pragma_table_info('github_app') WHERE name IN ('app_id','slug','html_url','client_id','client_secret_ct','private_key_ct','webhook_secret_ct','created_at','updated_at')`,
	).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 9 {
		t.Fatalf("expected 9 core columns on github_app, got count=%d", n)
	}
	err = db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM pragma_table_info('github_app_installations') WHERE name IN ('installation_id','account_login','account_type','target_type','repo_selection','suspended_at','last_synced_at','created_at')`,
	).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 8 {
		t.Fatalf("expected 8 core columns on github_app_installations, got count=%d", n)
	}
}

func TestApplyMigrationsIncludesProjectGitAuthTable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "git-auth.db")
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
	var name string
	err = db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='project_git_auth'`).Scan(&name)
	if err != nil || name != "project_git_auth" {
		t.Fatalf("missing table project_git_auth: %v", err)
	}
	var n int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('project_git_auth') WHERE name IN ('project_id','provider','token_ct','token_last4','created_at','updated_at')`).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 6 {
		t.Fatalf("expected 6 core columns on project_git_auth, got count=%d", n)
	}
}
