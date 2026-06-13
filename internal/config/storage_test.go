package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStorageConfigAppliesSafeDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "storage.yaml")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadStorageConfig(path)
	if err != nil {
		t.Fatalf("load storage config: %v", err)
	}

	if cfg.DatabasePath != filepath.Join("data", "argus.duckdb") {
		t.Fatalf("unexpected database path: %s", cfg.DatabasePath)
	}
	if cfg.Durable.WarnBytes != 21_000_000_000 {
		t.Fatalf("unexpected durable warning threshold: %d", cfg.Durable.WarnBytes)
	}
	if cfg.Durable.StopWideningBytes != 24_000_000_000 {
		t.Fatalf("unexpected durable stop threshold: %d", cfg.Durable.StopWideningBytes)
	}
	if cfg.Durable.RetentionReviewBytes != 27_000_000_000 {
		t.Fatalf("unexpected durable review threshold: %d", cfg.Durable.RetentionReviewBytes)
	}
	if cfg.Durable.HardLimitBytes != 30_000_000_000 {
		t.Fatalf("unexpected durable hard limit: %d", cfg.Durable.HardLimitBytes)
	}
	if cfg.Temporary.HardAbortBytes != 68_000_000_000 {
		t.Fatalf("unexpected temporary hard abort threshold: %d", cfg.Temporary.HardAbortBytes)
	}
	if cfg.MinimumFreeDiskBytes != 10_000_000_000 {
		t.Fatalf("unexpected minimum free disk: %d", cfg.MinimumFreeDiskBytes)
	}
}
