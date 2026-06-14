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

	flags.StringVar(&labelsPath, "labels", "", "completed relevance label fixture CSV")
	flags.StringVar(&scorePath, "score-path", "", "relevance score Parquet to evaluate")
	flags.StringVar(&evaluatorScript, "evaluator-script", "scripts/dev/duckdb_evaluate_relevance.py", "DuckDB relevance evaluator")
	flags.Float64Var(&minimumRetainPrecision, "minimum-retain-precision", 0.70, "candidate retained-precision gate")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if labelsPath == "" || scorePath == "" {
		fmt.Fprintln(stderr, "labels and score-path are required")
		return 2
	}

	result, err := relevance.Evaluate(context.Background(), relevance.EvaluationOptions{
		LabelsPath:             labelsPath,
		ScorePath:              scorePath,
		ScriptPath:             evaluatorScript,
		MinimumRetainPrecision: minimumRetainPrecision,
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
