package caddy

import (
	"fmt"
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
