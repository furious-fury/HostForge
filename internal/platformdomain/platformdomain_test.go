package platformdomain

import "testing"

func TestHostname(t *testing.T) {
	if got, want := Hostname("apps.example.com", "Paylance!", ""), "paylance.apps.example.com"; got != want {
		t.Fatalf("Hostname() = %q, want %q", got, want)
	}
	if got, want := Hostname("apps.example.com", "Paylance", "abc123"), "paylance-abc123.apps.example.com"; got != want {
		t.Fatalf("Hostname() = %q, want %q", got, want)
	}
}

func TestNormalizeBase(t *testing.T) {
	got, err := NormalizeBase(" Apps.Example.COM. ")
	if err != nil || got != "apps.example.com" {
		t.Fatalf("NormalizeBase() = %q, %v", got, err)
	}
}
