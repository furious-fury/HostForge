package dnsops

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCheckRegistrarARecords_unknownWithoutExpectedIP(t *testing.T) {
	checks := CheckRegistrarARecords(context.Background(), []string{"API.EXAMPLE.COM", "api.example.com", "www.example.com"}, "", 0)
	if len(checks) != 2 {
		t.Fatalf("got %d checks, want 2", len(checks))
	}
	for _, check := range checks {
		if check.Status != "unknown" || check.ResolvedIPv4 == nil {
			t.Fatalf("unexpected check: %+v", check)
		}
	}
}

func TestValidateDomainName_empty(t *testing.T) {
	if err := ValidateDomainName(""); !errors.Is(err, ErrDomainNameEmpty) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateDomainName_tooLong(t *testing.T) {
	host := strings.Repeat("a", 250) + ".example.com"
	if err := ValidateDomainName(host); !errors.Is(err, ErrDomainNameTooLong) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateDomainName_invalid(t *testing.T) {
	if err := ValidateDomainName("not_a_valid_fqdn"); !errors.Is(err, ErrDomainNameInvalid) {
		t.Fatalf("got %v", err)
	}
}
