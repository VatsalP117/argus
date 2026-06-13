package candidate

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"argus/internal/config"
	"argus/internal/database"
	"argus/internal/manifest"
	"argus/internal/relevance"
)

func TestCommitCandidatesIsTransactionalReconciledAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	databasePath := filepath.Join(dir, "argus.duckdb")
	adminScript := filepath.Join("..", "..", "scripts", "dev", "duckdb_admin.py")
	if _, err := database.Migrate(context.Background(), database.AdminOptions{
		DatabasePath:  databasePath,
		MigrationsDir: filepath.Join("..", "..", "sql", "migrations"),
		ScriptPath:    adminScript,
	}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	candidatePath := filepath.Join(dir, "candidates.parquet")
	createCommitCandidateFixture(t, candidatePath)
	candidateChecksum, err := FileSHA256(candidatePath)
	if err != nil {
		t.Fatalf("checksum candidates: %v", err)
	}

	relevanceRules := commitTestRelevanceConfig()
	scorePath := filepath.Join(dir, "scores.parquet")
	scoreResult, err := relevance.Score(context.Background(), relevance.Options{
		InputPath:      candidatePath,
		OutputPath:     scorePath,
		ScriptPath:     filepath.Join("..", "..", "scripts", "dev", "duckdb_score_candidates.py"),
		TempDir:        filepath.Join(dir, "duckdb-tmp"),
		MemoryLimit:    "1GB",
		Threads:        1,
		RelevanceRules: relevanceRules,
	})
	if err != nil {
		t.Fatalf("score candidates: %v", err)
	}
	if scoreResult.RowsRetainedCandidates != 2 {
		t.Fatalf("expected fixture to retain 2 candidates, got %d", scoreResult.RowsRetainedCandidates)
	}
	scoreChecksum, err := FileSHA256(scorePath)
	if err != nil {
		t.Fatalf("checksum scores: %v", err)
	}

	sourceManifest := manifest.Manifest{
		ManifestID:      "manifest-123",
		GeneratedAt:     "2026-06-13T00:00:00Z",
		DatasetRepo:     "open-index/arctic",
		ArchiveRevision: "revision-123",
		PipelineName:    "test-pipeline",
		Summary: manifest.Summary{
			EntryCount: 1,
			BytesTotal: 1234,
		},
		Entries: []manifest.Entry{
			{
				EntryID:         "comments-2021-01-000",
				RecordType:      "comments",
				ShardPath:       "data/comments/2021/01/000.parquet",
				SizeBytes:       1234,
				ArchiveRevision: "revision-123",
				SourceIdentity:  "source-identity-123",
			},
		},
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := manifest.Write(manifestPath, sourceManifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	manifestChecksum, err := FileSHA256(manifestPath)
	if err != nil {
		t.Fatalf("checksum manifest: %v", err)
	}

	checkpoint := ScanCheckpoint{
		Status:              "completed",
		ManifestID:          sourceManifest.ManifestID,
		EntryID:             sourceManifest.Entries[0].EntryID,
		SourceIdentity:      sourceManifest.Entries[0].SourceIdentity,
		CandidateVersion:    "broad_v1",
		CandidateConfigHash: "sha256:candidate-config",
		OutputPath:          candidatePath,
		OutputSHA256:        candidateChecksum,
		StartedAt:           time.Now().UTC().Format(time.RFC3339),
		FinishedAt:          time.Now().UTC().Format(time.RFC3339),
		Result: ScanResult{
			Status:                   "completed",
			EntryID:                  sourceManifest.Entries[0].EntryID,
			RecordType:               "comments",
			RowsSeen:                 10,
			RowsCandidates:           3,
			RowsRejectedEarly:        7,
			BytesWritten:             123,
			OutputPath:               candidatePath,
			CandidateVersion:         "broad_v1",
			MatchedByGroup:           map[string]int64{"pain_language": 1, "travel_language": 1},
			SubredditPriorCandidates: 1,
		},
	}

	options := CommitOptions{
		DatabasePath:        databasePath,
		CandidatePath:       candidatePath,
		CandidateChecksum:   candidateChecksum,
		ScorePath:           scorePath,
		ScoreChecksum:       scoreChecksum,
		ManifestPath:        manifestPath,
		ManifestChecksum:    manifestChecksum,
		SourceManifest:      sourceManifest,
		ManifestEntry:       sourceManifest.Entries[0],
		ScanCheckpoint:      checkpoint,
		RelevanceRules:      relevanceRules,
		RelevanceConfigHash: "sha256:relevance-config",
		AuthorHashSalt:      "test-author-hash-salt",
		ScriptPath:          filepath.Join("..", "..", "scripts", "dev", "duckdb_commit_candidates.py"),
	}

	first, err := Commit(context.Background(), options)
	if err != nil {
		t.Fatalf("commit candidates: %v", err)
	}
	if first.Status != "completed" {
		t.Fatalf("expected completed status, got %s", first.Status)
	}
	if first.RowsRetained != 2 || !first.SourceEquationValid || !first.StagingEquationValid {
		t.Fatalf("unexpected reconciliation: %+v", first)
	}
	if first.RelevanceRows != 6 {
		t.Fatalf("expected 6 relevance rows, got %d", first.RelevanceRows)
	}

	second, err := Commit(context.Background(), options)
	if err != nil {
		t.Fatalf("repeat commit: %v", err)
	}
	if second.Status != "skipped_existing" {
		t.Fatalf("expected skipped_existing, got %s", second.Status)
	}

	counts := inspectCommitCounts(t, databasePath)
	if counts["documents"] != 2 || counts["document_relevance"] != 6 {
		t.Fatalf("unexpected durable counts: %v", counts)
	}
	if counts["ingest_batches"] != 1 || counts["batch_reconciliation"] != 1 {
		t.Fatalf("expected one durable batch and reconciliation row: %v", counts)
	}
	if counts["scored_staging"] != 1 {
		t.Fatalf("expected scored staging metadata to be durable: %v", counts)
	}
}

func commitTestRelevanceConfig() config.RelevanceConfig {
	return config.RelevanceConfig{
		Version: "test_v1",
		Tiers:   config.RelevanceTiers{A: 0.80, B: 0.60, C: 0.40},
		Domains: []config.RelevanceDomain{
			{
				Name:                 "travel",
				RequiredAnyTerms:     []string{"visa"},
				SubredditPriorWeight: 0.15,
				GroupWeights: map[string]float64{
					"travel_language": 0.50,
					"pain_language":   0.15,
				},
			},
			{
				Name:                 "saas_opportunity",
				SubredditPriorWeight: 0.10,
				GroupWeights: map[string]float64{
					"product_and_tool_language":  0.25,
					"business_workflow_language": 0.35,
					"request_intent":             0.25,
				},
			},
			{
				Name:                 "app_opportunity",
				SubredditPriorWeight: 0.10,
				GroupWeights: map[string]float64{
					"product_and_tool_language": 0.35,
					"request_intent":            0.30,
				},
			},
		},
		SignalMappings: []config.SignalMapping{
			{RuleGroup: "pain_language", SignalType: "pain_point", Score: 0.70},
			{RuleGroup: "request_intent", SignalType: "feature_request", Score: 0.80},
			{RuleGroup: "travel_language", SignalType: "travel_problem", Score: 0.60},
		},
	}
}

func createCommitCandidateFixture(t *testing.T, outputPath string) {
	t.Helper()
	script := `
import duckdb
import sys
con = duckdb.connect()
con.execute("""
COPY (
    SELECT * FROM (
        VALUES
            ('comment', 'travel-pain', 'travel-pain', 'alice', 'randomcity', 3::BIGINT, to_timestamp(1610000000), 'open-index/arctic', 'revision-123', 'data/comments/2021/01/000.parquet', 'source-identity-123', 'manifest-123', 'comments-2021-01-000', 't3_thread1', 't3_thread1', NULL::VARCHAR, 'The visa process is frustrating.', 'The visa process is frustrating.', 'https://www.reddit.com/comments/thread1/_/travel-pain', false, false, false, false, 'broad_v1', '["travel_language","pain_language"]'::JSON, '["visa","frustrating"]'::JSON, '["travel_language","pain_language"]'::JSON),
            ('comment', 'saas-request', 'saas-request', 'bob', 'smallbusiness', 5::BIGINT, to_timestamp(1610000001), 'open-index/arctic', 'revision-123', 'data/comments/2021/01/000.parquet', 'source-identity-123', 'manifest-123', 'comments-2021-01-000', 't3_thread2', 't3_thread2', NULL::VARCHAR, 'I need an app for this reporting workflow.', 'I need an app for this reporting workflow.', 'https://www.reddit.com/comments/thread2/_/saas-request', false, false, false, false, 'broad_v1', '["product_and_tool_language","business_workflow_language","request_intent"]'::JSON, '["app","reporting","need an app"]'::JSON, '["product_and_tool_language","business_workflow_language","request_intent"]'::JSON),
            ('comment', 'prior-only', 'prior-only', 'carol', 'travel', 1::BIGINT, to_timestamp(1610000002), 'open-index/arctic', 'revision-123', 'data/comments/2021/01/000.parquet', 'source-identity-123', 'manifest-123', 'comments-2021-01-000', 't3_thread3', 't3_thread3', NULL::VARCHAR, 'A pleasant photograph from my morning walk.', 'A pleasant photograph from my morning walk.', 'https://www.reddit.com/comments/thread3/_/prior-only', true, false, false, false, 'broad_v1', '[]'::JSON, '[]'::JSON, '["subreddit_prior"]'::JSON)
    ) AS t(source_type, source_id, raw_id, author, subreddit, score, created_at, archive_repo, archive_revision, source_file, source_identity, manifest_id, manifest_entry_id, thread_id, parent_id, title, original_text, candidate_text, source_url, subreddit_prior_match, is_deleted, is_removed, is_bot_like, candidate_version, matched_rule_groups, matched_terms, candidate_reasons)
)
TO ? (FORMAT PARQUET)
""", [sys.argv[1]])
`
	cmd := exec.Command("python3", "-c", script, outputPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create commit candidate fixture: %v: %s", err, output)
	}
}

func inspectCommitCounts(t *testing.T, databasePath string) map[string]int64 {
	t.Helper()
	script := `
import duckdb
import json
import sys
con = duckdb.connect(sys.argv[1], read_only=True)
tables = ["documents", "document_relevance", "signals", "entities", "ingest_batches", "batch_reconciliation"]
counts = {table: con.execute(f"SELECT count(*) FROM {table}").fetchone()[0] for table in tables}
counts["scored_staging"] = con.execute("""
    SELECT count(*)
    FROM staged_candidate_batches
    WHERE score_path IS NOT NULL
      AND score_checksum IS NOT NULL
      AND score_bytes > 0
""").fetchone()[0]
print(json.dumps(counts))
`
	output, err := exec.Command("python3", "-c", script, databasePath).CombinedOutput()
	if err != nil {
		t.Fatalf("inspect commit counts: %v: %s", err, output)
	}
	var counts map[string]int64
	if err := json.Unmarshal(output, &counts); err != nil {
		t.Fatalf("parse commit counts: %v: %s", err, output)
	}
	return counts
}
