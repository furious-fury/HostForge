package caddy

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIsCaddyAdminUnreachable(t *testing.T) {
	err := fmt.Errorf(`Error: sending configuration to instance: performing request: Post "http://localhost:2019/load": dial tcp 127.0.0.1:2019: connect: connection refused`)
	if !isCaddyAdminUnreachable(err) {
		t.Fatal("expected admin unreachable")
	}
	if isCaddyAdminUnreachable(fmt.Errorf("caddy validate: some parse error")) {
		t.Fatal("unexpected match")
	}
	if isCaddyAdminUnreachable(nil) {
		t.Fatal("nil")
	}
}

func TestWriteAtomicMakesGeneratedFileGroupReadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX file mode bits")
	}
	path := filepath.Join(t.TempDir(), "hostforge.caddy")
	if err := writeAtomic(path, []byte("example.com {}\n")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o640); got != want {
		t.Fatalf("generated file permissions = %o, want %o", got, want)
	}
}

func TestRenderBootstrapConfigIsHTTPSOnly(t *testing.T) {
	got := RenderBootstrapConfig("203.0.113.4", 8443)
	for _, want := range []string{"auto_https disable_redirects", "https://203.0.113.4:8443", "tls", "reverse_proxy 127.0.0.1:8080"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, "http://") {
		t.Fatal("bootstrap must not render HTTP")
	}
}
