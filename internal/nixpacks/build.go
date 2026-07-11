// Package nixpacks invokes the nixpacks CLI to build app images for Docker.
package nixpacks

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// DefaultNodeVersion is the major Node.js version Nixpacks will use when the
// project doesn't declare one via engines.node, .nvmrc, or its own
// NIXPACKS_NODE_VERSION env var. Nixpacks' built-in default is 18, which is
// too old for most modern stacks; major 22 matches current Nixpacks + nixpkgs
// pins (nodejs_22) and satisfies common ">=20" / "^22.12" style tooling when
// the bundled nixpkgs snapshot is recent enough.
const DefaultNodeVersion = "22"

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
	args := []string{"build", ".", "--name", name, "-o", outDir}
	args = append(args, defaultNixpacksFlags()...)
	cmd := exec.CommandContext(ctx, "nixpacks", args...)
	cmd.Dir = workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = nixpacksEnv()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nixpacks build in %s: %w", workDir, err)
	}
	return nil
}

// BuildImage runs `nixpacks build` in workDir and emits an image in the local Docker daemon.
// Streams stdout/stderr to the process stdout/stderr.
func BuildImage(ctx context.Context, workDir, imageRef string) error {
	return BuildImageWithWriters(ctx, workDir, imageRef, os.Stdout, os.Stderr)
}

// BuildImageWithWriters runs `nixpacks build` and streams process output to provided writers.
func BuildImageWithWriters(ctx context.Context, workDir, imageRef string, stdout, stderr io.Writer) error {
	if imageRef == "" {
		imageRef = "hostforge/app:latest"
	}
	args := []string{"build", ".", "--name", imageRef}
	args = append(args, defaultNixpacksFlags()...)
	cmd := exec.CommandContext(ctx, "nixpacks", args...)
	cmd.Dir = workDir
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = nixpacksEnv()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nixpacks build image in %s: %w", workDir, err)
	}
	return nil
}

// defaultNixpacksFlags returns CLI flags injected into every nixpacks
// invocation. Currently sets the default Node version unless the operator
// has already set NIXPACKS_NODE_VERSION in the server environment.
func defaultNixpacksFlags() []string {
	if v := os.Getenv("NIXPACKS_NODE_VERSION"); v != "" {
		return nil
	}
	return []string{"--env", "NIXPACKS_NODE_VERSION=" + DefaultNodeVersion}
}

// nixpacksEnv returns the process environment with HostForge defaults injected
// for Nixpacks. Values already set by the operator are preserved (no override).
func nixpacksEnv() []string {
	env := os.Environ()
	if os.Getenv("NIXPACKS_NODE_VERSION") == "" {
		env = append(env, "NIXPACKS_NODE_VERSION="+DefaultNodeVersion)
	}
	return env
}
