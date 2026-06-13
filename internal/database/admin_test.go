package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateCreatesDatabaseAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	migrationsDir := filepath.Join(dir, "migrations")
	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		t.Fatalf("create migrations dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(migrationsDir, "001_create_example.sql"),
		[]byte("CREATE TABLE example (id BIGINT PRIMARY KEY);"),
		0o644,
	); err != nil {
		t.Fatalf("write migration: %v", err)
	}

	options := AdminOptions{
		DatabasePath:  filepath.Join(dir, "argus.duckdb"),
		MigrationsDir: migrationsDir,
		ScriptPath:    filepath.Join("..", "..", "scripts", "dev", "duckdb_admin.py"),
	}

	first, err := Migrate(context.Background(), options)
	if err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if first.SchemaVersion != 1 {
		t.Fatalf("expected schema version 1, got %d", first.SchemaVersion)
	}
	if len(first.AppliedMigrations) != 1 || first.AppliedMigrations[0] != 1 {
		t.Fatalf("expected migration 1 to be applied, got %v", first.AppliedMigrations)
	}

	second, err := Migrate(context.Background(), options)
	if err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if second.SchemaVersion != 1 {
		t.Fatalf("expected schema version 1, got %d", second.SchemaVersion)
	}
	if len(second.AppliedMigrations) != 0 {
		t.Fatalf("expected no migrations on second run, got %v", second.AppliedMigrations)
	}
}

func TestStatusReportsSchemaAndCapacityState(t *testing.T) {
	dir := t.TempDir()
	migrationsDir := filepath.Join(dir, "migrations")
	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		t.Fatalf("create migrations dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(migrationsDir, "001_create_example.sql"),
		[]byte("CREATE TABLE example (id BIGINT PRIMARY KEY);"),
		0o644,
	); err != nil {
		t.Fatalf("write migration: %v", err)
	}

	options := AdminOptions{
		DatabasePath:  filepath.Join(dir, "argus.duckdb"),
		MigrationsDir: migrationsDir,
		ScriptPath:    filepath.Join("..", "..", "scripts", "dev", "duckdb_admin.py"),
	}
	if _, err := Migrate(context.Background(), options); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	result, err := Status(context.Background(), options, CapacityPolicy{
		DurableWarnBytes:            1,
		DurableStopWideningBytes:    1_000_000_000,
		DurableRetentionReviewBytes: 2_000_000_000,
		DurableHardLimitBytes:       3_000_000_000,
		MinimumFreeDiskBytes:        1,
	})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !result.DatabaseExists {
		t.Fatal("expected database to exist")
	}
	if result.SchemaVersion != 1 {
		t.Fatalf("expected schema version 1, got %d", result.SchemaVersion)
	}
	if result.DatabaseSizeBytes <= 0 {
		t.Fatalf("expected positive database size, got %d", result.DatabaseSizeBytes)
	}
	if result.DurableState != "warning" {
		t.Fatalf("expected warning capacity state, got %s", result.DurableState)
	}
	if !result.CanStartNewBatch {
		t.Fatal("expected capacity policy to allow a new batch")
	}
}
