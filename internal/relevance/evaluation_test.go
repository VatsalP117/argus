package relevance

import (
	"context"
	"encoding/csv"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExportEvaluationCreatesDeterministicStratifiedLabelFixture(t *testing.T) {
	dir := t.TempDir()
	candidatePath := filepath.Join(dir, "candidates.parquet")
	scorePath := filepath.Join(dir, "scores.parquet")
	createEvaluationFixture(t, candidatePath, scorePath)

	outputPath := filepath.Join(dir, "relevance-labels.csv")
	result, err := ExportEvaluation(context.Background(), EvaluationExportOptions{
		CandidatePath:    candidatePath,
		ScorePath:        scorePath,
		OutputPath:       outputPath,
		ScriptPath:       filepath.Join("..", "..", "scripts", "dev", "duckdb_export_relevance_eval.py"),
		SamplePerStratum: 1,
		Seed:             "fixture-v1",
	})
	if err != nil {
		t.Fatalf("export relevance evaluation: %v", err)
	}

	if result.RowsExported != 3 {
		t.Fatalf("expected 3 exported rows, got %d", result.RowsExported)
	}
	for _, stratum := range []string{"retain", "evaluate", "discard"} {
		if result.StratumCounts[stratum] != 1 {
			t.Fatalf("expected one %s row, got %v", stratum, result.StratumCounts)
		}
	}

	file, err := os.Open(outputPath)
	if err != nil {
		t.Fatalf("open evaluation fixture: %v", err)
	}
	defer file.Close()
	records, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatalf("read evaluation fixture: %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("expected header plus 3 rows, got %d", len(records))
	}

	header := map[string]int{}
	for index, column := range records[0] {
		header[column] = index
	}
	for _, column := range []string{
		"source_id",
		"original_text",
		"source_url",
		"sample_stratum",
		"label_travel",
		"label_saas_opportunity",
		"label_app_opportunity",
		"false_positive_category",
		"label_notes",
	} {
		if _, ok := header[column]; !ok {
			t.Fatalf("missing evaluation column %q: %v", column, records[0])
		}
	}
	for _, record := range records[1:] {
		if record[header["source_url"]] == "" || record[header["original_text"]] == "" {
			t.Fatalf("expected evidence text and backlink: %v", record)
		}
		for _, label := range []string{
			"label_travel",
			"label_saas_opportunity",
			"label_app_opportunity",
		} {
			if record[header[label]] != "" {
				t.Fatalf("expected blank analyst label %q, got %q", label, record[header[label]])
			}
		}
	}
}

func createEvaluationFixture(t *testing.T, candidatePath, scorePath string) {
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
            ('comment', 'retain-a', 'travel', 'Embassy appointment delays are frustrating.', 'https://reddit.test/retain-a', '["embassy","frustrating"]'::JSON, '["travel_language","pain_language"]'::JSON),
            ('comment', 'retain-b', 'apps', 'I need an app to automate this workflow.', 'https://reddit.test/retain-b', '["need an app","workflow"]'::JSON, '["request_intent","product_and_tool_language"]'::JSON),
            ('comment', 'evaluate-a', 'visas', 'Passport renewal process discussion.', 'https://reddit.test/evaluate-a', '["passport"]'::JSON, '["travel_language"]'::JSON),
            ('comment', 'evaluate-b', 'business', 'The reporting workflow is manual.', 'https://reddit.test/evaluate-b', '["reporting","manual"]'::JSON, '["business_workflow_language","workaround_language"]'::JSON),
            ('comment', 'discard-a', 'cards', 'Visa card payment information.', 'https://reddit.test/discard-a', '["visa","payment"]'::JSON, '["travel_language","pricing_and_payment"]'::JSON),
            ('comment', 'discard-b', 'games', 'Customs is a game map.', 'https://reddit.test/discard-b', '["customs"]'::JSON, '["travel_language"]'::JSON)
    ) AS t(source_type, source_id, subreddit, original_text, source_url, matched_terms, matched_rule_groups)
)
TO ? (FORMAT PARQUET)
""", [candidate_path])
con.execute("""
COPY (
    SELECT * FROM (
        VALUES
            ('comment','retain-a','travel',0.75,'B','retain'),
            ('comment','retain-a','saas_opportunity',0.10,'D','discard'),
            ('comment','retain-a','app_opportunity',0.10,'D','discard'),
            ('comment','retain-b','travel',0.00,'D','discard'),
            ('comment','retain-b','saas_opportunity',0.65,'B','retain'),
            ('comment','retain-b','app_opportunity',0.70,'B','retain'),
            ('comment','evaluate-a','travel',0.50,'C','evaluate'),
            ('comment','evaluate-a','saas_opportunity',0.00,'D','discard'),
            ('comment','evaluate-a','app_opportunity',0.00,'D','discard'),
            ('comment','evaluate-b','travel',0.00,'D','discard'),
            ('comment','evaluate-b','saas_opportunity',0.50,'C','evaluate'),
            ('comment','evaluate-b','app_opportunity',0.40,'C','evaluate'),
            ('comment','discard-a','travel',0.20,'D','discard'),
            ('comment','discard-a','saas_opportunity',0.20,'D','discard'),
            ('comment','discard-a','app_opportunity',0.20,'D','discard'),
            ('comment','discard-b','travel',0.10,'D','discard'),
            ('comment','discard-b','saas_opportunity',0.00,'D','discard'),
            ('comment','discard-b','app_opportunity',0.00,'D','discard')
    ) AS t(source_type, source_id, domain, relevance_score, relevance_tier, decision)
)
TO ? (FORMAT PARQUET)
""", [score_path])
`
	cmd := exec.Command("python3", "-c", script, candidatePath, scorePath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create evaluation fixture: %v: %s", err, output)
	}
}
