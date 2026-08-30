package main

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
)

// -version (ADR-0002 §24.2/§24.5) must print and exit 0 before any startup
// validation runs — install.sh's "record installed version" step calls it
// right after a fresh install, before any of HOSTFORGE_API_TOKEN,
// HOSTFORGE_SESSION_SECRET, etc. are necessarily configured.
func TestRunServerVersionFlagExitsBeforeConfigValidation(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdout := os.Stdout
	os.Stdout = w

	code := runServer(log, []string{"-version"})

	w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}

	if code != 0 {
		t.Fatalf("runServer(-version) exit code = %d, want 0; output=%q", code, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "commit=") || !strings.Contains(out, "build_time=") {
		t.Fatalf("unexpected -version output: %q", out)
	}
	if !strings.HasPrefix(out, "v") {
		t.Fatalf("-version output should start with the v-prefixed display version, got: %q", out)
	}
}
