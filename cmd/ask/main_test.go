package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"argus/internal/config"
	"argus/internal/llm"
)

func TestSanitizePlanLimitsAndFilters(t *testing.T) {
	plan := []plannedQuery{
		{
			QueryName: "signal_summary",
			Filters: map[string]string{
				"signal-type": "Pain_Point",
				"topic-hint":  "Visas",
				"bad-filter":  "ignored",
			},
			Limit: 99,
		},
		{
			QueryName: "not_allowed",
			Filters:   map[string]string{},
			Limit:     5,
		},
	}

	got := sanitizePlan(plan, "what pain points exist?", 4)
	if len(got) == 0 {
		t.Fatalf("expected sanitized plan")
	}
	if got[0].QueryName != "signal_summary" {
		t.Fatalf("unexpected query name: %#v", got[0])
	}
	if got[0].Limit != 4 {
		t.Fatalf("expected default limit fallback, got %#v", got[0])
	}
	if got[0].Filters["signal-type"] != "pain_point" {
		t.Fatalf("unexpected filters: %#v", got[0].Filters)
	}
	if got[0].Filters["topic-hint"] != "visa" {
		t.Fatalf("unexpected topic-hint normalization: %#v", got[0].Filters)
	}
	if _, ok := got[0].Filters["bad-filter"]; ok {
		t.Fatalf("unexpected bad filter survived: %#v", got[0].Filters)
	}
}

func TestFallbackPlanForPainPointQuestion(t *testing.T) {
	got := fallbackPlan("What pain points come up most often?", 5)
	if len(got) < 2 {
		t.Fatalf("expected two fallback queries, got %#v", got)
	}
	if got[0].QueryName != "signal_summary" {
		t.Fatalf("unexpected first query: %#v", got[0])
	}
	if got[0].Filters["signal-type"] != "pain_point" {
		t.Fatalf("unexpected first filters: %#v", got[0].Filters)
	}
}

func TestExtractKeywordFallback(t *testing.T) {
	if got := extractKeywordFallback("Find quotes about visa delays in travel posts"); got != "visa" {
		t.Fatalf("unexpected fallback keyword: %q", got)
	}
}

func TestTopicAwareFallbackPlanUsesSingularizedTopicHint(t *testing.T) {
	currentPlan := []plannedQuery{
		{
			QueryName: "signal_summary",
			Filters: map[string]string{
				"signal-type": "pain_point",
				"topic-hint":  "visas",
			},
			Limit: 6,
		},
	}

	got := topicAwareFallbackPlan("What pain points about visas come up most often?", currentPlan, 6)
	if len(got) != 2 {
		t.Fatalf("expected two topic-aware fallback queries, got %#v", got)
	}
	if got[0].Filters["contains-text"] != "visa" {
		t.Fatalf("unexpected contains-text: %#v", got[0].Filters)
	}
	if got[0].Filters["signal-type"] != "pain_point" {
		t.Fatalf("unexpected signal-type: %#v", got[0].Filters)
	}
	if got[0].Filters["topic-hint"] != "*" {
		t.Fatalf("expected wildcard topic-hint in broadened plan, got %#v", got[0].Filters)
	}
}

func TestExtractKeywordFallbackSkipsGenericQuestionWords(t *testing.T) {
	if got := extractKeywordFallback("What pain points about visas come up most often?"); got != "visa" {
		t.Fatalf("unexpected fallback keyword: %q", got)
	}
}

func TestRunAskFlowWithMockLLM(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(wd, "..", ".."))
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir returned error: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")

		switch callCount {
		case 1:
			_, _ = w.Write([]byte(`{
				"id": "planner_1",
				"model": "deepseek-v4-flash",
				"choices": [
					{
						"message": {
							"content": "{\"intent\":\"pain_point_analysis\",\"query_plan\":[{\"query_name\":\"signal_summary\",\"filters\":{\"signal-type\":\"pain_point\",\"topic-hint\":\"visa\"},\"limit\":3,\"reason\":\"Summarize recurring visa pain points\"},{\"query_name\":\"signal_evidence\",\"filters\":{\"signal-type\":\"pain_point\",\"topic-hint\":\"visa\"},\"limit\":2,\"reason\":\"Inspect representative evidence rows\"}],\"notes\":[\"Focus on visa-related pain points only.\"]}"
						}
					}
				]
			}`))
		default:
			_, _ = w.Write([]byte(`{
				"id": "answer_1",
				"model": "deepseek-v4-flash",
				"choices": [
					{
						"message": {
							"content": "{\"summary\":\"Visa-related pain points recur across the frozen slice, with difficulty and process friction appearing repeatedly.\",\"claims\":[{\"statement\":\"Visa pain points show up repeatedly in the deterministic signals for the frozen travel slice.\",\"evidence_refs\":[\"q1.r1\",\"q2.r1\"],\"confidence\":\"medium\"}],\"evidence\":[{\"ref\":\"q1.r1\",\"why\":\"This summary row shows the leading visa pain-point pattern.\"}],\"caveats\":[\"This answer is limited to the deterministic Jan-Feb 2021 V0 slice.\"]}"
						}
					}
				]
			}`))
		}
	}))
	defer server.Close()

	cfg, err := config.LoadPipelineConfig("configs/pipelines/v0-travel-jan-feb-2021.yaml")
	if err != nil {
		t.Fatalf("LoadPipelineConfig returned error: %v", err)
	}

	client := llm.NewClient(server.URL, "test-key")
	output, err := runAskFlow(
		context.Background(),
		client,
		cfg,
		"deepseek",
		"deepseek-v4-flash",
		"What pain points about visas come up most often?",
		"*",
		3,
		false,
		duckDBOptions{MemoryLimit: "4GB", Threads: 2, TempDir: ".duckdb/tmp"},
	)
	if err != nil {
		t.Fatalf("runAskFlow returned error: %v", err)
	}
	if output.Intent != "pain_point_analysis" {
		t.Fatalf("unexpected intent: %#v", output)
	}
	if len(output.QueryResults) != 2 {
		t.Fatalf("unexpected query results: %#v", output.QueryResults)
	}
	if !strings.Contains(output.Answer.Summary, "Visa-related pain points recur") {
		t.Fatalf("unexpected answer summary: %#v", output.Answer)
	}
	if len(output.QueryResults[0].Rows) == 0 {
		t.Fatalf("expected at least one retrieved row")
	}
}
