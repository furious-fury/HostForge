// Package validation holds small, dependency-light checks used by the CLI and scripts.
package validation

import (
	"context"
	"fmt"

	"github.com/hostforge/hostforge/internal/docker"
)

// CheckDocker pings the Docker daemon using the same client path as deploy (DOCKER_HOST, etc.).
func CheckDocker(ctx context.Context) error {
	cli, err := docker.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("docker: %w", err)
	}
	_ = cli.Close()
	return nil
}
