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

func TestCommitCandidatesTieredRetentionReviewTierOptIn(t *testing.T) {
	dir := t.TempDir()
	defaultDB := filepath.Join(dir, "default.duckdb")
	reviewDB := filepath.Join(dir, "review.duckdb")
	adminScript := filepath.Join("..", "..", "scripts", "dev", "duckdb_admin.py")
	for _, dbPath := range []string{defaultDB, reviewDB} {
		if _, err := database.Migrate(context.Background(), database.AdminOptions{
			DatabasePath:  dbPath,
			MigrationsDir: filepath.Join("..", "..", "sql", "migrations"),
			ScriptPath:    adminScript,
		}); err != nil {
			t.Fatalf("migrate database: %v", err)
		}
	}

	candidatePath := filepath.Join(dir, "candidates.parquet")
	createTieredRetentionCandidateFixture(t, candidatePath)
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
		t.Fatalf("expected 2 trusted retain candidates from scorer, got %d", scoreResult.RowsRetainedCandidates)
	}
	if scoreResult.RowsEvaluationCandidates != 1 {
		t.Fatalf("expected 1 evaluate (C-tier) candidate from scorer, got %d", scoreResult.RowsEvaluationCandidates)
	}
	scoreChecksum, err := FileSHA256(scorePath)
	if err != nil {
		t.Fatalf("checksum scores: %v", err)
	}

	sourceManifest := manifest.Manifest{
		ManifestID:      "manifest-tiered",
		GeneratedAt:     "2026-06-18T00:00:00Z",
		DatasetRepo:     "open-index/arctic",
		ArchiveRevision: "revision-tiered",
		PipelineName:    "test-pipeline",
		Summary:         manifest.Summary{EntryCount: 1, BytesTotal: 1234},
		Entries: []manifest.Entry{
			{
				EntryID:         "comments-2021-01-000",
				RecordType:      "comments",
				ShardPath:       "data/comments/2021/01/000.parquet",
				SizeBytes:       1234,
				ArchiveRevision: "revision-tiered",
				SourceIdentity:  "source-identity-tiered",
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
			Status:            "completed",
			EntryID:           sourceManifest.Entries[0].EntryID,
			RecordType:        "comments",
			RowsSeen:          8,
			RowsCandidates:    4,
			RowsRejectedEarly: 4,
			BytesWritten:      123,
			OutputPath:        candidatePath,
			CandidateVersion:  "broad_v1",
			MatchedByGroup:    map[string]int64{"travel_language": 2, "pain_language": 1, "product_and_tool_language": 1, "business_workflow_language": 1, "request_intent": 1},
			SubredditPriorCandidates: 1,
		},
	}

	baseOptions := CommitOptions{
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

	defaultOptions := baseOptions
	defaultOptions.DatabasePath = defaultDB
	defaultOptions.IncludeReviewTier = false
	defaultResult, err := Commit(context.Background(), defaultOptions)
	if err != nil {
		t.Fatalf("default commit: %v", err)
	}
	if defaultResult.Status != "completed" {
		t.Fatalf("expected completed, got %s", defaultResult.Status)
	}
	if defaultResult.RowsRetained != 2 {
		t.Fatalf("default commit must exclude C-tier review evidence, expected 2 retained, got %d", defaultResult.RowsRetained)
	}
	if defaultResult.RowsReviewTier != 0 {
		t.Fatalf("default commit must not retain review-tier rows, got %d", defaultResult.RowsReviewTier)
	}
	if defaultResult.RelevanceRows != 6 {
		t.Fatalf("default commit relevance rows, expected 6, got %d", defaultResult.RelevanceRows)
	}

	defaultCounts := inspectCommitCounts(t, defaultDB)
	if defaultCounts["documents"] != 2 || defaultCounts["document_relevance"] != 6 {
		t.Fatalf("default durable counts must be trusted-only: %v", defaultCounts)
	}
	if _, present := inspectRelevanceRow(t, defaultDB, "comment:travel-evaluate", "travel"); present {
		t.Fatalf("default commit must not commit the C-tier travel-evaluate document")
	}

	reviewOptions := baseOptions
	reviewOptions.DatabasePath = reviewDB
	reviewOptions.IncludeReviewTier = true
	reviewResult, err := Commit(context.Background(), reviewOptions)
	if err != nil {
		t.Fatalf("review-tier commit: %v", err)
	}
	if reviewResult.Status != "completed" {
		t.Fatalf("expected completed, got %s", reviewResult.Status)
	}
	if reviewResult.RowsRetained != 3 {
		t.Fatalf("opt-in commit must include C-tier review evidence, expected 3 retained, got %d", reviewResult.RowsRetained)
	}
	if reviewResult.RowsReviewTier != 1 {
		t.Fatalf("opt-in commit must report 1 review-tier row, got %d", reviewResult.RowsReviewTier)
	}
	if reviewResult.RelevanceRows != 9 {
		t.Fatalf("opt-in commit relevance rows, expected 9, got %d", reviewResult.RelevanceRows)
	}
	if !reviewResult.SourceEquationValid || !reviewResult.StagingEquationValid {
		t.Fatalf("opt-in commit reconciliation failed: %+v", reviewResult)
	}

	reviewCounts := inspectCommitCounts(t, reviewDB)
	if reviewCounts["documents"] != 3 || reviewCounts["document_relevance"] != 9 {
		t.Fatalf("opt-in durable counts must include review tier: %v", reviewCounts)
	}

	row, present := inspectRelevanceRow(t, reviewDB, "comment:travel-evaluate", "travel")
	if !present {
		t.Fatalf("opt-in commit must retain the C-tier travel-evaluate relevance row")
	}
	if row["decision"] != "evaluate" {
		t.Fatalf("C-tier row must preserve decision=evaluate, got %v", row["decision"])
	}
	if row["relevance_tier"] != "C" {
		t.Fatalf("C-tier row must preserve relevance_tier=C, got %v", row["relevance_tier"])
	}
	reasonsJSON, ok := row["decision_reasons"].(string)
	if !ok || len(reasonsJSON) == 0 {
		t.Fatalf("C-tier row must preserve non-empty decision_reasons, got %v", row["decision_reasons"])
	}
	var reasons []interface{}
	if err := json.Unmarshal([]byte(reasonsJSON), &reasons); err != nil {
		t.Fatalf("C-tier decision_reasons is not valid JSON: %v", reasonsJSON)
	}
	if len(reasons) == 0 {
		t.Fatalf("C-tier row must preserve non-empty decision_reasons, got %v", reasonsJSON)
	}
}

func TestCommitCandidatesReviewTierRetryOnDefaultBatchErrors(t *testing.T) {
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
	createTieredRetentionCandidateFixture(t, candidatePath)
	candidateChecksum, err := FileSHA256(candidatePath)
	if err != nil {
		t.Fatalf("checksum candidates: %v", err)
	}

	relevanceRules := commitTestRelevanceConfig()
	scorePath := filepath.Join(dir, "scores.parquet")
	if _, err := relevance.Score(context.Background(), relevance.Options{
		InputPath:      candidatePath,
		OutputPath:     scorePath,
		ScriptPath:     filepath.Join("..", "..", "scripts", "dev", "duckdb_score_candidates.py"),
		TempDir:        filepath.Join(dir, "duckdb-tmp"),
		MemoryLimit:    "1GB",
		Threads:        1,
		RelevanceRules: relevanceRules,
	}); err != nil {
		t.Fatalf("score candidates: %v", err)
	}
	scoreChecksum, err := FileSHA256(scorePath)
	if err != nil {
		t.Fatalf("checksum scores: %v", err)
	}

	sourceManifest := manifest.Manifest{
		ManifestID:      "manifest-tiered",
		GeneratedAt:     "2026-06-18T00:00:00Z",
		DatasetRepo:     "open-index/arctic",
		ArchiveRevision: "revision-tiered",
		PipelineName:    "test-pipeline",
		Summary:         manifest.Summary{EntryCount: 1, BytesTotal: 1234},
		Entries: []manifest.Entry{
			{
				EntryID:         "comments-2021-01-000",
				RecordType:      "comments",
				ShardPath:       "data/comments/2021/01/000.parquet",
				SizeBytes:       1234,
				ArchiveRevision: "revision-tiered",
				SourceIdentity:  "source-identity-tiered",
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
			RowsSeen:                 8,
			RowsCandidates:           4,
			RowsRejectedEarly:        4,
			BytesWritten:             123,
			OutputPath:               candidatePath,
			CandidateVersion:         "broad_v1",
			MatchedByGroup:           map[string]int64{"travel_language": 2, "pain_language": 1, "product_and_tool_language": 1, "business_workflow_language": 1, "request_intent": 1},
			SubredditPriorCandidates: 1,
		},
	}

	baseOptions := CommitOptions{
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

	defaultOptions := baseOptions
	if _, err := Commit(context.Background(), defaultOptions); err != nil {
		t.Fatalf("default commit: %v", err)
	}

	retryOptions := baseOptions
	retryOptions.IncludeReviewTier = true
	if _, err := Commit(context.Background(), retryOptions); err == nil {
		t.Fatalf("retry with --include-review-tier on a default-committed batch must error, not silently skip")
	}

	defaultCounts := inspectCommitCounts(t, databasePath)
	if defaultCounts["documents"] != 2 || defaultCounts["document_relevance"] != 6 {
		t.Fatalf("retry must not mutate the default-committed durable corpus: %v", defaultCounts)
	}

	repeatOptions := baseOptions
	repeatResult, err := Commit(context.Background(), repeatOptions)
	if err != nil {
		t.Fatalf("repeating the same default scope must remain idempotent: %v", err)
	}
	if repeatResult.Status != "skipped_existing" {
		t.Fatalf("repeating same scope must skip, got %s", repeatResult.Status)
	}
	if repeatResult.RowsRetained != 2 || repeatResult.RowsReviewTier != 0 {
		t.Fatalf("repeating same scope must report unchanged counts: %+v", repeatResult)
	}
}

func createTieredRetentionCandidateFixture(t *testing.T, outputPath string) {
	t.Helper()
	script := `
import duckdb
import sys
con = duckdb.connect()
con.execute("""
COPY (
    SELECT * FROM (
        VALUES
            ('comment', 'travel-pain', 'travel-pain', 'alice', 'randomcity', 3::BIGINT, to_timestamp(1610000000), 'open-index/arctic', 'revision-tiered', 'data/comments/2021/01/000.parquet', 'source-identity-tiered', 'manifest-tiered', 'comments-2021-01-000', 't3_thread1', 't3_thread1', NULL::VARCHAR, 'The visa process is frustrating.', 'The visa process is frustrating.', 'https://www.reddit.com/comments/thread1/_/travel-pain', false, false, false, false, 'broad_v1', '["travel_language","pain_language"]'::JSON, '["visa","frustrating"]'::JSON, '["travel_language","pain_language"]'::JSON),
            ('comment', 'saas-request', 'saas-request', 'bob', 'smallbusiness', 5::BIGINT, to_timestamp(1610000001), 'open-index/arctic', 'revision-tiered', 'data/comments/2021/01/000.parquet', 'source-identity-tiered', 'manifest-tiered', 'comments-2021-01-000', 't3_thread2', 't3_thread2', NULL::VARCHAR, 'I need an app for this reporting workflow.', 'I need an app for this reporting workflow.', 'https://www.reddit.com/comments/thread2/_/saas-request', false, false, false, false, 'broad_v1', '["product_and_tool_language","business_workflow_language","request_intent"]'::JSON, '["app","reporting","need an app"]'::JSON, '["product_and_tool_language","business_workflow_language","request_intent"]'::JSON),
            ('comment', 'travel-evaluate', 'travel-evaluate', 'dave', 'travel', 2::BIGINT, to_timestamp(1610000003), 'open-index/arctic', 'revision-tiered', 'data/comments/2021/01/000.parquet', 'source-identity-tiered', 'manifest-tiered', 'comments-2021-01-000', 't3_thread4', 't3_thread4', NULL::VARCHAR, 'The visa appointment was delayed.', 'The visa appointment was delayed.', 'https://www.reddit.com/comments/thread4/_/travel-evaluate', false, false, false, false, 'broad_v1', '["travel_language"]'::JSON, '["visa","appointment"]'::JSON, '["travel_language"]'::JSON),
            ('comment', 'prior-only', 'prior-only', 'carol', 'travel', 1::BIGINT, to_timestamp(1610000002), 'open-index/arctic', 'revision-tiered', 'data/comments/2021/01/000.parquet', 'source-identity-tiered', 'manifest-tiered', 'comments-2021-01-000', 't3_thread3', 't3_thread3', NULL::VARCHAR, 'A pleasant photograph from my morning walk.', 'A pleasant photograph from my morning walk.', 'https://www.reddit.com/comments/thread3/_/prior-only', true, false, false, false, 'broad_v1', '[]'::JSON, '[]'::JSON, '["subreddit_prior"]'::JSON)
    ) AS t(source_type, source_id, raw_id, author, subreddit, score, created_at, archive_repo, archive_revision, source_file, source_identity, manifest_id, manifest_entry_id, thread_id, parent_id, title, original_text, candidate_text, source_url, subreddit_prior_match, is_deleted, is_removed, is_bot_like, candidate_version, matched_rule_groups, matched_terms, candidate_reasons)
)
TO ? (FORMAT PARQUET)
""", [sys.argv[1]])
`
	cmd := exec.Command("python3", "-c", script, outputPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create tiered retention candidate fixture: %v: %s", err, output)
	}
}

func inspectRelevanceRow(t *testing.T, databasePath, documentID, domain string) (map[string]interface{}, bool) {
	t.Helper()
	script := `
import duckdb
import json
import sys
con = duckdb.connect(sys.argv[1], read_only=True)
row = con.execute(
    "SELECT relevance_score, relevance_tier, decision, decision_reasons, relevance_version "
    "FROM document_relevance WHERE document_id = ? AND domain = ?",
    [sys.argv[2], sys.argv[3]],
).fetchone()
if not row:
    print(json.dumps({}))
else:
    print(json.dumps({
        "relevance_score": row[0],
        "relevance_tier": row[1],
        "decision": row[2],
        "decision_reasons": row[3],
        "relevance_version": row[4],
    }))
`
	output, err := exec.Command("python3", "-c", script, databasePath, documentID, domain).CombinedOutput()
	if err != nil {
		t.Fatalf("inspect relevance row: %v: %s", err, output)
	}
	var row map[string]interface{}
	if err := json.Unmarshal(output, &row); err != nil {
		t.Fatalf("parse relevance row: %v: %s", err, output)
	}
	if len(row) == 0 {
		return nil, false
	}
	return row, true
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
