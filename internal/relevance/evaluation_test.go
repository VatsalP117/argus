package relevance

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
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
		RetainSample:     -1,
		EvaluateSample:   -1,
		DiscardSample:    -1,
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
		if result.StratumPopulations[stratum] != 2 {
			t.Fatalf("expected %s population 2, got %v", stratum, result.StratumPopulations)
		}
	}
	if result.SamplingSeed != "fixture-v1" {
		t.Fatalf("expected sampling seed fixture-v1, got %s", result.SamplingSeed)
	}

	records := readCSV(t, outputPath)
	if len(records) != 4 {
		t.Fatalf("expected header plus 3 rows, got %d", len(records))
	}

	header := headerMap(records[0])
	for _, column := range []string{
		"source_id",
		"text_excerpt",
		"source_url",
		"sample_stratum",
		"stratum_population",
		"sample_rank",
		"sampling_seed",
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
		if record[header["source_url"]] == "" || record[header["text_excerpt"]] == "" {
			t.Fatalf("expected evidence text and backlink: %v", record)
		}
		if record[header["sampling_seed"]] != "fixture-v1" {
			t.Fatalf("expected sampling seed in row: %v", record)
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

func TestExportEvaluationIncludesAllRetainedRowsWhenRetainSampleIsZero(t *testing.T) {
	dir := t.TempDir()
	candidatePath := filepath.Join(dir, "candidates.parquet")
	scorePath := filepath.Join(dir, "scores.parquet")
	createEvaluationFixture(t, candidatePath, scorePath)

	outputPath := filepath.Join(dir, "relevance-labels.csv")
	result, err := ExportEvaluation(context.Background(), EvaluationExportOptions{
		CandidatePath:  candidatePath,
		ScorePath:      scorePath,
		OutputPath:     outputPath,
		ScriptPath:     filepath.Join("..", "..", "scripts", "dev", "duckdb_export_relevance_eval.py"),
		SamplePerStratum: 1,
		RetainSample:   0,
		EvaluateSample: 1,
		DiscardSample:  1,
		Seed:           "retain-all",
	})
	if err != nil {
		t.Fatalf("export relevance evaluation: %v", err)
	}

	if result.StratumCounts["retain"] != 2 {
		t.Fatalf("expected both retained rows, got %d", result.StratumCounts["retain"])
	}
	if result.StratumCounts["evaluate"] != 1 || result.StratumCounts["discard"] != 1 {
		t.Fatalf("expected capped evaluate/discard rows, got %v", result.StratumCounts)
	}
	if result.StratumPopulations["retain"] != 2 {
		t.Fatalf("expected retain population 2, got %d", result.StratumPopulations["retain"])
	}
}

func TestExportEvaluationCapsStratumSamplesIndependently(t *testing.T) {
	dir := t.TempDir()
	candidatePath := filepath.Join(dir, "candidates.parquet")
	scorePath := filepath.Join(dir, "scores.parquet")
	createEvaluationFixture(t, candidatePath, scorePath)

	outputPath := filepath.Join(dir, "relevance-labels.csv")
	result, err := ExportEvaluation(context.Background(), EvaluationExportOptions{
		CandidatePath:  candidatePath,
		ScorePath:      scorePath,
		OutputPath:     outputPath,
		ScriptPath:     filepath.Join("..", "..", "scripts", "dev", "duckdb_export_relevance_eval.py"),
		SamplePerStratum: 1,
		RetainSample:   0,
		EvaluateSample: 0,
		DiscardSample:  0,
		Seed:           "all-rows",
	})
	if err != nil {
		t.Fatalf("export relevance evaluation: %v", err)
	}

	for _, stratum := range []string{"retain", "evaluate", "discard"} {
		if result.StratumCounts[stratum] != result.StratumPopulations[stratum] {
			t.Fatalf("expected %s sample to equal population, got %d/%d", stratum, result.StratumCounts[stratum], result.StratumPopulations[stratum])
		}
	}
	if result.RowsExported != 6 {
		t.Fatalf("expected all 6 rows, got %d", result.RowsExported)
	}
}

func TestExportEvaluationSamplingIsDeterministicBySeed(t *testing.T) {
	dir := t.TempDir()
	candidatePath := filepath.Join(dir, "candidates.parquet")
	scorePath := filepath.Join(dir, "scores.parquet")
	createEvaluationFixture(t, candidatePath, scorePath)

	scriptPath := filepath.Join("..", "..", "scripts", "dev", "duckdb_export_relevance_eval.py")
	outputA := filepath.Join(dir, "labels-a.csv")
	outputB := filepath.Join(dir, "labels-b.csv")

	_, err := ExportEvaluation(context.Background(), EvaluationExportOptions{
		CandidatePath:  candidatePath,
		ScorePath:      scorePath,
		OutputPath:     outputA,
		ScriptPath:     scriptPath,
		SamplePerStratum: 1,
		EvaluateSample: 1,
		DiscardSample:  1,
		Seed:           "same-seed",
	})
	if err != nil {
		t.Fatalf("export a: %v", err)
	}
	_, err = ExportEvaluation(context.Background(), EvaluationExportOptions{
		CandidatePath:  candidatePath,
		ScorePath:      scorePath,
		OutputPath:     outputB,
		ScriptPath:     scriptPath,
		SamplePerStratum: 1,
		EvaluateSample: 1,
		DiscardSample:  1,
		Seed:           "same-seed",
	})
	if err != nil {
		t.Fatalf("export b: %v", err)
	}

	idsA := sourceIDs(t, outputA)
	idsB := sourceIDs(t, outputB)
	if !reflect.DeepEqual(idsA, idsB) {
		t.Fatalf("same seed produced different source IDs: %v vs %v", idsA, idsB)
	}
}

func TestExportEvaluationDifferentSeedsCanChangeSample(t *testing.T) {
	dir := t.TempDir()
	candidatePath := filepath.Join(dir, "candidates.parquet")
	scorePath := filepath.Join(dir, "scores.parquet")
	createLargeEvaluationFixture(t, candidatePath, scorePath, 10)

	scriptPath := filepath.Join("..", "..", "scripts", "dev", "duckdb_export_relevance_eval.py")
	outputA := filepath.Join(dir, "labels-a.csv")
	outputB := filepath.Join(dir, "labels-b.csv")

	_, err := ExportEvaluation(context.Background(), EvaluationExportOptions{
		CandidatePath:    candidatePath,
		ScorePath:        scorePath,
		OutputPath:       outputA,
		ScriptPath:       scriptPath,
		SamplePerStratum: 5,
		RetainSample:     -1,
		EvaluateSample:   -1,
		DiscardSample:    -1,
		Seed:             "seed-a",
	})
	if err != nil {
		t.Fatalf("export a: %v", err)
	}
	_, err = ExportEvaluation(context.Background(), EvaluationExportOptions{
		CandidatePath:    candidatePath,
		ScorePath:        scorePath,
		OutputPath:       outputB,
		ScriptPath:       scriptPath,
		SamplePerStratum: 5,
		RetainSample:     -1,
		EvaluateSample:   -1,
		DiscardSample:    -1,
		Seed:             "seed-b",
	})
	if err != nil {
		t.Fatalf("export b: %v", err)
	}

	idsA := sourceIDs(t, outputA)
	idsB := sourceIDs(t, outputB)
	if reflect.DeepEqual(idsA, idsB) {
		t.Fatalf("different seeds produced identical source IDs: %v", idsA)
	}
}

func createLargeEvaluationFixture(t *testing.T, candidatePath, scorePath string, perStratum int) {
	t.Helper()
	candidates := make([]string, 0, perStratum*3)
	scores := make([]string, 0, perStratum*9)
	for i := 0; i < perStratum; i++ {
		sourceID := fmt.Sprintf("retain-%03d", i)
		candidates = append(candidates, fmt.Sprintf("('comment', '%s', 'travel', 'text %d.', 'https://reddit.test/%s', '[]'::JSON, '[]'::JSON)", sourceID, i, sourceID))
		scores = append(scores,
			fmt.Sprintf("('comment','%s','travel',0.75,'B','retain')", sourceID),
			fmt.Sprintf("('comment','%s','saas_opportunity',0.10,'D','discard')", sourceID),
			fmt.Sprintf("('comment','%s','app_opportunity',0.10,'D','discard')", sourceID),
		)
	}
	for i := 0; i < perStratum; i++ {
		sourceID := fmt.Sprintf("evaluate-%03d", i)
		candidates = append(candidates, fmt.Sprintf("('comment', '%s', 'visas', 'text %d.', 'https://reddit.test/%s', '[]'::JSON, '[]'::JSON)", sourceID, i, sourceID))
		scores = append(scores,
			fmt.Sprintf("('comment','%s','travel',0.50,'C','evaluate')", sourceID),
			fmt.Sprintf("('comment','%s','saas_opportunity',0.00,'D','discard')", sourceID),
			fmt.Sprintf("('comment','%s','app_opportunity',0.00,'D','discard')", sourceID),
		)
	}
	for i := 0; i < perStratum; i++ {
		sourceID := fmt.Sprintf("discard-%03d", i)
		candidates = append(candidates, fmt.Sprintf("('comment', '%s', 'cards', 'text %d.', 'https://reddit.test/%s', '[]'::JSON, '[]'::JSON)", sourceID, i, sourceID))
		scores = append(scores,
			fmt.Sprintf("('comment','%s','travel',0.20,'D','discard')", sourceID),
			fmt.Sprintf("('comment','%s','saas_opportunity',0.20,'D','discard')", sourceID),
			fmt.Sprintf("('comment','%s','app_opportunity',0.20,'D','discard')", sourceID),
		)
	}

	script := fmt.Sprintf(`
import duckdb
import sys
candidate_path, score_path = sys.argv[1], sys.argv[2]
con = duckdb.connect()
con.execute("""
COPY (
    SELECT * FROM (VALUES %s) AS t(source_type, source_id, subreddit, original_text, source_url, matched_terms, matched_rule_groups)
) TO ? (FORMAT PARQUET)
""", [candidate_path])
con.execute("""
COPY (
    SELECT * FROM (VALUES %s) AS t(source_type, source_id, domain, relevance_score, relevance_tier, decision)
) TO ? (FORMAT PARQUET)
""", [score_path])
`, stringTupleList(candidates), stringTupleList(scores))
	cmd := exec.Command("python3", "-c", script, candidatePath, scorePath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create large evaluation fixture: %v: %s", err, output)
	}
}

func stringTupleList(items []string) string {
	return strings.Join(items, ", ")
}

func sourceIDs(t *testing.T, path string) []string {
	t.Helper()
	records := readCSV(t, path)
	header := headerMap(records[0])
	ids := make([]string, 0, len(records)-1)
	for _, record := range records[1:] {
		ids = append(ids, record[header["source_id"]])
	}
	return ids
}

func readCSV(t *testing.T, path string) [][]string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			t.Errorf("close %s: %v", path, cerr)
		}
	}()
	records, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return records
}

func headerMap(columns []string) map[string]int {
	m := make(map[string]int, len(columns))
	for i, column := range columns {
		m[column] = i
	}
	return m
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
