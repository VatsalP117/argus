package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"argus/internal/relevance"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("evaluate-relevance", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var labelsPath string
	var scorePath string
	var evaluatorScript string
	var minimumRetainPrecision float64
	var minimumWeightedRecall float64
	var minimumDomainPrecision float64
	var minimumDomainRetainedCount int64
	var maxFalsePositiveCategoryRate float64

	flags.StringVar(&labelsPath, "labels", "", "completed relevance label fixture CSV")
	flags.StringVar(&scorePath, "score-path", "", "relevance score Parquet to evaluate")
	flags.StringVar(&evaluatorScript, "evaluator-script", "scripts/dev/duckdb_evaluate_relevance.py", "DuckDB relevance evaluator")
	flags.Float64Var(&minimumRetainPrecision, "minimum-retain-precision", 0.70, "candidate retained-precision gate")
	flags.Float64Var(&minimumWeightedRecall, "minimum-weighted-recall", 0.60, "weighted retained-recall gate")
	flags.Float64Var(&minimumDomainPrecision, "minimum-domain-precision", 0.60, "per-domain retained-precision gate")
	flags.Int64Var(&minimumDomainRetainedCount, "minimum-domain-retained-count", 10, "minimum retained predictions for domain gate")
	flags.Float64Var(&maxFalsePositiveCategoryRate, "max-false-positive-category-rate", 0.20, "maximum share of retained rows in one false-positive category")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if labelsPath == "" || scorePath == "" {
		fmt.Fprintln(stderr, "labels and score-path are required")
		return 2
	}

	result, err := relevance.Evaluate(context.Background(), relevance.EvaluationOptions{
		LabelsPath:                   labelsPath,
		ScorePath:                    scorePath,
		ScriptPath:                   evaluatorScript,
		MinimumRetainPrecision:       minimumRetainPrecision,
		MinimumWeightedRecall:        minimumWeightedRecall,
		MinimumDomainPrecision:       minimumDomainPrecision,
		MinimumDomainRetainedCount:   minimumDomainRetainedCount,
		MaxFalsePositiveCategoryRate: maxFalsePositiveCategoryRate,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	if !result.QualityGatePassed {
		return 3
	}
	return 0
}
