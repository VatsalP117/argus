package candidate

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"argus/internal/config"
	"argus/internal/manifest"
)

type Options struct {
	Entry          manifest.Entry
	ManifestID     string
	DatasetRepo    string
	OutputPath     string
	ScriptPath     string
	TempDir        string
	MemoryLimit    string
	Threads        int
	CandidateRules config.CandidateConfig
}

type ScanResult struct {
	Status                   string           `json:"status"`
	EntryID                  string           `json:"entry_id"`
	RecordType               string           `json:"record_type"`
	RowsSeen                 int64            `json:"rows_seen"`
	RowsCandidates           int64            `json:"rows_candidates"`
	RowsRejectedEarly        int64            `json:"rows_rejected_early"`
	BytesWritten             int64            `json:"bytes_written"`
	OutputPath               string           `json:"output_path"`
	CandidateVersion         string           `json:"candidate_version"`
	MatchedByGroup           map[string]int64 `json:"matched_by_group"`
	SubredditPriorCandidates int64            `json:"subreddit_prior_candidates"`
}

func Scan(ctx context.Context, options Options) (ScanResult, error) {
	var result ScanResult

	if err := options.CandidateRules.Validate(); err != nil {
		return result, fmt.Errorf("validate candidate rules: %w", err)
	}
	if options.Entry.SourceIdentity == "" || options.Entry.ArchiveRevision == "" {
		return result, fmt.Errorf("manifest entry %q does not have pinned source identity", options.Entry.EntryID)
	}
	if options.Entry.RecordType != "comments" && options.Entry.RecordType != "submissions" {
		return result, fmt.Errorf("unsupported record type %q", options.Entry.RecordType)
	}

	rulesJSON, err := json.Marshal(options.CandidateRules)
	if err != nil {
		return result, fmt.Errorf("encode candidate rules: %w", err)
	}

	scriptPath := options.ScriptPath
	if scriptPath == "" {
		scriptPath = "scripts/dev/duckdb_scan_candidates.py"
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
		"--input-url", options.Entry.ResolveURL,
		"--output-path", options.OutputPath,
		"--record-type", options.Entry.RecordType,
		"--entry-id", options.Entry.EntryID,
		"--manifest-id", options.ManifestID,
		"--dataset-repo", options.DatasetRepo,
		"--archive-revision", options.Entry.ArchiveRevision,
		"--source-path", options.Entry.ShardPath,
		"--source-identity", options.Entry.SourceIdentity,
		"--rules-json", string(rulesJSON),
		"--duckdb-memory-limit", memoryLimit,
		"--duckdb-threads", fmt.Sprintf("%d", threads),
		"--duckdb-temp-dir", tempDir,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return result, fmt.Errorf("candidate scan failed for %s: %w: %s", options.Entry.EntryID, err, output)
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return result, fmt.Errorf("parse candidate scan result for %s: %w: %s", options.Entry.EntryID, err, output)
	}
	if result.Status != "completed" && result.Status != "completed_zero_rows" {
		return result, fmt.Errorf("candidate scan returned status %q", result.Status)
	}
	return result, nil
}
