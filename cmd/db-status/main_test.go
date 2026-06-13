package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"argus/internal/database"
)

func TestRunReportsConfiguredDatabaseStatus(t *testing.T) {
	dir := t.TempDir()
	migrationsDir := filepath.Join(dir, "migrations")
	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		t.Fatalf("create migrations dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(migrationsDir, "001_create_example.sql"),
		[]byte("CREATE TABLE example (id BIGINT);"),
		0o644,
	); err != nil {
		t.Fatalf("write migration: %v", err)
	}

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "dev", "duckdb_admin.py"))
	if err != nil {
		t.Fatalf("resolve admin script: %v", err)
	}
	databasePath := filepath.Join(dir, "argus.duckdb")
	if _, err := database.Migrate(context.Background(), database.AdminOptions{
		DatabasePath:  databasePath,
		MigrationsDir: migrationsDir,
		ScriptPath:    scriptPath,
	}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	configPath := filepath.Join(dir, "storage.yaml")
	configText := fmt.Sprintf("database_path: %s\nmigrations_dir: %s\n", databasePath, migrationsDir)
	if err := os.WriteFile(configPath, []byte(configText), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run(
		[]string{"--config", configPath, "--admin-script", scriptPath},
		&stdout,
		&stderr,
	)
	if exitCode != 0 {
		t.Fatalf("expected success, code=%d stderr=%s", exitCode, stderr.String())
	}

	var result database.StatusResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("parse output: %v: %s", err, stdout.String())
	}
	if result.SchemaVersion != 1 {
		t.Fatalf("expected schema version 1, got %d", result.SchemaVersion)
	}
	if result.Thresholds.DurableHardLimitBytes != 30_000_000_000 {
		t.Fatalf("unexpected hard limit: %d", result.Thresholds.DurableHardLimitBytes)
	}
	if result.Thresholds.TemporaryHardAbortBytes != 68_000_000_000 {
		t.Fatalf("unexpected temporary hard abort: %d", result.Thresholds.TemporaryHardAbortBytes)
	}
}
