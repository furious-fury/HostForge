package repository

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/hostforge/hostforge/internal/database"
	"github.com/hostforge/hostforge/internal/models"
)

func TestProjectGitAuthRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := database.OpenSQLite(ctx, filepath.Join(dir, "hf.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := New(db)
	p, err := store.CreateProject(ctx, CreateProjectInput{
		Name:          "app",
		RepoURL:       "https://github.com/example/app",
		Branch:        "main",
		DeployRuntime: models.DeployRuntimeAuto,
	})
	if err != nil {
		t.Fatal(err)
	}

	meta, err := store.UpsertProjectGitHubAuth(ctx, p.ID, []byte("cipher1"), "cdef")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Provider != gitAuthProviderGitHub || meta.TokenLast4 != "cdef" {
		t.Fatalf("unexpected metadata: %+v", meta)
	}

	sealed, err := store.GetProjectGitAuthSealed(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(sealed.TokenCT) != "cipher1" || sealed.Provider != gitAuthProviderGitHub {
		t.Fatalf("unexpected sealed row: %+v", sealed)
	}

	meta2, err := store.UpsertProjectGitHubAuth(ctx, p.ID, []byte("cipher2"), "7890")
	if err != nil {
		t.Fatal(err)
	}
	if meta2.TokenLast4 != "7890" {
		t.Fatalf("expected update to change last4, got %+v", meta2)
	}
	sealed2, err := store.GetProjectGitAuthSealed(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(sealed2.TokenCT) != "cipher2" {
		t.Fatalf("expected updated ciphertext, got %q", string(sealed2.TokenCT))
	}

	if err := store.DeleteProjectGitAuth(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetProjectGitAuthMeta(ctx, p.ID); err == nil {
		t.Fatal("expected no rows after delete")
	}
	if err := store.DeleteProjectGitAuth(ctx, p.ID); err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}
