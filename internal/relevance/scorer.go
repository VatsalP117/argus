package relevance

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"argus/internal/config"
)

type Options struct {
	InputPath      string
	OutputPath     string
	ScriptPath     string
	TempDir        string
	MemoryLimit    string
	Threads        int
	RelevanceRules config.RelevanceConfig
}

type ScoreResult struct {
	Status                   string           `json:"status"`
	RowsCandidates           int64            `json:"rows_candidates"`
	RowsScored               int64            `json:"rows_scored"`
	RowsRetainedCandidates   int64            `json:"rows_retained_candidates"`
	RowsEvaluationCandidates int64            `json:"rows_evaluation_candidates"`
	RowsDiscardedCandidates  int64            `json:"rows_discarded_candidates"`
	TierCounts               map[string]int64 `json:"tier_counts"`
	OutputPath               string           `json:"output_path"`
	BytesWritten             int64            `json:"bytes_written"`
	RelevanceVersion         string           `json:"relevance_version"`
}

func Score(ctx context.Context, options Options) (ScoreResult, error) {
	var result ScoreResult

	if err := options.RelevanceRules.Validate(); err != nil {
		return result, fmt.Errorf("validate relevance rules: %w", err)
	}
	rulesJSON, err := json.Marshal(options.RelevanceRules)
	if err != nil {
		return result, fmt.Errorf("encode relevance rules: %w", err)
	}

	scriptPath := options.ScriptPath
	if scriptPath == "" {
		scriptPath = "scripts/dev/duckdb_score_candidates.py"
	}
	memoryLimit := options.MemoryLimit
	if memoryLimit == "" {
		memoryLimit = "4GB"
	}
	threads := options.Threads
	if threads <= 0 {
		threads = 4
	}
	tempDir := options.TempDir
	if tempDir == "" {
		tempDir = ".duckdb/tmp"
	}

	cmd := exec.CommandContext(
		ctx,
		"python3",
		scriptPath,
		"--input-path", options.InputPath,
		"--output-path", options.OutputPath,
		"--rules-json", string(rulesJSON),
		"--duckdb-memory-limit", memoryLimit,
		"--duckdb-threads", fmt.Sprintf("%d", threads),
		"--duckdb-temp-dir", tempDir,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return result, fmt.Errorf("candidate scoring failed: %w: %s", err, output)
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return result, fmt.Errorf("parse candidate scoring result: %w: %s", err, output)
	}
	if result.Status != "completed" {
		return result, fmt.Errorf("candidate scoring returned status %q", result.Status)
	}
	return result, nil
}
