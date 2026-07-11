package version

import "testing"

func TestString_nonEmpty(t *testing.T) {
	if s := String(); s == "" || s == "0.0.0-dev" {
		t.Fatalf("unexpected version %q", s)
	}
	if d := Display(); d != "v"+String() {
		t.Fatalf("Display %q want v+%q", d, String())
	}
}
