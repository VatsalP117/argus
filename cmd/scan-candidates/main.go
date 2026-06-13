package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"argus/internal/candidate"
	"argus/internal/config"
	"argus/internal/manifest"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("scan-candidates", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var manifestPath string
	var entryID string
	var candidateConfigPath string
	var outputPath string
	var scannerScript string
	var duckDBMemoryLimit string
	var duckDBThreads int
	var duckDBTempDir string
	var force bool

	flags.StringVar(&manifestPath, "manifest", "", "path to a pinned source manifest")
	flags.StringVar(&entryID, "entry-id", "", "single manifest entry to scan")
	flags.StringVar(&candidateConfigPath, "candidate-config", "configs/candidates/broad-v1.yaml", "path to broad candidate rules")
	flags.StringVar(&outputPath, "output-path", "", "candidate Parquet output path")
	flags.StringVar(&scannerScript, "scanner-script", "scripts/dev/duckdb_scan_candidates.py", "path to DuckDB scanner adapter")
	flags.StringVar(&duckDBMemoryLimit, "duckdb-memory-limit", "4GB", "DuckDB memory limit")
	flags.IntVar(&duckDBThreads, "duckdb-threads", 4, "DuckDB worker threads")
	flags.StringVar(&duckDBTempDir, "duckdb-temp-dir", filepath.Join(".duckdb", "tmp"), "DuckDB spill directory")
	flags.BoolVar(&force, "force", false, "replace an existing candidate output")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if manifestPath == "" {
		fmt.Fprintln(stderr, "manifest is required")
		return 2
	}
	if entryID == "" {
		fmt.Fprintln(stderr, "entry-id is required")
		return 2
	}

	sourceManifest, err := manifest.Load(manifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "load manifest: %v\n", err)
		return 1
	}
	entry, ok := findEntry(sourceManifest.Entries, entryID)
	if !ok {
		fmt.Fprintf(stderr, "manifest entry %q was not found\n", entryID)
		return 1
	}

	rules, err := config.LoadCandidateConfig(candidateConfigPath)
	if err != nil {
		fmt.Fprintf(stderr, "load candidate config: %v\n", err)
		return 1
	}

	if outputPath == "" {
		outputPath = filepath.Join(
			"data",
			"tmp",
			"candidates",
			sourceManifest.ManifestID,
			entry.EntryID+"-candidates.parquet",
		)
	}
	if !force {
		if info, err := os.Stat(outputPath); err == nil && info.Size() > 0 {
			fmt.Fprintf(stderr, "candidate output already exists: %s; use --force to replace it\n", outputPath)
			return 1
		} else if err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(stderr, "inspect candidate output: %v\n", err)
			return 1
		}
	}

	result, err := candidate.Scan(context.Background(), candidate.Options{
		Entry:          entry,
		ManifestID:     sourceManifest.ManifestID,
		DatasetRepo:    sourceManifest.DatasetRepo,
		OutputPath:     outputPath,
		ScriptPath:     scannerScript,
		TempDir:        duckDBTempDir,
		MemoryLimit:    duckDBMemoryLimit,
		Threads:        duckDBThreads,
		CandidateRules: rules,
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

func findEntry(entries []manifest.Entry, entryID string) (manifest.Entry, bool) {
	for _, entry := range entries {
		if entry.EntryID == entryID {
			return entry, true
		}
	}
	return manifest.Entry{}, false
}
