package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/hostforge/hostforge/internal/validation"
)

func printValidateUsage() {
	fmt.Fprintf(os.Stderr, `%s validate <subcommand>

Subcommands:
  docker     Ping Docker Engine (same env as deploy: DOCKER_HOST, etc.)
  preflight  docker + required tools on PATH (git, nixpacks)

`, os.Args[0])
}

func runValidate(log *slog.Logger, args []string) int {
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		printValidateUsage()
		return 2
	}
	switch args[0] {
	case "docker":
		return runValidateDocker(log)
	case "preflight":
		return runValidatePreflight(log)
	default:
		fmt.Fprintf(os.Stderr, "error: unknown validate subcommand %q\n\n", args[0])
		printValidateUsage()
		return 2
	}
}

func runValidateDocker(log *slog.Logger) int {
	ctx := context.Background()
	if err := validation.CheckDocker(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	log.Info("validate docker: ok (daemon reachable)")
	fmt.Println("docker: ok")
	return 0
}

func runValidatePreflight(log *slog.Logger) int {
	if code := runValidateDocker(log); code != 0 {
		return code
	}
	for _, tool := range []struct {
		name string
		args []string
	}{
		{"git", []string{"--version"}},
		{"nixpacks", []string{"--version"}},
	} {
		path, err := exec.LookPath(tool.name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s not on PATH\n", tool.name)
			return 1
		}
		cmd := exec.Command(path, tool.args...)
		cmd.Stderr = os.Stderr
		out, err := cmd.Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s: %v\n", tool.name, err)
			return 1
		}
		line := strings.TrimSpace(string(out))
		if len(line) > 120 {
			line = line[:120] + "..."
		}
		fmt.Printf("%s: ok (%s)\n", tool.name, line)
	}
	log.Info("validate preflight: ok")
	fmt.Println("preflight: ok")
	return 0
}
