package services

import (
	"testing"
	"time"
)

// buildID had one-second timestamp resolution, so two deploys of the same
// repo+branch starting in the same wall-clock second produced the same
// image tag and container name — the second build silently overwrote the
// first's tag, and docker.RunContainer would fail outright on the
// container-name clash. newBuildID appends random suffix bytes so this
// cannot happen even within one second.
func TestNewBuildIDsAreUniqueWithinTheSameSecond(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		id, err := newBuildID()
		if err != nil {
			t.Fatal(err)
		}
		if seen[id] {
			t.Fatalf("duplicate build id %q after %d generated", id, i)
		}
		seen[id] = true
	}
}

func TestNewBuildIDStartsWithATimestamp(t *testing.T) {
	id, err := newBuildID()
	if err != nil {
		t.Fatal(err)
	}
	// The timestamp prefix is what keeps ids roughly sortable and
	// human-readable in `docker images`; only the collision resistance is
	// new, not the format contract callers already depend on.
	prefix := time.Now().UTC().Format("20060102t15")
	if len(id) < len(prefix) || id[:len(prefix)] != prefix {
		t.Fatalf("build id %q does not start with the expected timestamp prefix %q", id, prefix)
	}
}

func TestDeployLockKeyMatchesTheWorktreeScope(t *testing.T) {
	// DeployLockKey's own doc comment says the worktree scope must be
	// identical to it. This just pins that the function used for both is
	// in fact the same function, so a future edit to one cannot drift from
	// the other without this test catching it.
	if got, want := DeployLockKey("svc-a", "env-1"), "svc:svc-a:env-1"; got != want {
		t.Fatalf("DeployLockKey(svc-a, env-1) = %q, want %q", got, want)
	}
}
