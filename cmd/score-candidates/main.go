package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"argus/internal/candidate"
	"argus/internal/config"
	"argus/internal/relevance"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("score-candidates", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var scanCheckpointPath string
	var relevanceConfigPath string
	var outputPath string
	var scorerScript string
	var duckDBMemoryLimit string
	var duckDBThreads int
	var duckDBTempDir string
	var force bool

	flags.StringVar(&scanCheckpointPath, "scan-checkpoint", "", "completed candidate scan checkpoint")
	flags.StringVar(&relevanceConfigPath, "relevance-config", "configs/relevance/deterministic-v2.yaml", "path to deterministic relevance rules")
	flags.StringVar(&outputPath, "output-path", "", "relevance score Parquet output path")
	flags.StringVar(&scorerScript, "scorer-script", "scripts/dev/duckdb_score_candidates.py", "path to DuckDB scorer adapter")
	flags.StringVar(&duckDBMemoryLimit, "duckdb-memory-limit", "4GB", "DuckDB memory limit")
	flags.IntVar(&duckDBThreads, "duckdb-threads", 4, "DuckDB worker threads")
	flags.StringVar(&duckDBTempDir, "duckdb-temp-dir", filepath.Join(".duckdb", "tmp"), "DuckDB spill directory")
	flags.BoolVar(&force, "force", false, "replace an existing score output")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if scanCheckpointPath == "" {
		fmt.Fprintln(stderr, "scan-checkpoint is required")
		return 2
	}

	checkpoint, err := candidate.LoadScanCheckpoint(scanCheckpointPath)
	if err != nil {
		fmt.Fprintf(stderr, "load scan checkpoint: %v\n", err)
		return 1
	}
	if checkpoint.Status != "completed" {
		fmt.Fprintf(stderr, "scan checkpoint status must be completed, got %q\n", checkpoint.Status)
		return 1
	}
	if checkpoint.OutputPath == "" || checkpoint.OutputSHA256 == "" {
		fmt.Fprintln(stderr, "scan checkpoint does not contain candidate staging output")
		return 1
	}
	actualChecksum, err := candidate.FileSHA256(checkpoint.OutputPath)
	if err != nil {
		fmt.Fprintf(stderr, "checksum candidate staging: %v\n", err)
		return 1
	}
	if actualChecksum != checkpoint.OutputSHA256 {
		fmt.Fprintln(stderr, "candidate staging checksum does not match scan checkpoint")
		return 1
	}

	rules, err := config.LoadRelevanceConfig(relevanceConfigPath)
	if err != nil {
		fmt.Fprintf(stderr, "load relevance config: %v\n", err)
		return 1
	}
	if outputPath == "" {
		ext := filepath.Ext(checkpoint.OutputPath)
		base := strings.TrimSuffix(checkpoint.OutputPath, ext)
		outputPath = base + "-scores" + ext
	}
	if !force {
		if info, err := os.Stat(outputPath); err == nil && info.Size() > 0 {
			fmt.Fprintf(stderr, "score output already exists: %s; use --force to replace it\n", outputPath)
			return 1
		} else if err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(stderr, "inspect score output: %v\n", err)
			return 1
		}
	}

	result, err := relevance.Score(context.Background(), relevance.Options{
		InputPath:      checkpoint.OutputPath,
		OutputPath:     outputPath,
		ScriptPath:     scorerScript,
		TempDir:        duckDBTempDir,
		MemoryLimit:    duckDBMemoryLimit,
		Threads:        duckDBThreads,
		RelevanceRules: rules,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if result.RowsCandidates != checkpoint.Result.RowsCandidates {
		fmt.Fprintf(
			stderr,
			"score reconciliation failed: checkpoint candidates=%d scored candidates=%d\n",
			checkpoint.Result.RowsCandidates,
			result.RowsCandidates,
		)
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
