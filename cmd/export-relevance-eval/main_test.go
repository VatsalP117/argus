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

func TestRunExportsChecksumValidatedEvaluationFixture(t *testing.T) {
	dir := t.TempDir()
	candidatePath := filepath.Join(dir, "candidates.parquet")
	scorePath := filepath.Join(dir, "scores.parquet")
	createFixture(t, candidatePath, scorePath)

	checksum, err := candidate.FileSHA256(candidatePath)
	if err != nil {
		t.Fatalf("checksum candidates: %v", err)
	}
	checkpointPath := filepath.Join(dir, "scan.json")
	if err := candidate.WriteScanCheckpoint(checkpointPath, candidate.ScanCheckpoint{
		Status:         "completed",
		ManifestID:     "manifest-123",
		EntryID:        "comments-2021-01-000",
		SourceIdentity: "identity-123",
		OutputPath:     candidatePath,
		OutputSHA256:   checksum,
		StartedAt:      time.Now().UTC().Format(time.RFC3339),
		FinishedAt:     time.Now().UTC().Format(time.RFC3339),
		Result: candidate.ScanResult{
			Status:         "completed",
			RowsCandidates: 3,
			OutputPath:     candidatePath,
		},
	}); err != nil {
		t.Fatalf("write checkpoint: %v", err)
	}

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "dev", "duckdb_export_relevance_eval.py"))
	if err != nil {
		t.Fatalf("resolve exporter script: %v", err)
	}
	outputPath := filepath.Join(dir, "labels.csv")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{
		"--scan-checkpoint", checkpointPath,
		"--score-path", scorePath,
		"--output-path", outputPath,
		"--sample-per-stratum", "1",
		"--seed", "fixture-v1",
		"--exporter-script", scriptPath,
	}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected success, code=%d stderr=%s", exitCode, stderr.String())
	}

	var result relevance.EvaluationExportResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("parse output: %v: %s", err, stdout.String())
	}
	if result.RowsExported != 3 {
		t.Fatalf("expected 3 rows, got %+v", result)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("expected label fixture: %v", err)
	}
}

func createFixture(t *testing.T, candidatePath, scorePath string) {
	t.Helper()
	script := `
import duckdb
import sys
candidate_path, score_path = sys.argv[1], sys.argv[2]
con = duckdb.connect()
con.execute("""
COPY (
    SELECT * FROM (
        VALUES
            ('comment','retain','travel','Embassy appointment delays are frustrating.','https://reddit.test/retain','["embassy"]'::JSON,'["travel_language","pain_language"]'::JSON),
            ('comment','evaluate','apps','A reporting workflow.','https://reddit.test/evaluate','["reporting"]'::JSON,'["business_workflow_language"]'::JSON),
            ('comment','discard','cards','Visa card payment.','https://reddit.test/discard','["visa"]'::JSON,'["travel_language"]'::JSON)
    ) AS t(source_type,source_id,subreddit,original_text,source_url,matched_terms,matched_rule_groups)
) TO ? (FORMAT PARQUET)
""", [candidate_path])
con.execute("""
COPY (
    SELECT * FROM (
        VALUES
            ('comment','retain','travel',0.7,'B','retain'),
            ('comment','retain','saas_opportunity',0.0,'D','discard'),
            ('comment','retain','app_opportunity',0.0,'D','discard'),
            ('comment','evaluate','travel',0.0,'D','discard'),
            ('comment','evaluate','saas_opportunity',0.5,'C','evaluate'),
            ('comment','evaluate','app_opportunity',0.4,'C','evaluate'),
            ('comment','discard','travel',0.2,'D','discard'),
            ('comment','discard','saas_opportunity',0.0,'D','discard'),
            ('comment','discard','app_opportunity',0.0,'D','discard')
    ) AS t(source_type,source_id,domain,relevance_score,relevance_tier,decision)
) TO ? (FORMAT PARQUET)
""", [score_path])
`
	cmd := exec.Command("python3", "-c", script, candidatePath, scorePath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create fixture: %v: %s", err, output)
	}
}
