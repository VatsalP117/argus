package database

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

type AdminOptions struct {
	PythonExecutable string
	ScriptPath       string
	DatabasePath     string
	MigrationsDir    string
}

type MigrationResult struct {
	Status            string `json:"status"`
	DatabasePath      string `json:"database_path"`
	SchemaVersion     int    `json:"schema_version"`
	AppliedMigrations []int  `json:"applied_migrations"`
}

type CapacityPolicy struct {
	DurableWarnBytes            int64 `json:"durable_warn_bytes"`
	DurableStopWideningBytes    int64 `json:"durable_stop_widening_bytes"`
	DurableRetentionReviewBytes int64 `json:"durable_retention_review_bytes"`
	DurableHardLimitBytes       int64 `json:"durable_hard_limit_bytes"`
	TemporaryWarnBytes          int64 `json:"temporary_warn_bytes"`
	TemporaryStopNewBatchBytes  int64 `json:"temporary_stop_new_batch_bytes"`
	TemporaryHardAbortBytes     int64 `json:"temporary_hard_abort_bytes"`
	TemporaryMaxStagingBytes    int64 `json:"temporary_max_staging_bytes"`
	MinimumFreeDiskBytes        int64 `json:"minimum_free_disk_bytes"`
}

type StatusResult struct {
	Status            string         `json:"status"`
	DatabasePath      string         `json:"database_path"`
	DatabaseExists    bool           `json:"database_exists"`
	DatabaseSizeBytes int64          `json:"database_size_bytes"`
	SchemaVersion     int            `json:"schema_version"`
	FreeDiskBytes     int64          `json:"free_disk_bytes"`
	DurableState      string         `json:"durable_state"`
	FreeDiskState     string         `json:"free_disk_state"`
	CanStartNewBatch  bool           `json:"can_start_new_batch"`
	Thresholds        CapacityPolicy `json:"thresholds"`
}

type rawStatusResult struct {
	Status            string `json:"status"`
	DatabasePath      string `json:"database_path"`
	DatabaseExists    bool   `json:"database_exists"`
	DatabaseSizeBytes int64  `json:"database_size_bytes"`
	SchemaVersion     int    `json:"schema_version"`
	FreeDiskBytes     int64  `json:"free_disk_bytes"`
}

func Migrate(ctx context.Context, options AdminOptions) (MigrationResult, error) {
	var result MigrationResult

	pythonExecutable := options.PythonExecutable
	if pythonExecutable == "" {
		pythonExecutable = "python3"
	}
	scriptPath := options.ScriptPath
	if scriptPath == "" {
		scriptPath = "scripts/dev/duckdb_admin.py"
	}

	cmd := exec.CommandContext(
		ctx,
		pythonExecutable,
		scriptPath,
		"migrate",
		"--database-path", options.DatabasePath,
		"--migrations-dir", options.MigrationsDir,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return result, fmt.Errorf("duckdb migration failed: %w: %s", err, output)
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return result, fmt.Errorf("parse duckdb migration result: %w: %s", err, output)
	}
	if result.Status != "completed" {
		return result, fmt.Errorf("duckdb migration returned status %q", result.Status)
	}
	return result, nil
}

func Status(ctx context.Context, options AdminOptions, policy CapacityPolicy) (StatusResult, error) {
	var raw rawStatusResult

	pythonExecutable := options.PythonExecutable
	if pythonExecutable == "" {
		pythonExecutable = "python3"
	}
	scriptPath := options.ScriptPath
	if scriptPath == "" {
		scriptPath = "scripts/dev/duckdb_admin.py"
	}

	cmd := exec.CommandContext(
		ctx,
		pythonExecutable,
		scriptPath,
		"status",
		"--database-path", options.DatabasePath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return StatusResult{}, fmt.Errorf("duckdb status failed: %w: %s", err, output)
	}
	if err := json.Unmarshal(output, &raw); err != nil {
		return StatusResult{}, fmt.Errorf("parse duckdb status result: %w: %s", err, output)
	}
	if raw.Status != "completed" {
		return StatusResult{}, fmt.Errorf("duckdb status returned status %q", raw.Status)
	}

	freeDiskState := "healthy"
	if raw.FreeDiskBytes < policy.MinimumFreeDiskBytes {
		freeDiskState = "low"
	}

	return StatusResult{
		Status:            raw.Status,
		DatabasePath:      raw.DatabasePath,
		DatabaseExists:    raw.DatabaseExists,
		DatabaseSizeBytes: raw.DatabaseSizeBytes,
		SchemaVersion:     raw.SchemaVersion,
		FreeDiskBytes:     raw.FreeDiskBytes,
		DurableState:      durableState(raw.DatabaseSizeBytes, policy),
		FreeDiskState:     freeDiskState,
		CanStartNewBatch: raw.DatabaseSizeBytes < policy.DurableStopWideningBytes &&
			raw.FreeDiskBytes >= policy.MinimumFreeDiskBytes,
		Thresholds: policy,
	}, nil
}

func durableState(databaseSizeBytes int64, policy CapacityPolicy) string {
	switch {
	case databaseSizeBytes >= policy.DurableHardLimitBytes:
		return "hard_limit"
	case databaseSizeBytes >= policy.DurableRetentionReviewBytes:
		return "retention_review"
	case databaseSizeBytes >= policy.DurableStopWideningBytes:
		return "stop_widening"
	case databaseSizeBytes >= policy.DurableWarnBytes:
		return "warning"
	default:
		return "healthy"
	}
}
