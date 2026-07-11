package dnsops

import (
	"errors"
	"strings"
	"testing"
)

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
