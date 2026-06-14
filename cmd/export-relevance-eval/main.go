package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"argus/internal/candidate"
	"argus/internal/relevance"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("export-relevance-eval", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var scanCheckpointPath string
	var scorePath string
	var outputPath string
	var exporterScript string
	var seed string
	var samplePerStratum int
	var retainSample int
	var evaluateSample int
	var discardSample int
	var force bool

	flags.StringVar(&scanCheckpointPath, "scan-checkpoint", "", "completed candidate scan checkpoint")
	flags.StringVar(&scorePath, "score-path", "", "candidate relevance score Parquet")
	flags.StringVar(&outputPath, "output-path", "", "analyst label fixture CSV")
	flags.StringVar(&exporterScript, "exporter-script", "scripts/dev/duckdb_export_relevance_eval.py", "DuckDB evaluation exporter")
	flags.StringVar(&seed, "seed", "relevance-eval-v1", "stable sampling seed")
	flags.IntVar(&samplePerStratum, "sample-per-stratum", 100, "rows sampled from each decision stratum")
	flags.IntVar(&retainSample, "retain-sample", 0, "retained rows to export; 0 means all, -1 uses sample-per-stratum")
	flags.IntVar(&evaluateSample, "evaluate-sample", -1, "evaluate rows to export; 0 means all, -1 uses sample-per-stratum")
	flags.IntVar(&discardSample, "discard-sample", -1, "discard rows to export; 0 means all, -1 uses sample-per-stratum")
	flags.BoolVar(&force, "force", false, "replace an existing label fixture")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if scanCheckpointPath == "" || scorePath == "" || outputPath == "" {
		fmt.Fprintln(stderr, "scan-checkpoint, score-path, and output-path are required")
		return 2
	}

	checkpoint, err := candidate.LoadScanCheckpoint(scanCheckpointPath)
	if err != nil {
		fmt.Fprintf(stderr, "load scan checkpoint: %v\n", err)
		return 1
	}
	if checkpoint.Status != "completed" || checkpoint.OutputPath == "" || checkpoint.OutputSHA256 == "" {
		fmt.Fprintln(stderr, "scan checkpoint does not contain completed candidate staging")
		return 1
	}
	checksum, err := candidate.FileSHA256(checkpoint.OutputPath)
	if err != nil {
		fmt.Fprintf(stderr, "checksum candidate staging: %v\n", err)
		return 1
	}
	if checksum != checkpoint.OutputSHA256 {
		fmt.Fprintln(stderr, "candidate staging checksum does not match scan checkpoint")
		return 1
	}
	if info, err := os.Stat(scorePath); err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		fmt.Fprintf(stderr, "score input is not a non-empty regular file: %s\n", scorePath)
		return 1
	}
	if !force {
		if info, err := os.Stat(outputPath); err == nil && info.Size() > 0 {
			fmt.Fprintf(stderr, "evaluation output already exists: %s; use --force to replace it\n", outputPath)
			return 1
		} else if err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(stderr, "inspect evaluation output: %v\n", err)
			return 1
		}
	}

	result, err := relevance.ExportEvaluation(context.Background(), relevance.EvaluationExportOptions{
		CandidatePath:    checkpoint.OutputPath,
		ScorePath:        scorePath,
		OutputPath:       outputPath,
		ScriptPath:       exporterScript,
		SamplePerStratum: samplePerStratum,
		RetainSample:     retainSample,
		EvaluateSample:   evaluateSample,
		DiscardSample:    discardSample,
		Seed:             seed,
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
	return 0
}
