package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"argus/internal/config"
	"argus/internal/database"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("db-status", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var configPath string
	var databasePath string
	var adminScript string
	flags.StringVar(&configPath, "config", "configs/storage/local.yaml", "path to storage config")
	flags.StringVar(&databasePath, "database-path", "", "override DuckDB database path")
	flags.StringVar(&adminScript, "admin-script", "scripts/dev/duckdb_admin.py", "path to DuckDB admin adapter")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.LoadStorageConfig(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "load storage config: %v\n", err)
		return 1
	}
	if databasePath != "" {
		cfg.DatabasePath = databasePath
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(stderr, "validate storage config: %v\n", err)
		return 1
	}

	result, err := database.Status(
		context.Background(),
		database.AdminOptions{
			DatabasePath: cfg.DatabasePath,
			ScriptPath:   adminScript,
		},
		database.CapacityPolicy{
			DurableWarnBytes:            cfg.Durable.WarnBytes,
			DurableStopWideningBytes:    cfg.Durable.StopWideningBytes,
			DurableRetentionReviewBytes: cfg.Durable.RetentionReviewBytes,
			DurableHardLimitBytes:       cfg.Durable.HardLimitBytes,
			TemporaryWarnBytes:          cfg.Temporary.WarnBytes,
			TemporaryStopNewBatchBytes:  cfg.Temporary.StopNewBatchBytes,
			TemporaryHardAbortBytes:     cfg.Temporary.HardAbortBytes,
			TemporaryMaxStagingBytes:    cfg.Temporary.MaxStagingBytes,
			MinimumFreeDiskBytes:        cfg.MinimumFreeDiskBytes,
		},
	)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}
