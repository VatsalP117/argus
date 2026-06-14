package relevance

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

type EvaluationOptions struct {
	LabelsPath             string
	ScorePath              string
	ScriptPath             string
	MinimumRetainPrecision float64
}

type ClassificationMetrics struct {
	LabeledPositive       int64   `json:"labeled_positive"`
	RetainedPredictions   int64   `json:"retained_predictions"`
	TruePositiveRetained  int64   `json:"true_positive_retained"`
	FalsePositiveRetained int64   `json:"false_positive_retained"`
	FalseNegativeRetained int64   `json:"false_negative_retained"`
	RetainedPrecision     float64 `json:"retained_precision"`
	RetainedRecall        float64 `json:"retained_recall"`
	F1                    float64 `json:"f1"`
}

type EvaluationResult struct {
	Status            string                           `json:"status"`
	RelevanceVersion  string                           `json:"relevance_version"`
	RowsLabeled       int64                            `json:"rows_labeled"`
	Domains           map[string]ClassificationMetrics `json:"domains"`
	Candidate         ClassificationMetrics            `json:"candidate"`
	TrapLeakage       map[string]int64                 `json:"trap_leakage"`
	MinimumPrecision  float64                          `json:"minimum_retain_precision"`
	QualityGatePassed bool                             `json:"quality_gate_passed"`
}

func Evaluate(ctx context.Context, options EvaluationOptions) (EvaluationResult, error) {
	var result EvaluationResult

	if options.LabelsPath == "" || options.ScorePath == "" {
		return result, fmt.Errorf("labels and score paths are required")
	}
	if options.MinimumRetainPrecision <= 0 || options.MinimumRetainPrecision > 1 {
		return result, fmt.Errorf("minimum retain precision must be within (0, 1]")
	}
	scriptPath := options.ScriptPath
	if scriptPath == "" {
		scriptPath = "scripts/dev/duckdb_evaluate_relevance.py"
	}
	cmd := exec.CommandContext(
		ctx,
		"python3",
		scriptPath,
		"--labels-path", options.LabelsPath,
		"--score-path", options.ScorePath,
		"--minimum-retain-precision", fmt.Sprintf("%g", options.MinimumRetainPrecision),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return result, fmt.Errorf("evaluate relevance failed: %w: %s", err, output)
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return result, fmt.Errorf("parse relevance evaluation result: %w: %s", err, output)
	}
	if result.Status != "completed" {
		return result, fmt.Errorf("relevance evaluation returned status %q", result.Status)
	}
	return result, nil
}
