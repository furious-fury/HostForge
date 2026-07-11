package railpack

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hostforge/hostforge/internal/builder"
)

type runCall struct {
	name string
	args []string
	dir  string
}

type fakeRunner struct {
	calls []runCall
	run   func(string, []string, string, io.Writer, io.Writer) error
}

func (f *fakeRunner) Run(_ context.Context, name string, args []string, dir string, stdout, stderr io.Writer) error {
	f.calls = append(f.calls, runCall{name: name, args: append([]string(nil), args...), dir: dir})
	return f.run(name, args, dir, stdout, stderr)
}

func newTestPlanner(t *testing.T, runner *fakeRunner) *Planner {
	t.Helper()
	planner, err := NewPlanner("railpack-test", DefaultVersion)
	if err != nil {
		t.Fatal(err)
	}
	planner.runner = runner
	return planner
}

func TestPrepare_WritesPlanAndInfoOutsideWorktree(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()
	artifacts := t.TempDir()
	runner := &fakeRunner{run: func(_ string, args []string, _ string, stdout, _ io.Writer) error {
		if reflect.DeepEqual(args, []string{"--version"}) {
			_, _ = io.WriteString(stdout, "railpack 0.23.0\n")
			return nil
		}
		planPath, infoPath := args[3], args[5]
		if err := os.WriteFile(planPath, []byte(`{"steps":{}}`), 0o600); err != nil {
			return err
		}
		return os.WriteFile(infoPath, []byte(`{"providers":[]}`), 0o600)
	}}

	preparation, err := newTestPlanner(t, runner).Prepare(context.Background(), PrepareRequest{Worktree: worktree, ArtifactsDir: artifacts}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if preparation.Version != DefaultVersion || filepath.Dir(preparation.PlanPath) != artifacts || filepath.Dir(preparation.InfoPath) != artifacts {
		t.Fatalf("unexpected preparation: %+v", preparation)
	}
	if len(runner.calls) != 2 || !reflect.DeepEqual(runner.calls[1].args[:2], []string{"prepare", worktree}) {
		t.Fatalf("unexpected calls: %+v", runner.calls)
	}
}

func TestPrepare_RejectsVersionMismatchBeforePrepare(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{run: func(_ string, args []string, _ string, stdout, _ io.Writer) error {
		if reflect.DeepEqual(args, []string{"--version"}) {
			_, _ = io.WriteString(stdout, "railpack 0.22.0\n")
		}
		return nil
	}}
	_, err := newTestPlanner(t, runner).Prepare(context.Background(), PrepareRequest{Worktree: t.TempDir(), ArtifactsDir: t.TempDir()}, nil, nil)
	if err == nil || len(runner.calls) != 1 {
		t.Fatalf("got err=%v calls=%+v", err, runner.calls)
	}
}

func TestPrepare_ClassifiesUnsupportedSource(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{run: func(_ string, args []string, _ string, stdout, stderr io.Writer) error {
		if reflect.DeepEqual(args, []string{"--version"}) {
			_, _ = io.WriteString(stdout, "railpack 0.23.0\n")
			return nil
		}
		_, _ = io.WriteString(stderr, "No providers found for this repository\n")
		return errors.New("exit status 1")
	}}
	_, err := newTestPlanner(t, runner).Prepare(context.Background(), PrepareRequest{Worktree: t.TempDir(), ArtifactsDir: t.TempDir()}, nil, nil)
	if !errors.Is(err, builder.ErrUnsupported) {
		t.Fatalf("got %v, want unsupported", err)
	}
}

func TestPrepare_RejectsArtifactsInsideWorktree(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()
	runner := &fakeRunner{run: func(_ string, _ []string, _ string, _, _ io.Writer) error {
		t.Fatal("runner must not be called")
		return nil
	}}
	_, err := newTestPlanner(t, runner).Prepare(context.Background(), PrepareRequest{Worktree: worktree, ArtifactsDir: filepath.Join(worktree, ".hostforge")}, nil, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
}
