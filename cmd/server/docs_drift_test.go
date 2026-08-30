package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoRetiredTermsInPublicFacingDocs guards ADR-0002 §24.7: documentation
// and marketing copy must not describe retired functionality. It checks only
// the files a prospective user or crawler actually reads — README.md and the
// public site — not docs/, which legitimately keeps historical documents
// (docs/v2-migration-baseline.md, docs/operator-validation-phase1.md) that
// describe what HostForge used to be. Add a term here only after confirming
// it is genuinely gone from the product, not merely from these two files.
func TestNoRetiredTermsInPublicFacingDocs(t *testing.T) {
	retiredTerms := []string{"nixpacks", "cmd/cli"}

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	// Directories under site/ that are build output or dependencies, not
	// source — scanning them would both be pointless (they're regenerated)
	// and produce false positives from vendored third-party code.
	skipDirNames := map[string]bool{
		"node_modules": true,
		"dist":         true,
		".git":         true,
	}
	// Extensions worth reading as text. Binary/asset files are skipped.
	textExt := map[string]bool{
		".md": true, ".html": true, ".ts": true, ".tsx": true,
		".js": true, ".jsx": true, ".css": true, ".json": true,
	}

	var checked int
	check := func(path string) {
		ext := strings.ToLower(filepath.Ext(path))
		if !textExt[ext] {
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		checked++
		lower := bytes.ToLower(data)
		for _, term := range retiredTerms {
			if bytes.Contains(lower, []byte(strings.ToLower(term))) {
				rel, _ := filepath.Rel(repoRoot, path)
				t.Errorf("%s: contains retired term %q", rel, term)
			}
		}
	}

	check(filepath.Join(repoRoot, "README.md"))

	siteRoot := filepath.Join(repoRoot, "site")
	if err := filepath.WalkDir(siteRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirNames[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		check(path)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if checked < 5 {
		t.Fatalf("only checked %d files; the walk is probably misconfigured (repoRoot=%s)", checked, repoRoot)
	}
}
