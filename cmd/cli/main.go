// Package main implements HostForge operator diagnostics and route synchronization.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/furious-fury/HostForge/internal/config"
	"github.com/furious-fury/HostForge/internal/database"
	"github.com/furious-fury/HostForge/internal/logging"
	"github.com/furious-fury/HostForge/internal/repository"
	"github.com/furious-fury/HostForge/internal/services"
	"github.com/furious-fury/HostForge/internal/version"
)

func main() {
	log := logging.New()
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}
	var code int
	switch os.Args[1] {
	case "caddy":
		code = runCaddy(log, os.Args[2:])
	case "validate":
		code = runValidate(log, os.Args[2:])
	case "version":
		fmt.Printf("hostforge %s\n", version.Display())
	default:
		printUsage()
		code = 2
	}
	os.Exit(code)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `%s <command>

Commands:
  caddy sync [-data-dir path]  Validate and apply service domain routes
  validate docker|preflight   Run host diagnostics
  version                     Print the HostForge version
`, os.Args[0])
}

func runCaddy(log *slog.Logger, args []string) int {
	if len(args) == 0 || args[0] != "sync" {
		printUsage()
		return 2
	}
	flags := flag.NewFlagSet("caddy sync", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	dataDir := flags.String("data-dir", "", "data directory (overrides "+config.DataDirEnv+")")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	cfg, err := config.Load(*dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: config: %v\n", err)
		return 2
	}
	ctx := context.Background()
	db, err := database.OpenSQLite(ctx, cfg.DBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: database: %v\n", err)
		return 1
	}
	defer db.Close()
	if err := services.SyncCaddyRoutes(ctx, log, cfg, repository.New(db)); err != nil {
		fmt.Fprintf(os.Stderr, "error: caddy sync: %v\n", err)
		return 1
	}
	fmt.Println("caddy: synced")
	return 0
}
