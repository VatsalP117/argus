package candidate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"argus/internal/config"
	"argus/internal/manifest"
)

func TestScanRetainsBroadMatchesOutsideSubredditPriors(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "comments.parquet")
	createFixtureParquet(t, sourcePath)

	result, err := Scan(context.Background(), Options{
		Entry: manifest.Entry{
			EntryID:         "comments-2021-01-000",
			RecordType:      "comments",
			ResolveURL:      sourcePath,
			ShardPath:       "data/comments/2021/01/000.parquet",
			ArchiveRevision: "revision-123",
			SourceIdentity:  "source-identity-123",
		},
		ManifestID:  "manifest-123",
		DatasetRepo: "open-index/arctic",
		OutputPath:  filepath.Join(dir, "candidates.parquet"),
		ScriptPath:  filepath.Join("..", "..", "scripts", "dev", "duckdb_scan_candidates.py"),
		TempDir:     filepath.Join(dir, "duckdb-tmp"),
		MemoryLimit: "1GB",
		Threads:     1,
		CandidateRules: config.CandidateConfig{
			Version:           "test_v1",
			MinimumTextLength: 40,
			ExcludedExactText: []string{"[deleted]", "[removed]"},
			SubredditPriors:   []string{"travel"},
			RuleGroups: []config.CandidateRuleGroup{
				{Name: "pain_language", Terms: []string{"frustrating", "takes forever"}},
				{Name: "travel_language", Terms: []string{"visa", "embassy"}},
				{Name: "product_language", Terms: []string{"app"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("scan candidates: %v", err)
	}

	if result.RowsSeen != 3 {
		t.Fatalf("expected 3 rows seen, got %d", result.RowsSeen)
	}
	if result.RowsCandidates != 2 {
		t.Fatalf("expected 2 candidates, got %d", result.RowsCandidates)
	}
	if result.RowsRejectedEarly != 1 {
		t.Fatalf("expected 1 early rejection, got %d", result.RowsRejectedEarly)
	}
	if result.MatchedByGroup["travel_language"] != 1 {
		t.Fatalf("expected one travel-language match, got %v", result.MatchedByGroup)
	}
	if result.SubredditPriorCandidates != 1 {
		t.Fatalf("expected one subreddit-prior candidate, got %d", result.SubredditPriorCandidates)
	}
	if info, err := os.Stat(result.OutputPath); err != nil || info.Size() == 0 {
		t.Fatalf("expected non-empty candidate parquet, info=%v err=%v", info, err)
	}
}

func createFixtureParquet(t *testing.T, outputPath string) {
	t.Helper()
	script := `
import duckdb
import sys
con = duckdb.connect()
con.execute("""
COPY (
    SELECT * FROM (
        VALUES
            ('outside-prior', 'person1', 'randomcity', 'The embassy appointment system is frustrating and takes forever to use.', 3, 1610000000, 't3_thread1', 't3_thread1'),
            ('prior-only', 'person2', 'travel', 'Sharing a beautiful photograph and a cheerful story from yesterday morning.', 2, 1610000001, 't3_thread2', 't3_thread2'),
            ('unrelated', 'person3', 'cats', 'A happy cat enjoys colorful blankets and long sleeping sessions at home.', 1, 1610000002, 't3_thread3', 't3_thread3')
    ) AS t(id, author, subreddit, body, score, created_utc, link_id, parent_id)
)
TO ? (FORMAT PARQUET)
""", [sys.argv[1]])
`
	cmd := exec.Command("python3", "-c", script, outputPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create fixture parquet: %v: %s", err, output)
	}
}
