package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type StorageConfig struct {
	DatabasePath         string `yaml:"database_path"`
	MigrationsDir        string `yaml:"migrations_dir"`
	SnapshotsDir         string `yaml:"snapshots_dir"`
	MinimumFreeDiskBytes int64  `yaml:"minimum_free_disk_bytes"`
	Durable              struct {
		WarnBytes            int64 `yaml:"warn_bytes"`
		StopWideningBytes    int64 `yaml:"stop_widening_bytes"`
		RetentionReviewBytes int64 `yaml:"retention_review_bytes"`
		HardLimitBytes       int64 `yaml:"hard_limit_bytes"`
	} `yaml:"durable"`
	Temporary struct {
		WarnBytes         int64 `yaml:"warn_bytes"`
		StopNewBatchBytes int64 `yaml:"stop_new_batch_bytes"`
		HardAbortBytes    int64 `yaml:"hard_abort_bytes"`
		MaxStagingBytes   int64 `yaml:"max_staging_bytes"`
	} `yaml:"temporary"`
}

func DefaultStorageConfig() StorageConfig {
	var cfg StorageConfig
	cfg.DatabasePath = filepath.Join("data", "argus.duckdb")
	cfg.MigrationsDir = filepath.Join("sql", "migrations")
	cfg.SnapshotsDir = filepath.Join("data", "snapshots")
	cfg.MinimumFreeDiskBytes = 10_000_000_000
	cfg.Durable.WarnBytes = 21_000_000_000
	cfg.Durable.StopWideningBytes = 24_000_000_000
	cfg.Durable.RetentionReviewBytes = 27_000_000_000
	cfg.Durable.HardLimitBytes = 30_000_000_000
	cfg.Temporary.WarnBytes = 55_000_000_000
	cfg.Temporary.StopNewBatchBytes = 62_000_000_000
	cfg.Temporary.HardAbortBytes = 68_000_000_000
	cfg.Temporary.MaxStagingBytes = 8_000_000_000
	return cfg
}

func LoadStorageConfig(path string) (StorageConfig, error) {
	cfg := DefaultStorageConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (cfg StorageConfig) Validate() error {
	if cfg.DatabasePath == "" {
		return fmt.Errorf("database_path is required")
	}
	if cfg.MigrationsDir == "" {
		return fmt.Errorf("migrations_dir is required")
	}
	if cfg.MinimumFreeDiskBytes <= 0 {
		return fmt.Errorf("minimum_free_disk_bytes must be positive")
	}
	if !(0 < cfg.Durable.WarnBytes &&
		cfg.Durable.WarnBytes < cfg.Durable.StopWideningBytes &&
		cfg.Durable.StopWideningBytes < cfg.Durable.RetentionReviewBytes &&
		cfg.Durable.RetentionReviewBytes < cfg.Durable.HardLimitBytes) {
		return fmt.Errorf("durable thresholds must increase from warn through hard limit")
	}
	if !(0 < cfg.Temporary.WarnBytes &&
		cfg.Temporary.WarnBytes < cfg.Temporary.StopNewBatchBytes &&
		cfg.Temporary.StopNewBatchBytes < cfg.Temporary.HardAbortBytes) {
		return fmt.Errorf("temporary thresholds must increase from warn through hard abort")
	}
	if cfg.Temporary.MaxStagingBytes <= 0 {
		return fmt.Errorf("temporary max_staging_bytes must be positive")
	}
	return nil
}
