package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"argus/internal/candidate"
	"argus/internal/manifest"
)

func TestRunScansOnePinnedManifestEntry(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "comments.parquet")
	createFixtureParquet(t, sourcePath)

	manifestPath := filepath.Join(dir, "manifest.json")
	if err := manifest.Write(manifestPath, manifest.Manifest{
		ManifestID:      "manifest-123",
		DatasetRepo:     "open-index/arctic",
		ArchiveRevision: "revision-123",
		Entries: []manifest.Entry{
			{
				EntryID:         "comments-2021-01-000",
				RecordType:      "comments",
				ResolveURL:      sourcePath,
				ShardPath:       "data/comments/2021/01/000.parquet",
				ArchiveRevision: "revision-123",
				SourceIdentity:  "source-identity-123",
			},
		},
	}); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	configPath := filepath.Join(dir, "candidates.yaml")
	configText := `
version: test_v1
minimum_text_length: 40
excluded_exact_text: ["[deleted]", "[removed]"]
subreddit_priors: ["travel"]
rule_groups:
  - name: pain_language
    terms: ["frustrating", "takes forever"]
  - name: travel_language
    terms: ["visa", "embassy"]
  - name: product_language
    terms: ["app"]
`
	if err := os.WriteFile(configPath, []byte(configText), 0o644); err != nil {
		t.Fatalf("write candidate config: %v", err)
	}

	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "dev", "duckdb_scan_candidates.py"))
	if err != nil {
		t.Fatalf("resolve scanner script: %v", err)
	}
	outputPath := filepath.Join(dir, "candidates-output.parquet")
	checkpointPath := filepath.Join(dir, "scan-checkpoint.json")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	args := []string{
		"--manifest", manifestPath,
		"--entry-id", "comments-2021-01-000",
		"--candidate-config", configPath,
		"--output-path", outputPath,
		"--checkpoint-path", checkpointPath,
		"--scanner-script", scriptPath,
		"--duckdb-temp-dir", filepath.Join(dir, "duckdb-tmp"),
	}
	exitCode := run(args, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected success, code=%d stderr=%s", exitCode, stderr.String())
	}

	var result candidate.ScanResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("parse output: %v: %s", err, stdout.String())
	}
	if result.RowsSeen != 3 || result.RowsCandidates != 2 {
		t.Fatalf("unexpected scan counts: %+v", result)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = run(args, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected idempotent retry, code=%d stderr=%s", exitCode, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("parse retry output: %v: %s", err, stdout.String())
	}
	if result.Status != "skipped_existing" {
		t.Fatalf("expected skipped_existing retry, got %s", result.Status)
	}
}

func createFixtureParquet(t *testing.T, outputPath string) {
	t.Helper()
	script := fmt.Sprintf(`
import duckdb
con = duckdb.connect()
con.execute("""
COPY (
    SELECT * FROM (
        VALUES
            ('outside-prior', 'person1', 'randomcity', 'The embassy appointment system is frustrating and takes forever to use.', 3, 1610000000, 't3_thread1', 't3_thread1'),
            ('prior-only', 'person2', 'travel', 'Sharing a beautiful photograph and a cheerful story from yesterday morning.', 2, 1610000001, 't3_thread2', 't3_thread2'),
            ('unrelated', 'person3', 'cats', 'A happy cat enjoys colorful blankets at https://example.com/share?utm_name=ios_app every morning.', 1, 1610000002, 't3_thread3', 't3_thread3')
    ) AS t(id, author, subreddit, body, score, created_utc, link_id, parent_id)
)
TO '%s' (FORMAT PARQUET)
""")
`, filepath.ToSlash(outputPath))
	cmd := exec.Command("python3", "-c", script)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create fixture parquet: %v: %s", err, output)
	}
}
