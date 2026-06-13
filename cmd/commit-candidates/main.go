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
	"argus/internal/localsecret"
	"argus/internal/manifest"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("commit-candidates", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var storageConfigPath string
	var manifestPath string
	var entryID string
	var scanCheckpointPath string
	var scorePath string
	var relevanceConfigPath string
	var authorHashSaltFile string
	var commitScript string

	flags.StringVar(&storageConfigPath, "storage-config", "configs/storage/local.yaml", "path to storage config")
	flags.StringVar(&manifestPath, "manifest", "", "path to the pinned source manifest")
	flags.StringVar(&entryID, "entry-id", "", "single manifest entry to commit")
	flags.StringVar(&scanCheckpointPath, "scan-checkpoint", "", "completed candidate scan checkpoint")
	flags.StringVar(&scorePath, "score-path", "", "deterministic relevance score Parquet")
	flags.StringVar(&relevanceConfigPath, "relevance-config", "configs/relevance/deterministic-v1.yaml", "path to relevance rules")
	flags.StringVar(&authorHashSaltFile, "author-hash-salt-file", filepath.Join("state", "secrets", "author-hash-salt"), "ignored local author hash salt file")
	flags.StringVar(&commitScript, "commit-script", "scripts/dev/duckdb_commit_candidates.py", "path to DuckDB commit adapter")
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

	storage, err := config.LoadStorageConfig(storageConfigPath)
	if err != nil {
		fmt.Fprintf(stderr, "load storage config: %v\n", err)
		return 1
	}
	if _, err := os.Stat(storage.DatabasePath); err != nil {
		fmt.Fprintf(stderr, "durable database is unavailable; run db-migrate: %v\n", err)
		return 1
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
	if scanCheckpointPath == "" {
		scanCheckpointPath = filepath.Join(
			"state",
			"checkpoints",
			"candidate-scan",
			sourceManifest.ManifestID,
			entry.EntryID+".json",
		)
	}
	checkpoint, err := candidate.LoadScanCheckpoint(scanCheckpointPath)
	if err != nil {
		fmt.Fprintf(stderr, "load scan checkpoint: %v\n", err)
		return 1
	}
	if checkpoint.ManifestID != sourceManifest.ManifestID ||
		checkpoint.EntryID != entry.EntryID ||
		checkpoint.SourceIdentity != entry.SourceIdentity {
		fmt.Fprintln(stderr, "scan checkpoint does not match the pinned manifest entry")
		return 1
	}
	if scorePath == "" {
		ext := filepath.Ext(checkpoint.OutputPath)
		scorePath = strings.TrimSuffix(checkpoint.OutputPath, ext) + "-scores" + ext
	}

	relevanceRules, err := config.LoadRelevanceConfig(relevanceConfigPath)
	if err != nil {
		fmt.Fprintf(stderr, "load relevance config: %v\n", err)
		return 1
	}
	manifestChecksum, err := candidate.FileSHA256(manifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "checksum manifest: %v\n", err)
		return 1
	}
	candidateChecksum, err := candidate.FileSHA256(checkpoint.OutputPath)
	if err != nil {
		fmt.Fprintf(stderr, "checksum candidate staging: %v\n", err)
		return 1
	}
	if candidateChecksum != checkpoint.OutputSHA256 {
		fmt.Fprintln(stderr, "candidate staging checksum does not match scan checkpoint")
		return 1
	}
	scoreChecksum, err := candidate.FileSHA256(scorePath)
	if err != nil {
		fmt.Fprintf(stderr, "checksum relevance staging: %v\n", err)
		return 1
	}
	relevanceConfigHash, err := candidate.FileSHA256(relevanceConfigPath)
	if err != nil {
		fmt.Fprintf(stderr, "checksum relevance config: %v\n", err)
		return 1
	}
	authorHashSalt, err := localsecret.Ensure(authorHashSaltFile)
	if err != nil {
		fmt.Fprintf(stderr, "prepare author hash salt: %v\n", err)
		return 1
	}

	result, err := candidate.Commit(context.Background(), candidate.CommitOptions{
		DatabasePath:        storage.DatabasePath,
		CandidatePath:       checkpoint.OutputPath,
		CandidateChecksum:   candidateChecksum,
		ScorePath:           scorePath,
		ScoreChecksum:       scoreChecksum,
		ManifestPath:        manifestPath,
		ManifestChecksum:    manifestChecksum,
		SourceManifest:      sourceManifest,
		ManifestEntry:       entry,
		ScanCheckpoint:      checkpoint,
		RelevanceRules:      relevanceRules,
		RelevanceConfigHash: relevanceConfigHash,
		AuthorHashSalt:      authorHashSalt,
		ScriptPath:          commitScript,
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
