package railpack

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/hostforge/hostforge/internal/builder"
)

func TestDockerfileBuilder_ImplementsBuilderContract(t *testing.T) {
	t.Parallel()
	request, _ := preparedBuildInput(t)
	if err := os.WriteFile(filepath.Join(request.Worktree, "Dockerfile"), []byte("FROM scratch\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{run: func(_ string, _ []string, _ string, stdout, _ io.Writer) error {
		_, _ = io.WriteString(stdout, "docker image tar")
		return nil
	}}
	builderImpl, err := NewDockerfileBuilder(testExecutor(t, runner, &fakeImageStore{imageID: "sha256:dockerfile"}))
	if err != nil {
		t.Fatal(err)
	}
	var events []builder.Event
	result, err := builderImpl.Build(context.Background(), request, func(event builder.Event) { events = append(events, event) })
	if err != nil {
		t.Fatal(err)
	}
	if result.Kind != builder.KindDockerfile || result.StackKind != "dockerfile" || result.StackLabel != "Dockerfile" || len(events) == 0 {
		t.Fatalf("unexpected result=%+v events=%+v", result, events)
	}
}
