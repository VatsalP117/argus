package relevance

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

type EvaluationExportOptions struct {
	CandidatePath    string
	ScorePath        string
	OutputPath       string
	ScriptPath       string
	SamplePerStratum int
	Seed             string
}

type EvaluationExportResult struct {
	Status        string           `json:"status"`
	RowsExported  int64            `json:"rows_exported"`
	StratumCounts map[string]int64 `json:"stratum_counts"`
	OutputPath    string           `json:"output_path"`
}

func ExportEvaluation(ctx context.Context, options EvaluationExportOptions) (EvaluationExportResult, error) {
	var result EvaluationExportResult

	if options.CandidatePath == "" || options.ScorePath == "" || options.OutputPath == "" {
		return result, fmt.Errorf("candidate, score, and output paths are required")
	}
	if options.SamplePerStratum <= 0 {
		return result, fmt.Errorf("sample per stratum must be positive")
	}
	if options.Seed == "" {
		return result, fmt.Errorf("evaluation seed is required")
	}

	scriptPath := options.ScriptPath
	if scriptPath == "" {
		scriptPath = "scripts/dev/duckdb_export_relevance_eval.py"
	}
	cmd := exec.CommandContext(
		ctx,
		"python3",
		scriptPath,
		"--candidate-path", options.CandidatePath,
		"--score-path", options.ScorePath,
		"--output-path", options.OutputPath,
		"--sample-per-stratum", fmt.Sprintf("%d", options.SamplePerStratum),
		"--seed", options.Seed,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return result, fmt.Errorf("export relevance evaluation failed: %w: %s", err, output)
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return result, fmt.Errorf("parse relevance evaluation result: %w: %s", err, output)
	}
	if result.Status != "completed" {
		return result, fmt.Errorf("relevance evaluation export returned status %q", result.Status)
	}
	return result, nil
}
