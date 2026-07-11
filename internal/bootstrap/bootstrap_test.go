package bootstrap

import "testing"

func TestValidateRejectsUnsafeConfigurations(t *testing.T) {
	for _, c := range []Config{{Enabled: true}, {Enabled: true, PublicIP: "example.com", HTTPSPort: 443, ExpiresAt: "2026-08-01T00:00:00Z"}, {Enabled: true, PublicIP: "203.0.113.4", HTTPSPort: 80, ExpiresAt: ""}} {
		if c.Validate() == nil {
			t.Fatalf("expected invalid config: %#v", c)
		}
	}
}

func TestAddress(t *testing.T) {
	c := Config{PublicIP: "203.0.113.4", HTTPSPort: 8443}
	if got := c.Address(); got != "https://203.0.113.4:8443" {
		t.Fatalf("address=%q", got)
	}
}
