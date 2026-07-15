package services

import (
	"path/filepath"
	"testing"
)

func TestResolveServiceBuildDirectory(t *testing.T) {
	worktree := filepath.Join(t.TempDir(), "repo")
	got, err := ResolveServiceBuildDirectory(worktree, "apps/api")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(worktree, "apps", "api") {
		t.Fatalf("got %q", got)
	}
	for _, invalid := range []string{"../api", "../../etc", "/tmp/api"} {
		if _, err := ResolveServiceBuildDirectory(worktree, invalid); err == nil || FirstPublicCode(err) != "invalid_root_directory" {
			t.Fatalf("expected invalid_root_directory for %q, got %v", invalid, err)
		}
	}
}
