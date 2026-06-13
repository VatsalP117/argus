package database

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestProductMigrationsCreateCandidateLifecycleSchema(t *testing.T) {
	dir := t.TempDir()
	databasePath := filepath.Join(dir, "argus.duckdb")
	scriptPath := filepath.Join("..", "..", "scripts", "dev", "duckdb_admin.py")
	migrationsDir := filepath.Join("..", "..", "sql", "migrations")

	result, err := Migrate(context.Background(), AdminOptions{
		DatabasePath:  databasePath,
		MigrationsDir: migrationsDir,
		ScriptPath:    scriptPath,
	})
	if err != nil {
		t.Fatalf("migrate product schema: %v", err)
	}
	if result.SchemaVersion != 3 {
		t.Fatalf("expected schema version 3, got %d", result.SchemaVersion)
	}

	query := `
import duckdb
import json
import sys
con = duckdb.connect(sys.argv[1], read_only=True)
tables = [row[0] for row in con.execute("""
    SELECT table_name
    FROM information_schema.tables
    WHERE table_schema = 'main'
    ORDER BY table_name
""").fetchall()]
print(json.dumps(tables))
`
	output, err := exec.Command("python3", "-c", query, databasePath).CombinedOutput()
	if err != nil {
		t.Fatalf("inspect product schema: %v: %s", err, output)
	}
	var tables []string
	if err := json.Unmarshal(output, &tables); err != nil {
		t.Fatalf("parse product tables: %v: %s", err, output)
	}

	for _, required := range []string{
		"candidate_scan_runs",
		"candidate_rule_yields",
		"staged_candidate_batches",
		"staging_cleanup_events",
	} {
		if !contains(tables, required) {
			t.Fatalf("required table %q is missing from %v", required, tables)
		}
	}

	var stagingColumns []string
	columnOutput, err := exec.Command(
		"python3",
		"-c",
		`
import duckdb
import json
import sys
con = duckdb.connect(sys.argv[1], read_only=True)
print(json.dumps([row[0] for row in con.execute("DESCRIBE staged_candidate_batches").fetchall()]))
`,
		databasePath,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("inspect staging columns: %v: %s", err, columnOutput)
	}
	if err := json.Unmarshal(columnOutput, &stagingColumns); err != nil {
		t.Fatalf("parse staging columns: %v: %s", err, columnOutput)
	}
	for _, required := range []string{"score_path", "score_checksum", "score_bytes", "relevance_version", "relevance_config_hash"} {
		if !contains(stagingColumns, required) {
			t.Fatalf("required staging column %q is missing from %v", required, stagingColumns)
		}
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
