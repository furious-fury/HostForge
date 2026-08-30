package railpack

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/furious-fury/HostForge/internal/builder"
)

func TestAdapter_BuildsWithPrepareAndBuildKitThenCleansArtifacts(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()
	artifactsRoot := t.TempDir()
	plannerRunner := &fakeRunner{run: func(_ string, args []string, _ string, stdout, _ io.Writer) error {
		if len(args) == 1 && args[0] == "--version" {
			_, _ = io.WriteString(stdout, "railpack 0.23.0\n")
			return nil
		}
		if err := os.WriteFile(args[3], []byte(`{"steps":{}}`), 0o600); err != nil {
			return err
		}
		return os.WriteFile(args[5], []byte(`{"detectedProviders":["go"]}`), 0o600)
	}}
	planner := newTestPlanner(t, plannerRunner)
	buildRunner := &fakeRunner{run: func(_ string, _ []string, _ string, stdout, _ io.Writer) error {
		_, _ = io.WriteString(stdout, "docker image tar")
		return nil
	}}
	executor := testExecutor(t, buildRunner, &fakeImageStore{imageID: "sha256:abc"})
	adapter, err := NewAdapter(AdapterConfig{Planner: planner, Executor: executor, ArtifactsRoot: artifactsRoot})
	if err != nil {
		t.Fatal(err)
	}
	request := builder.Request{Worktree: worktree, ImageRef: "hostforge/example:build-1", Platform: "linux/amd64", CacheKey: "project-1"}
	var events []builder.Event
	result, err := adapter.Build(context.Background(), request, func(event builder.Event) { events = append(events, event) })
	if err != nil {
		t.Fatal(err)
	}
	if result.ImageID != "sha256:abc" || result.StackKind != "go" || result.StackLabel != "Go" || len(events) < 2 || events[0].Phase != "prepare" || events[len(events)-1].Phase == "prepare" {
		t.Fatalf("unexpected result=%+v events=%+v", result, events)
	}
	// Content must have been captured (into the returned Result) before
	// the artifacts directory below is confirmed deleted. These two
	// assertions are deliberately in tension: capturing plan/info content
	// only works if the read happens before the artifacts dir is
	// removed, so this is the one test that would catch a future change
	// that moved the read to after cleanup.
	if result.PlanJSON != `{"steps":{}}` || result.InfoJSON != `{"detectedProviders":["go"]}` {
		t.Fatalf("provenance not captured before artifact cleanup: plan=%q info=%q", result.PlanJSON, result.InfoJSON)
	}
	entries, err := os.ReadDir(artifactsRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary artifacts remain: %+v", entries)
	}
}

func TestNewAdapter_RequiresAllComponents(t *testing.T) {
	t.Parallel()
	planner, err := NewPlanner("railpack-test", DefaultVersion)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewAdapter(AdapterConfig{Planner: planner, ArtifactsRoot: filepath.Join(t.TempDir(), "artifacts")}); err == nil {
		t.Fatal("expected missing executor error")
	}
}
