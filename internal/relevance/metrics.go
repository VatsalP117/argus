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
	MinimumWeightedRecall  float64
	MinimumDomainPrecision float64
	MinimumDomainRetainedCount int64
	MaxFalsePositiveCategoryRate float64
}

type StratumMetrics struct {
	Population          int64   `json:"population"`
	SampleSize          int64   `json:"sample_size"`
	PositiveCount       int64   `json:"positive_count"`
	SampledPositiveRate float64 `json:"sampled_positive_rate"`
}

type ClassificationMetrics struct {
	LabeledPositive              int64   `json:"labeled_positive"`
	RetainedPredictions          int64   `json:"retained_predictions"`
	TruePositiveRetained         int64   `json:"true_positive_retained"`
	FalsePositiveRetained        int64   `json:"false_positive_retained"`
	FalseNegativeRetained        int64   `json:"false_negative_retained"`
	RetainedPrecision            float64 `json:"retained_precision"`
	ExactRetainedPrecision       float64 `json:"exact_retained_precision"`
	RetainedRecall               float64 `json:"retained_recall"`
	FixtureRecall                float64 `json:"fixture_recall"`
	EstimatedTotalRelevant       float64 `json:"estimated_total_relevant"`
	WeightedRetainedRecallEstimate float64 `json:"weighted_retained_recall_estimate"`
	F1                           float64 `json:"f1"`
}

type EvaluationResult struct {
	Status                                  string                           `json:"status"`
	RelevanceVersion                        string                           `json:"relevance_version"`
	RowsLabeled                             int64                            `json:"rows_labeled"`
	Domains                                 map[string]ClassificationMetrics `json:"domains"`
	Candidate                               ClassificationMetrics            `json:"candidate"`
	Strata                                  map[string]StratumMetrics        `json:"strata"`
	TrapLeakage                             map[string]int64                 `json:"trap_leakage"`
	MissingSourceURLCount                   int64                            `json:"missing_source_url_count"`
	VisaRetainedFalsePositives              int64                            `json:"visa_retained_false_positives"`
	PromotionBotRetainedFalsePositives      int64                            `json:"promotion_bot_retained_false_positives"`
	FalsePositiveCategoryRateViolations     map[string]int64                 `json:"false_positive_category_rate_violations"`
	DomainPrecisionFailures                 map[string]ClassificationMetrics `json:"domain_precision_failures"`
	MinimumRetainPrecision                  float64                          `json:"minimum_retain_precision"`
	MinimumWeightedRecall                   float64                          `json:"minimum_weighted_recall"`
	QualityGatePassed                       bool                             `json:"quality_gate_passed"`
}

func Evaluate(ctx context.Context, options EvaluationOptions) (EvaluationResult, error) {
	var result EvaluationResult

	if options.LabelsPath == "" || options.ScorePath == "" {
		return result, fmt.Errorf("labels and score paths are required")
	}
	if options.MinimumRetainPrecision <= 0 || options.MinimumRetainPrecision > 1 {
		return result, fmt.Errorf("minimum retain precision must be within (0, 1]")
	}
	if options.MinimumWeightedRecall == 0 {
		options.MinimumWeightedRecall = 0.60
	}
	if options.MinimumDomainPrecision == 0 {
		options.MinimumDomainPrecision = 0.60
	}
	if options.MinimumDomainRetainedCount == 0 {
		options.MinimumDomainRetainedCount = 10
	}
	if options.MaxFalsePositiveCategoryRate == 0 {
		options.MaxFalsePositiveCategoryRate = 0.20
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
		"--minimum-weighted-recall", fmt.Sprintf("%g", options.MinimumWeightedRecall),
		"--minimum-domain-precision", fmt.Sprintf("%g", options.MinimumDomainPrecision),
		"--minimum-domain-retained-count", fmt.Sprintf("%d", options.MinimumDomainRetainedCount),
		"--max-false-positive-category-rate", fmt.Sprintf("%g", options.MaxFalsePositiveCategoryRate),
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
