// Package bootstrap defines the temporary, HTTPS-only onboarding ingress.
package bootstrap

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Config is the immutable bootstrap ingress configuration supplied at install time.
type Config struct {
	Enabled   bool
	PublicIP  string
	HTTPSPort int
	ExpiresAt string
}

// Validate rejects unsafe or incomplete bootstrap configurations before startup.
func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if ip := net.ParseIP(strings.TrimSpace(c.PublicIP)); ip == nil {
		return fmt.Errorf("bootstrap public IP must be a literal IP address")
	}
	if c.HTTPSPort < 1 || c.HTTPSPort > 65535 {
		return fmt.Errorf("bootstrap HTTPS port must be between 1 and 65535")
	}
	if strings.TrimSpace(c.ExpiresAt) == "" {
		return fmt.Errorf("bootstrap expiry is required")
	}
	if _, err := time.Parse(time.RFC3339, c.ExpiresAt); err != nil {
		return fmt.Errorf("bootstrap expiry must be RFC3339: %w", err)
	}
	return nil
}

// Address returns the explicit HTTPS listener address Caddy should use.
func (c Config) Address() string {
	return "https://" + net.JoinHostPort(c.PublicIP, strconv.Itoa(c.HTTPSPort))
}
