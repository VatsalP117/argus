package candidate

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

type CleanupOptions struct {
	DatabasePath  string
	IngestBatchID string
	ScriptPath    string
}

type CleanupResult struct {
	Status          string `json:"status"`
	IngestBatchID   string `json:"ingest_batch_id"`
	StagingBatchID  string `json:"staging_batch_id"`
	FilesRemoved    int64  `json:"files_removed"`
	BytesRemoved    int64  `json:"bytes_removed"`
	DurableChecksum string `json:"durable_checksum"`
	CleanupStatus   string `json:"cleanup_status"`
}

func Cleanup(ctx context.Context, options CleanupOptions) (CleanupResult, error) {
	var result CleanupResult

	if options.DatabasePath == "" {
		return result, fmt.Errorf("database path is required")
	}
	if options.IngestBatchID == "" {
		return result, fmt.Errorf("ingest batch id is required")
	}
	scriptPath := options.ScriptPath
	if scriptPath == "" {
		scriptPath = "scripts/dev/duckdb_cleanup_staging.py"
	}

	cmd := exec.CommandContext(
		ctx,
		"python3",
		scriptPath,
		"--database-path", options.DatabasePath,
		"--ingest-batch-id", options.IngestBatchID,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return result, fmt.Errorf("staging cleanup failed: %w: %s", err, output)
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return result, fmt.Errorf("parse staging cleanup result: %w: %s", err, output)
	}
	if result.Status != "completed" && result.Status != "skipped_existing" {
		return result, fmt.Errorf("staging cleanup returned status %q", result.Status)
	}
	return result, nil
}
