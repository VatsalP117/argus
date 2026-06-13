package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"argus/internal/candidate"
	"argus/internal/config"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("cleanup-staging", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var storageConfigPath string
	var ingestBatchID string
	var cleanupScript string
	flags.StringVar(&storageConfigPath, "storage-config", "configs/storage/local.yaml", "path to storage config")
	flags.StringVar(&ingestBatchID, "ingest-batch-id", "", "validated ingest batch to clean")
	flags.StringVar(&cleanupScript, "cleanup-script", "scripts/dev/duckdb_cleanup_staging.py", "path to DuckDB cleanup adapter")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if ingestBatchID == "" {
		fmt.Fprintln(stderr, "ingest-batch-id is required")
		return 2
	}

	storage, err := config.LoadStorageConfig(storageConfigPath)
	if err != nil {
		fmt.Fprintf(stderr, "load storage config: %v\n", err)
		return 1
	}
	result, err := candidate.Cleanup(context.Background(), candidate.CleanupOptions{
		DatabasePath:  storage.DatabasePath,
		IngestBatchID: ingestBatchID,
		ScriptPath:    cleanupScript,
	})
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
