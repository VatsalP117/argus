package candidate

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"argus/internal/config"
	"argus/internal/manifest"
)

type CommitOptions struct {
	DatabasePath        string
	CandidatePath       string
	CandidateChecksum   string
	ScorePath           string
	ScoreChecksum       string
	ManifestPath        string
	ManifestChecksum    string
	SourceManifest      manifest.Manifest
	ManifestEntry       manifest.Entry
	ScanCheckpoint      ScanCheckpoint
	RelevanceRules      config.RelevanceConfig
	RelevanceConfigHash string
	AuthorHashSalt      string
	ScriptPath          string
}

type CommitResult struct {
	Status               string `json:"status"`
	ScanRunID            string `json:"scan_run_id"`
	StagingBatchID       string `json:"staging_batch_id"`
	IngestBatchID        string `json:"ingest_batch_id"`
	RowsSeen             int64  `json:"rows_seen"`
	RowsRejectedEarly    int64  `json:"rows_rejected_early"`
	RowsStaged           int64  `json:"rows_staged"`
	RowsRetained         int64  `json:"rows_retained"`
	RowsRejectedLate     int64  `json:"rows_rejected_late"`
	RowsQuarantined      int64  `json:"rows_quarantined"`
	RelevanceRows        int64  `json:"relevance_rows"`
	SignalsWritten       int64  `json:"signals_written"`
	EntitiesWritten      int64  `json:"entities_written"`
	SourceEquationValid  bool   `json:"source_equation_valid"`
	StagingEquationValid bool   `json:"staging_equation_valid"`
	DurableChecksum      string `json:"durable_checksum"`
	CleanupStatus        string `json:"cleanup_status"`
}

type commitMetadata struct {
	CandidateChecksum   string                 `json:"candidate_checksum"`
	ScoreChecksum       string                 `json:"score_checksum"`
	ManifestPath        string                 `json:"manifest_path"`
	ManifestChecksum    string                 `json:"manifest_checksum"`
	Manifest            manifest.Manifest      `json:"manifest"`
	Entry               manifest.Entry         `json:"entry"`
	Checkpoint          ScanCheckpoint         `json:"checkpoint"`
	RelevanceRules      config.RelevanceConfig `json:"relevance_rules"`
	RelevanceConfigHash string                 `json:"relevance_config_hash"`
	AuthorHashSalt      string                 `json:"author_hash_salt"`
}

func Commit(ctx context.Context, options CommitOptions) (CommitResult, error) {
	var result CommitResult

	if err := options.RelevanceRules.Validate(); err != nil {
		return result, fmt.Errorf("validate relevance rules: %w", err)
	}
	if options.AuthorHashSalt == "" {
		return result, fmt.Errorf("author hash salt is required")
	}
	if options.ScanCheckpoint.Status != "completed" {
		return result, fmt.Errorf("scan checkpoint status must be completed")
	}
	if options.ScanCheckpoint.ManifestID != options.SourceManifest.ManifestID ||
		options.ScanCheckpoint.EntryID != options.ManifestEntry.EntryID ||
		options.ScanCheckpoint.SourceIdentity != options.ManifestEntry.SourceIdentity {
		return result, fmt.Errorf("scan checkpoint does not match manifest entry")
	}
	if options.ScanCheckpoint.OutputPath != options.CandidatePath ||
		options.ScanCheckpoint.OutputSHA256 != options.CandidateChecksum {
		return result, fmt.Errorf("candidate staging does not match scan checkpoint")
	}
	for path, expected := range map[string]string{
		options.CandidatePath: options.CandidateChecksum,
		options.ScorePath:     options.ScoreChecksum,
		options.ManifestPath:  options.ManifestChecksum,
	} {
		actual, err := FileSHA256(path)
		if err != nil {
			return result, fmt.Errorf("checksum %s: %w", path, err)
		}
		if actual != expected {
			return result, fmt.Errorf("checksum mismatch for %s", path)
		}
	}

	metadataJSON, err := json.Marshal(commitMetadata{
		CandidateChecksum:   options.CandidateChecksum,
		ScoreChecksum:       options.ScoreChecksum,
		ManifestPath:        options.ManifestPath,
		ManifestChecksum:    options.ManifestChecksum,
		Manifest:            options.SourceManifest,
		Entry:               options.ManifestEntry,
		Checkpoint:          options.ScanCheckpoint,
		RelevanceRules:      options.RelevanceRules,
		RelevanceConfigHash: options.RelevanceConfigHash,
		AuthorHashSalt:      options.AuthorHashSalt,
	})
	if err != nil {
		return result, fmt.Errorf("encode commit metadata: %w", err)
	}

	scriptPath := options.ScriptPath
	if scriptPath == "" {
		scriptPath = "scripts/dev/duckdb_commit_candidates.py"
	}
	cmd := exec.CommandContext(
		ctx,
		"python3",
		scriptPath,
		"--database-path", options.DatabasePath,
		"--candidate-path", options.CandidatePath,
		"--score-path", options.ScorePath,
		"--metadata-json", string(metadataJSON),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return result, fmt.Errorf("candidate commit failed: %w: %s", err, output)
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return result, fmt.Errorf("parse candidate commit result: %w: %s", err, output)
	}
	if result.Status != "completed" && result.Status != "skipped_existing" {
		return result, fmt.Errorf("candidate commit returned status %q", result.Status)
	}
	return result, nil
}
