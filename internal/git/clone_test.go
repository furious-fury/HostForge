package git

// WorktreeDir is scope-keyed to close a real corruption path: two deploys
// sharing a repo+branch used to share a directory and run git clone,
// checkout, and pull concurrently in it. Callers pass the deploy's lock
// key as scope, so anything the queue does not already serialise gets its
// own worktree.

import "testing"

func TestWorktreeDirDiffersByScope(t *testing.T) {
	a := WorktreeDir("svc:service-a:env-1", "https://github.com/acme/monorepo.git", "main")
	b := WorktreeDir("svc:service-b:env-1", "https://github.com/acme/monorepo.git", "main")
	if a == b {
		t.Fatalf("two services sharing a repo+branch produced the same worktree dir: %s", a)
	}
}

func TestWorktreeDirDiffersByEnvironmentWithinOneService(t *testing.T) {
	// One service deployed to two environments tracking the same branch —
	// lock_key is per (service, environment), so the worktree must be too,
	// or two environments that can legitimately deploy concurrently would
	// still share a checkout.
	staging := WorktreeDir("svc:service-a:staging", "https://github.com/acme/app.git", "main")
	production := WorktreeDir("svc:service-a:production", "https://github.com/acme/app.git", "main")
	if staging == production {
		t.Fatalf("two environments of one service produced the same worktree dir: %s", staging)
	}
}

func TestWorktreeDirStableForTheSameScope(t *testing.T) {
	first := WorktreeDir("svc:service-a:env-1", "https://github.com/acme/app.git", "main")
	second := WorktreeDir("svc:service-a:env-1", "https://github.com/acme/app.git", "main")
	if first != second {
		t.Fatalf("WorktreeDir is not deterministic: %s != %s", first, second)
	}
}

func TestWorktreeDirDiffersByBranchWithinOneScope(t *testing.T) {
	main := WorktreeDir("svc:service-a:env-1", "https://github.com/acme/app.git", "main")
	feature := WorktreeDir("svc:service-a:env-1", "https://github.com/acme/app.git", "feature-x")
	if main == feature {
		t.Fatalf("two branches within one scope produced the same worktree dir: %s", main)
	}
}
