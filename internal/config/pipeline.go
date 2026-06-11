package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type PipelineConfig struct {
	PipelineName string `yaml:"pipeline_name"`
	PhaseTarget  string `yaml:"phase_target"`
	Domain       string `yaml:"domain"`
	Archive      struct {
		Repo      string `yaml:"repo"`
		Transport string `yaml:"transport"`
	} `yaml:"archive"`
	RecordTypes []string `yaml:"record_types"`
	Months      []string `yaml:"months"`
	SmokeMonths []string `yaml:"smoke_months"`
	Subreddits  []string `yaml:"subreddits"`
	Output      struct {
		RawDir     string `yaml:"raw_dir"`
		CleanDir   string `yaml:"clean_dir"`
		MartsDir   string `yaml:"marts_dir"`
		ExportsDir string `yaml:"exports_dir"`
	} `yaml:"output"`
	State struct {
		CheckpointsDir string `yaml:"checkpoints_dir"`
		RunsDir        string `yaml:"runs_dir"`
		LogsDir        string `yaml:"logs_dir"`
	} `yaml:"state"`
	Safety struct {
		AbortIfTotalRawBytesGT                 int64 `yaml:"abort_if_total_raw_bytes_gt"`
		AbortIfSmokeRunBytesGT                 int64 `yaml:"abort_if_smoke_run_bytes_gt"`
		AbortIfMonthCountGTWithoutConfirmation int   `yaml:"abort_if_month_count_gt_without_confirmation"`
	} `yaml:"safety"`
	Quality struct {
		RequireSourceFile   bool `yaml:"require_source_file"`
		RequireIngestedAt   bool `yaml:"require_ingested_at"`
		QuarantineBadShards bool `yaml:"quarantine_bad_shards"`
	} `yaml:"quality"`
	Notes []string `yaml:"notes"`
}

func LoadPipelineConfig(path string) (PipelineConfig, error) {
	var cfg PipelineConfig

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}
