package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"argus/internal/candidate"
	"argus/internal/relevance"
)

func TestRunScoresChecksumValidatedCandidateStaging(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "candidates.parquet")
	createCandidateFixture(t, inputPath)
	inputChecksum, err := candidate.FileSHA256(inputPath)
	if err != nil {
		t.Fatalf("checksum candidate fixture: %v", err)
	}

	checkpointPath := filepath.Join(dir, "scan-checkpoint.json")
	if err := candidate.WriteScanCheckpoint(checkpointPath, candidate.ScanCheckpoint{
		Status:              "completed",
		ManifestID:          "manifest-123",
		EntryID:             "comments-2021-01-000",
		SourceIdentity:      "source-identity-123",
		CandidateVersion:    "broad_v1",
		CandidateConfigHash: "sha256:candidate-config",
		OutputPath:          inputPath,
		OutputSHA256:        inputChecksum,
		StartedAt:           time.Now().UTC().Format(time.RFC3339),
		FinishedAt:          time.Now().UTC().Format(time.RFC3339),
		Result: candidate.ScanResult{
			Status:           "completed",
			RowsCandidates:   3,
			OutputPath:       inputPath,
			CandidateVersion: "broad_v1",
		},
	}); err != nil {
		t.Fatalf("write scan checkpoint: %v", err)
	}

	configPath := filepath.Join(dir, "relevance.yaml")
	configText := `
version: test_v1
tiers: {a: 0.80, b: 0.60, c: 0.40}
domains:
  - name: travel
    subreddit_prior_weight: 0.15
    group_weights: {travel_language: 0.50, pain_language: 0.15}
  - name: saas_opportunity
    subreddit_prior_weight: 0.10
    group_weights: {product_and_tool_language: 0.25, business_workflow_language: 0.35, request_intent: 0.25}
  - name: app_opportunity
    subreddit_prior_weight: 0.10
    group_weights: {product_and_tool_language: 0.35, request_intent: 0.30}
`
	if err := os.WriteFile(configPath, []byte(configText), 0o644); err != nil {
		t.Fatalf("write relevance config: %v", err)
	}

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "dev", "duckdb_score_candidates.py"))
	if err != nil {
		t.Fatalf("resolve scorer script: %v", err)
	}
	outputPath := filepath.Join(dir, "scores.parquet")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run(
		[]string{
			"--scan-checkpoint", checkpointPath,
			"--relevance-config", configPath,
			"--output-path", outputPath,
			"--scorer-script", scriptPath,
			"--duckdb-temp-dir", filepath.Join(dir, "duckdb-tmp"),
		},
		&stdout,
		&stderr,
	)
	if exitCode != 0 {
		t.Fatalf("expected success, code=%d stderr=%s", exitCode, stderr.String())
	}

	var result relevance.ScoreResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("parse output: %v: %s", err, stdout.String())
	}
	if result.RowsCandidates != 3 || result.RowsRetainedCandidates != 2 {
		t.Fatalf("unexpected score result: %+v", result)
	}
}

func createCandidateFixture(t *testing.T, outputPath string) {
	t.Helper()
	script := `
import duckdb
import sys
con = duckdb.connect()
con.execute("""
COPY (
    SELECT * FROM (
        VALUES
            ('comment', 'travel-pain', false, false, '["travel_language","pain_language"]'::JSON, '["visa","frustrating"]'::JSON),
            ('comment', 'prior-only', true, false, '[]'::JSON, '[]'::JSON),
            ('comment', 'saas-request', false, false, '["product_and_tool_language","business_workflow_language","request_intent"]'::JSON, '["software","workflow","need an app"]'::JSON)
    ) AS t(source_type, source_id, subreddit_prior_match, is_bot_like, matched_rule_groups, matched_terms)
)
TO ? (FORMAT PARQUET)
""", [sys.argv[1]])
`
	cmd := exec.Command("python3", "-c", script, outputPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create candidate fixture: %v: %s", err, output)
	}
}
