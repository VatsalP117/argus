package candidate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"argus/internal/database"
	"argus/internal/manifest"
	"argus/internal/relevance"
)

func TestCleanupValidatedStagingIsAuditedAndIdempotent(t *testing.T) {
	fixture := prepareCommittedCleanupFixture(t)

	first, err := Cleanup(context.Background(), CleanupOptions{
		DatabasePath:  fixture.DatabasePath,
		IngestBatchID: fixture.IngestBatchID,
		ScriptPath:    filepath.Join("..", "..", "scripts", "dev", "duckdb_cleanup_staging.py"),
	})
	if err != nil {
		t.Fatalf("cleanup staging: %v", err)
	}
	if first.Status != "completed" || first.FilesRemoved != 2 {
		t.Fatalf("unexpected cleanup result: %+v", first)
	}
	for _, path := range []string{fixture.CandidatePath, fixture.ScorePath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected staging file to be removed: %s err=%v", path, err)
		}
	}

	second, err := Cleanup(context.Background(), CleanupOptions{
		DatabasePath:  fixture.DatabasePath,
		IngestBatchID: fixture.IngestBatchID,
		ScriptPath:    filepath.Join("..", "..", "scripts", "dev", "duckdb_cleanup_staging.py"),
	})
	if err != nil {
		t.Fatalf("repeat cleanup: %v", err)
	}
	if second.Status != "skipped_existing" {
		t.Fatalf("expected skipped_existing, got %s", second.Status)
	}
}

func TestCleanupRefusesTamperedStaging(t *testing.T) {
	fixture := prepareCommittedCleanupFixture(t)
	if err := os.WriteFile(fixture.CandidatePath, []byte("tampered"), 0o644); err != nil {
		t.Fatalf("tamper candidate staging: %v", err)
	}

	_, err := Cleanup(context.Background(), CleanupOptions{
		DatabasePath:  fixture.DatabasePath,
		IngestBatchID: fixture.IngestBatchID,
		ScriptPath:    filepath.Join("..", "..", "scripts", "dev", "duckdb_cleanup_staging.py"),
	})
	if err == nil {
		t.Fatal("expected cleanup to reject tampered staging")
	}
	if _, statErr := os.Stat(fixture.ScorePath); statErr != nil {
		t.Fatalf("score staging should remain after rejected cleanup: %v", statErr)
	}
}

type cleanupFixture struct {
	DatabasePath  string
	IngestBatchID string
	CandidatePath string
	ScorePath     string
}

func prepareCommittedCleanupFixture(t *testing.T) cleanupFixture {
	t.Helper()
	dir := t.TempDir()
	databasePath := filepath.Join(dir, "argus.duckdb")
	if _, err := database.Migrate(context.Background(), database.AdminOptions{
		DatabasePath:  databasePath,
		MigrationsDir: filepath.Join("..", "..", "sql", "migrations"),
		ScriptPath:    filepath.Join("..", "..", "scripts", "dev", "duckdb_admin.py"),
	}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	candidatePath := filepath.Join(dir, "candidates.parquet")
	createCommitCandidateFixture(t, candidatePath)
	candidateChecksum, err := FileSHA256(candidatePath)
	if err != nil {
		t.Fatalf("checksum candidates: %v", err)
	}
	candidateInfo, err := os.Stat(candidatePath)
	if err != nil {
		t.Fatalf("stat candidates: %v", err)
	}
	rules := commitTestRelevanceConfig()
	scorePath := filepath.Join(dir, "scores.parquet")
	if _, err := relevance.Score(context.Background(), relevance.Options{
		InputPath:      candidatePath,
		OutputPath:     scorePath,
		ScriptPath:     filepath.Join("..", "..", "scripts", "dev", "duckdb_score_candidates.py"),
		TempDir:        filepath.Join(dir, "duckdb-tmp"),
		MemoryLimit:    "1GB",
		Threads:        1,
		RelevanceRules: rules,
	}); err != nil {
		t.Fatalf("score candidates: %v", err)
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
		Summary:         manifest.Summary{EntryCount: 1, BytesTotal: 1234},
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
			BytesWritten:             candidateInfo.Size(),
			OutputPath:               candidatePath,
			CandidateVersion:         "broad_v1",
			MatchedByGroup:           map[string]int64{"pain_language": 1},
			SubredditPriorCandidates: 1,
		},
	}
	result, err := Commit(context.Background(), CommitOptions{
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
		RelevanceRules:      rules,
		RelevanceConfigHash: "sha256:relevance-config",
		AuthorHashSalt:      "test-author-hash-salt",
		ScriptPath:          filepath.Join("..", "..", "scripts", "dev", "duckdb_commit_candidates.py"),
	})
	if err != nil {
		t.Fatalf("commit candidates: %v", err)
	}

	return cleanupFixture{
		DatabasePath:  databasePath,
		IngestBatchID: result.IngestBatchID,
		CandidatePath: candidatePath,
		ScorePath:     scorePath,
	}
}
