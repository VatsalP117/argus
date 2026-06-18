package relevance

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestScoreUsesContextBoostsPenaltiesAndRequiredGroups(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "candidates.parquet")
	createContextCandidateFixture(t, inputPath)

	cfg := config.RelevanceConfig{
		Version: "test_v2",
		Tiers: config.RelevanceTiers{
			A: 0.80,
			B: 0.60,
			C: 0.40,
		},
		Domains: []config.RelevanceDomain{
			{
				Name:             "travel",
				RequiredAnyTerms: []string{"visa"},
				GroupWeights: map[string]float64{
					"travel_language": 0.50,
				},
				ContextWeights: map[string]float64{
					"work visa": 0.20,
				},
				ContextPenaltyWeights: map[string]float64{
					"visa card": 0.70,
				},
			},
			{
				Name:              "app_opportunity",
				RequiredAnyGroups: []string{"product_and_tool_language"},
				GroupWeights: map[string]float64{
					"product_and_tool_language": 0.25,
					"pain_language":             0.15,
				},
				ContextWeights: map[string]float64{
					"laggy":         0.20,
					"does not work": 0.20,
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

	if result.RowsCandidates != 5 || result.RowsScored != 10 {
		t.Fatalf("unexpected scoring counts: %+v", result)
	}
	if result.RowsRetainedCandidates != 2 {
		t.Fatalf("expected genuine visa and broken app to retain, got %+v", result)
	}
}

func TestScoreRequiresMinimumGroupMatchesWhenConfigured(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "candidates.parquet")
	createMinimumGroupFixture(t, inputPath)

	cfg := config.RelevanceConfig{
		Version: "test_v3",
		Tiers: config.RelevanceTiers{
			A: 0.80,
			B: 0.60,
			C: 0.40,
		},
		Domains: []config.RelevanceDomain{
			{
				Name:                "app_opportunity",
				MinimumGroupMatches: 2,
				GroupWeights: map[string]float64{
					"product_and_tool_language": 0.25,
					"pain_language":             0.20,
					"workaround_language":       0.20,
				},
				ContextWeights: map[string]float64{
					"does not work": 0.20,
					"workaround":    0.15,
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

	if result.RowsCandidates != 3 || result.RowsRetainedCandidates != 1 {
		t.Fatalf("expected only corroborated app pain to retain, got %+v", result)
	}
}

func TestDeterministicV3CharacterizesObservedTrapAndRecallBehaviors(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "candidates.parquet")
	createDeterministicV3Fixture(t, inputPath)

	cfg, err := config.LoadRelevanceConfig(filepath.Join("..", "..", "configs", "relevance", "deterministic-v3.yaml"))
	if err != nil {
		t.Fatalf("load v3 config: %v", err)
	}

	outputPath := filepath.Join(dir, "scores.parquet")
	_, err = Score(context.Background(), Options{
		InputPath:      inputPath,
		OutputPath:     outputPath,
		ScriptPath:     filepath.Join("..", "..", "scripts", "dev", "duckdb_score_candidates.py"),
		TempDir:        filepath.Join(dir, "duckdb-tmp"),
		MemoryLimit:    "1GB",
		Threads:        1,
		RelevanceRules: cfg,
	})
	if err != nil {
		t.Fatalf("score candidates: %v", err)
	}

	decisions := readDecisionSummary(t, outputPath)

	if decisions["travel-process-pain|travel"].Decision != "retain" {
		t.Fatalf("expected concrete travel pain to retain, got %+v", decisions["travel-process-pain|travel"])
	}
	if decisions["broken-app|app_opportunity"].Decision != "retain" {
		t.Fatalf("expected broken app behavior to retain, got %+v", decisions["broken-app|app_opportunity"])
	}
	if decisions["political-h1b|travel"].Decision == "retain" {
		t.Fatalf("expected political immigration to avoid retain, got %+v", decisions["political-h1b|travel"])
	}
	if decisions["referral-promo|app_opportunity"].Decision == "retain" {
		t.Fatalf("expected referral promo to avoid retain, got %+v", decisions["referral-promo|app_opportunity"])
	}
	if decisions["generic-app|app_opportunity"].Decision == "retain" {
		t.Fatalf("expected generic app mention to avoid retain, got %+v", decisions["generic-app|app_opportunity"])
	}
	if !strings.Contains(decisions["political-h1b|travel"].Reasons, "penalty:h1b") &&
		!strings.Contains(decisions["political-h1b|travel"].Reasons, "penalty:h1b visa") {
		t.Fatalf("expected political travel ambiguity reason, got %+v", decisions["political-h1b|travel"])
	}
}

func TestProximityBoostFiresForNearAnchorAndEvidence(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "candidates.parquet")
	createProximityFixture(t, inputPath)

	cfg := config.RelevanceConfig{
		Version: "test_proximity",
		Tiers:   config.RelevanceTiers{A: 0.80, B: 0.60, C: 0.40},
		Domains: []config.RelevanceDomain{
			{
				Name:             "travel",
				RequiredAnyTerms: []string{"hostel"},
				GroupWeights: map[string]float64{
					"travel_language": 0.50,
				},
				ProximityRules: []config.ProximityRule{
					{
						Name:         "travel_safety_loss",
						Anchors:      []string{"hostel"},
						Evidence:     []string{"stolen"},
						WindowTokens: 8,
						Weight:       0.20,
					},
				},
			},
		},
	}

	outputPath := filepath.Join(dir, "scores.parquet")
	_, err := Score(context.Background(), Options{
		InputPath:      inputPath,
		OutputPath:     outputPath,
		ScriptPath:     filepath.Join("..", "..", "scripts", "dev", "duckdb_score_candidates.py"),
		TempDir:        filepath.Join(dir, "duckdb-tmp"),
		MemoryLimit:    "1GB",
		Threads:        1,
		RelevanceRules: cfg,
	})
	if err != nil {
		t.Fatalf("score candidates: %v", err)
	}

	decisions := readDecisionSummary(t, outputPath)

	near := decisions["hostel-theft-near|travel"]
	if near.Decision != "retain" {
		t.Fatalf("expected near hostel+theft to retain via proximity boost, got %+v", near)
	}
	if !strings.Contains(near.Reasons, "proximity:travel_safety_loss") {
		t.Fatalf("expected proximity reason in near case, got %+v", near)
	}

	far := decisions["hostel-theft-far|travel"]
	if far.Decision == "retain" {
		t.Fatalf("expected far-apart hostel+theft to avoid retain, got %+v", far)
	}
	if strings.Contains(far.Reasons, "proximity:travel_safety_loss") {
		t.Fatalf("expected no proximity reason in far case, got %+v", far)
	}

	generic := decisions["hostel-only|travel"]
	if generic.Decision == "retain" {
		t.Fatalf("expected generic hostel mention to avoid retain, got %+v", generic)
	}
	if strings.Contains(generic.Reasons, "proximity:travel_safety_loss") {
		t.Fatalf("expected no proximity reason without evidence, got %+v", generic)
	}
}

func TestDeterministicV4RetainsProximityEvidenceWithoutTrapLeakage(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "candidates.parquet")
	createDeterministicV4Fixture(t, inputPath)

	cfg, err := config.LoadRelevanceConfig(filepath.Join("..", "..", "configs", "relevance", "deterministic-v4.yaml"))
	if err != nil {
		t.Fatalf("load v4 config: %v", err)
	}

	outputPath := filepath.Join(dir, "scores.parquet")
	_, err = Score(context.Background(), Options{
		InputPath:      inputPath,
		OutputPath:     outputPath,
		ScriptPath:     filepath.Join("..", "..", "scripts", "dev", "duckdb_score_candidates.py"),
		TempDir:        filepath.Join(dir, "duckdb-tmp"),
		MemoryLimit:    "1GB",
		Threads:        1,
		RelevanceRules: cfg,
	})
	if err != nil {
		t.Fatalf("score candidates: %v", err)
	}

	decisions := readDecisionSummary(t, outputPath)

	hostelTheft := decisions["hostel-theft-pain|travel"]
	if hostelTheft.Decision != "retain" {
		t.Fatalf("expected hostel theft travel safety to retain via proximity, got %+v", hostelTheft)
	}
	if !strings.Contains(hostelTheft.Reasons, "proximity:travel_safety_loss") {
		t.Fatalf("expected proximity:travel_safety_loss reason, got %+v", hostelTheft)
	}

	borderSecurity := decisions["border-security-pain|travel"]
	if borderSecurity.Decision != "retain" {
		t.Fatalf("expected border security travel pain to retain via proximity, got %+v", borderSecurity)
	}
	if !strings.Contains(borderSecurity.Reasons, "proximity:travel_border_security") {
		t.Fatalf("expected proximity:travel_border_security reason, got %+v", borderSecurity)
	}

	brokenSwitch := decisions["switch-failure-pain|app_opportunity"]
	if brokenSwitch.Decision != "retain" {
		t.Fatalf("expected switch failure app pain to retain via proximity, got %+v", brokenSwitch)
	}
	if !strings.Contains(brokenSwitch.Reasons, "proximity:app_failure_evidence") {
		t.Fatalf("expected proximity:app_failure_evidence reason, got %+v", brokenSwitch)
	}

	genericApp := decisions["generic-app-mention|app_opportunity"]
	if genericApp.Decision == "retain" {
		t.Fatalf("expected generic app mention to avoid retain, got %+v", genericApp)
	}
	if strings.Contains(genericApp.Reasons, "proximity:app_failure_evidence") {
		t.Fatalf("expected no proximity reason for generic app mention, got %+v", genericApp)
	}

	politicalH1b := decisions["political-h1b|travel"]
	if politicalH1b.Decision == "retain" {
		t.Fatalf("expected political immigration to avoid retain under v4, got %+v", politicalH1b)
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
            ('comment', 'travel-pain', 'travel', 'Visa processing is frustrating.', false, false, '["travel_language","pain_language"]'::JSON, '["visa","frustrating"]'::JSON),
            ('comment', 'prior-only', 'travel', 'A cheerful trip story.', true, false, '[]'::JSON, '[]'::JSON),
            ('comment', 'saas-request', 'business', 'I need an app for this workflow.', false, false, '["product_and_tool_language","business_workflow_language","request_intent"]'::JSON, '["software","workflow","need an app"]'::JSON),
            ('comment', 'generic-refund', 'shopping', 'The refund customer service was slow.', false, false, '["travel_language","pricing_and_payment","comparison_and_dissatisfaction"]'::JSON, '["refund","customer service"]'::JSON),
            ('comment', 'moderation-bot', 'tools', 'Feature request reporting bot.', false, true, '["product_and_tool_language","business_workflow_language","request_intent"]'::JSON, '["software","reporting","feature request"]'::JSON)
    ) AS t(source_type, source_id, subreddit, candidate_text, subreddit_prior_match, is_bot_like, matched_rule_groups, matched_terms)
)
TO ? (FORMAT PARQUET)
""", [sys.argv[1]])
`
	cmd := exec.Command("python3", "-c", script, outputPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create candidate fixture: %v: %s", err, output)
	}
}

func createContextCandidateFixture(t *testing.T, outputPath string) {
	t.Helper()
	script := `
import duckdb
import sys
con = duckdb.connect()
con.execute("""
COPY (
    SELECT * FROM (
        VALUES
            ('comment', 'genuine-visa', 'immigration', 'My work visa sponsorship was rejected.', false, false, '["travel_language"]'::JSON, '["visa"]'::JSON),
            ('comment', 'visa-card', 'creditcards', 'My Visa card payment was rejected.', false, false, '["travel_language","pricing_and_payment"]'::JSON, '["visa","payment"]'::JSON),
            ('comment', 'broken-app', 'android', 'The app is laggy and does not work.', false, false, '["product_and_tool_language","pain_language"]'::JSON, '["app"]'::JSON),
            ('comment', 'generic-app', 'android', 'I downloaded the app yesterday.', false, false, '["product_and_tool_language"]'::JSON, '["app"]'::JSON),
            ('comment', 'pain-without-product', 'life', 'This process is laggy and does not work.', false, false, '["pain_language"]'::JSON, '["frustrating"]'::JSON)
    ) AS t(source_type, source_id, subreddit, candidate_text, subreddit_prior_match, is_bot_like, matched_rule_groups, matched_terms)
)
TO ? (FORMAT PARQUET)
""", [sys.argv[1]])
`
	cmd := exec.Command("python3", "-c", script, outputPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create contextual candidate fixture: %v: %s", err, output)
	}
}

func createMinimumGroupFixture(t *testing.T, outputPath string) {
	t.Helper()
	script := `
import duckdb
import sys
con = duckdb.connect()
con.execute("""
COPY (
    SELECT * FROM (
        VALUES
            ('comment', 'broken-app', 'android', 'The app does not work and I need a workaround.', false, false, '["product_and_tool_language","pain_language","workaround_language"]'::JSON, '["app"]'::JSON),
            ('comment', 'generic-app', 'android', 'The app does not work.', false, false, '["product_and_tool_language"]'::JSON, '["app"]'::JSON),
            ('comment', 'pain-only', 'life', 'This does not work for me.', false, false, '["pain_language"]'::JSON, '["pain"]'::JSON)
    ) AS t(source_type, source_id, subreddit, candidate_text, subreddit_prior_match, is_bot_like, matched_rule_groups, matched_terms)
)
TO ? (FORMAT PARQUET)
""", [sys.argv[1]])
`
	cmd := exec.Command("python3", "-c", script, outputPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create minimum-group fixture: %v: %s", err, output)
	}
}

type decisionSummary struct {
	Decision string
	Reasons  string
}

func readDecisionSummary(t *testing.T, scorePath string) map[string]decisionSummary {
	t.Helper()
	script := `
import duckdb
import json
import sys
con = duckdb.connect()
rows = con.execute("""
    select source_id, domain, decision, decision_reasons
    from read_parquet(?)
""", [sys.argv[1]]).fetchall()
print(json.dumps({
    f"{source_id}|{domain}": {
        "decision": decision,
        "reasons": decision_reasons,
    }
    for source_id, domain, decision, decision_reasons in rows
}))
`
	cmd := exec.Command("python3", "-c", script, scorePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("read decision summary: %v: %s", err, output)
	}

	type payload struct {
		Decision string `json:"decision"`
		Reasons  string `json:"reasons"`
	}
	var raw map[string]payload
	if err := json.Unmarshal(output, &raw); err != nil {
		t.Fatalf("parse decision summary: %v: %s", err, output)
	}
	result := make(map[string]decisionSummary, len(raw))
	for key, value := range raw {
		result[key] = decisionSummary{
			Decision: value.Decision,
			Reasons:  value.Reasons,
		}
	}
	return result
}

func createDeterministicV3Fixture(t *testing.T, outputPath string) {
	t.Helper()
	script := `
import duckdb
import sys
con = duckdb.connect()
con.execute("""
COPY (
    SELECT * FROM (
        VALUES
            ('comment', 'travel-process-pain', 'travel', 'My passport renewal and embassy appointment are blocked and the airline canceled everything.', false, false, '["travel_language","pain_language"]'::JSON, '["passport","embassy","airline"]'::JSON),
            ('comment', 'political-h1b', 'politics', 'This is designed to get someone on a H1B visa and Biden will revert immigration rules.', false, false, '["travel_language","pain_language"]'::JSON, '["visa","immigration"]'::JSON),
            ('comment', 'broken-app', 'android', 'The app crashes, does not work, and I need a workaround.', false, false, '["product_and_tool_language","pain_language","workaround_language"]'::JSON, '["app"]'::JSON),
            ('comment', 'generic-app', 'android', 'I installed the app yesterday and it loads.', false, false, '["product_and_tool_language"]'::JSON, '["app"]'::JSON),
            ('comment', 'referral-promo', 'crypto', 'Use this referral code for bonus money in the app.', false, false, '["product_and_tool_language","workaround_language","comparison_and_dissatisfaction"]'::JSON, '["app"]'::JSON)
    ) AS t(source_type, source_id, subreddit, candidate_text, subreddit_prior_match, is_bot_like, matched_rule_groups, matched_terms)
)
TO ? (FORMAT PARQUET)
""", [sys.argv[1]])
`
	cmd := exec.Command("python3", "-c", script, outputPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create deterministic-v3 fixture: %v: %s", err, output)
	}
}

func createProximityFixture(t *testing.T, outputPath string) {
	t.Helper()
	script := `
import duckdb
import sys
con = duckdb.connect()
con.execute("""
COPY (
    SELECT * FROM (
        VALUES
            ('comment', 'hostel-theft-near', 'travel', 'In the hostel my laptop was stolen overnight.', false, false, '["travel_language"]'::JSON, '["hostel"]'::JSON),
            ('comment', 'hostel-theft-far', 'travel', 'The hostel was clean and the food was great. Later that month I read about stolen phones elsewhere.', false, false, '["travel_language"]'::JSON, '["hostel"]'::JSON),
            ('comment', 'hostel-only', 'travel', 'I stayed at a hostel in Berlin last summer.', false, false, '["travel_language"]'::JSON, '["hostel"]'::JSON)
    ) AS t(source_type, source_id, subreddit, candidate_text, subreddit_prior_match, is_bot_like, matched_rule_groups, matched_terms)
)
TO ? (FORMAT PARQUET)
""", [sys.argv[1]])
`
	cmd := exec.Command("python3", "-c", script, outputPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create proximity fixture: %v: %s", err, output)
	}
}

func createDeterministicV4Fixture(t *testing.T, outputPath string) {
	t.Helper()
	script := `
import duckdb
import sys
con = duckdb.connect()
con.execute("""
COPY (
    SELECT * FROM (
        VALUES
            ('comment', 'hostel-theft-pain', 'travel', 'Guest on Guest theft is common in hostels. People pretending to be guests at the hostel walk out with stolen computers.', false, false, '["travel_language","pain_language"]'::JSON, '["hostel","hostels"]'::JSON),
            ('comment', 'border-security-pain', 'travel', 'I was asked insane questions by border security trying to enter Australia. The security guard detained and questioned me at customs. I need to know how to avoid this.', false, false, '["travel_language","request_intent"]'::JSON, '["customs","border","immigration"]'::JSON),
            ('comment', 'switch-failure-pain', 'android', 'The switch does not work at all now. I air gapped it and the app shows an error. It fails to factory reset.', false, false, '["product_and_tool_language","pain_language","workaround_language"]'::JSON, '["app","switch"]'::JSON),
            ('comment', 'generic-app-mention', 'android', 'I downloaded the app and it installed fine.', false, false, '["product_and_tool_language"]'::JSON, '["app"]'::JSON),
            ('comment', 'political-h1b', 'politics', 'This is designed to get someone on a H1B visa and Biden will revert immigration rules.', false, false, '["travel_language","pain_language"]'::JSON, '["visa","immigration"]'::JSON)
    ) AS t(source_type, source_id, subreddit, candidate_text, subreddit_prior_match, is_bot_like, matched_rule_groups, matched_terms)
)
TO ? (FORMAT PARQUET)
""", [sys.argv[1]])
`
	cmd := exec.Command("python3", "-c", script, outputPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create deterministic-v4 fixture: %v: %s", err, output)
	}
}
