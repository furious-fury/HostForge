package nixpacks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// Build runs `nixpacks build` in workDir and writes artifacts to outDir (-o).
// Streams stdout/stderr to the process stdout/stderr (typically CLI).
func Build(ctx context.Context, workDir, outDir, imageName string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("mkdir build output: %w", err)
	}
	name := imageName
	if name == "" {
		name = "hostforge-app"
	}
	// -o / --out: filesystem output without requiring Docker for the image load (see nixpacks CLI docs).
	args := []string{"build", ".", "--name", name, "-o", outDir}
	cmd := exec.CommandContext(ctx, "nixpacks", args...)
	cmd.Dir = workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nixpacks build in %s: %w", workDir, err)
	}
	return nil
}

// BuildImage runs `nixpacks build` in workDir and emits an image in the local Docker daemon.
// Streams stdout/stderr to the process stdout/stderr.
func BuildImage(ctx context.Context, workDir, imageRef string) error {
	if imageRef == "" {
		imageRef = "hostforge/app:latest"
	}
	args := []string{"build", ".", "--name", imageRef}
	cmd := exec.CommandContext(ctx, "nixpacks", args...)
	cmd.Dir = workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nixpacks build image in %s: %w", workDir, err)
	}
	return nil
}
