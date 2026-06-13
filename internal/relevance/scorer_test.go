package relevance

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"argus/internal/config"
)

func TestScoreProducesDomainTiersAndRetentionDecisions(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "candidates.parquet")
	createCandidateFixture(t, inputPath)

	cfg := config.RelevanceConfig{
		Version: "test_v1",
		Tiers: config.RelevanceTiers{
			A: 0.80,
			B: 0.60,
			C: 0.40,
		},
		Domains: []config.RelevanceDomain{
			{
				Name:                 "travel",
				SubredditPriorWeight: 0.15,
				RequiredAnyTerms:     []string{"visa", "airport", "flight"},
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
	}

	result, err := Score(context.Background(), Options{
		InputPath:      inputPath,
		OutputPath:     filepath.Join(dir, "scores.parquet"),
		ScriptPath:     filepath.Join("..", "..", "scripts", "dev", "duckdb_score_candidates.py"),
		TempDir:        filepath.Join(dir, "duckdb-tmp"),
		MemoryLimit:    "1GB",
		Threads:        1,
		RelevanceRules: cfg,
	})
	if err != nil {
		t.Fatalf("score candidates: %v", err)
	}

	if result.RowsCandidates != 5 {
		t.Fatalf("expected 5 candidates, got %d", result.RowsCandidates)
	}
	if result.RowsScored != 15 {
		t.Fatalf("expected 15 domain scores, got %d", result.RowsScored)
	}
	if result.RowsRetainedCandidates != 2 {
		t.Fatalf("expected 2 retained candidates, got %d", result.RowsRetainedCandidates)
	}
	if result.TierCounts["A"] != 1 || result.TierCounts["B"] != 2 {
		t.Fatalf("unexpected tier counts: %v", result.TierCounts)
	}
	if info, err := os.Stat(result.OutputPath); err != nil || info.Size() == 0 {
		t.Fatalf("expected non-empty score parquet, info=%v err=%v", info, err)
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
            ('comment', 'saas-request', false, false, '["product_and_tool_language","business_workflow_language","request_intent"]'::JSON, '["software","workflow","need an app"]'::JSON),
            ('comment', 'generic-refund', false, false, '["travel_language","pricing_and_payment","comparison_and_dissatisfaction"]'::JSON, '["refund","customer service"]'::JSON),
            ('comment', 'moderation-bot', false, true, '["product_and_tool_language","business_workflow_language","request_intent"]'::JSON, '["software","reporting","feature request"]'::JSON)
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
