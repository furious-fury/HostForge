package repository

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/hostforge/hostforge/internal/database"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	db, err := database.OpenSQLite(context.Background(), filepath.Join(dir, "hf.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return New(db)
}

func TestGitHubAppRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newTestStore(t)

	if _, err := s.GetGitHubAppMeta(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows on empty, got %v", err)
	}

	meta, err := s.UpsertGitHubApp(ctx, UpsertGitHubAppInput{
		AppID:           12345,
		Slug:            "hostforge-test",
		HTMLURL:         "https://github.com/apps/hostforge-test",
		ClientID:        "Iv1.abc",
		ClientSecretCT:  []byte("cs"),
		PrivateKeyCT:    []byte("pk"),
		WebhookSecretCT: []byte("ws"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if meta.AppID != 12345 || meta.Slug != "hostforge-test" {
		t.Fatalf("unexpected meta: %+v", meta)
	}
	sealed, err := s.GetGitHubAppSealed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if string(sealed.PrivateKeyCT) != "pk" || string(sealed.WebhookSecretCT) != "ws" {
		t.Fatalf("unexpected sealed payload: %+v", sealed)
	}

	// Upsert replaces existing row (singleton).
	if _, err := s.UpsertGitHubApp(ctx, UpsertGitHubAppInput{
		AppID:        12345,
		Slug:         "hostforge-test",
		PrivateKeyCT: []byte("pk2"),
	}); err != nil {
		t.Fatal(err)
	}
	sealed2, _ := s.GetGitHubAppSealed(ctx)
	if string(sealed2.PrivateKeyCT) != "pk2" {
		t.Fatalf("expected private_key update, got %q", string(sealed2.PrivateKeyCT))
	}

	if err := s.DeleteGitHubApp(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteGitHubApp(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows on second delete, got %v", err)
	}
}

func TestGitHubInstallationsCRUD(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newTestStore(t)

	if err := s.UpsertGitHubInstallation(ctx, UpsertGitHubInstallationInput{
		InstallationID: 100,
		AccountLogin:   "alice",
		AccountType:    "User",
		TargetType:     "User",
		RepoSelection:  "selected",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertGitHubInstallation(ctx, UpsertGitHubInstallationInput{
		InstallationID: 200,
		AccountLogin:   "acme",
		AccountType:    "Organization",
		TargetType:     "Organization",
		RepoSelection:  "all",
	}); err != nil {
		t.Fatal(err)
	}

	list, err := s.ListGitHubInstallations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(list))
	}
	// Ordered by account_login ASC: acme, alice.
	if list[0].AccountLogin != "acme" || list[1].AccountLogin != "alice" {
		t.Fatalf("unexpected order: %+v", list)
	}

	if err := s.SuspendGitHubInstallation(ctx, 100, true); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetGitHubInstallation(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got.SuspendedAt == "" {
		t.Fatal("expected suspended_at to be set")
	}

	if err := s.SuspendGitHubInstallation(ctx, 100, false); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetGitHubInstallation(ctx, 100)
	if got.SuspendedAt != "" {
		t.Fatalf("expected suspended_at cleared, got %q", got.SuspendedAt)
	}

	if err := s.DeleteGitHubInstallation(ctx, 999); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows for missing id, got %v", err)
	}
	if err := s.DeleteGitHubInstallation(ctx, 100); err != nil {
		t.Fatal(err)
	}
}
