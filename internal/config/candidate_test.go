package config

import "testing"

func TestLoadCandidateConfigProvidesBroadResearchCoverage(t *testing.T) {
	cfg, err := LoadCandidateConfig("../../configs/candidates/broad-v1.yaml")
	if err != nil {
		t.Fatalf("load candidate config: %v", err)
	}

	if cfg.Version != "broad_v1" {
		t.Fatalf("unexpected version: %s", cfg.Version)
	}
	if cfg.MinimumTextLength != 40 {
		t.Fatalf("unexpected minimum text length: %d", cfg.MinimumTextLength)
	}

	requiredGroups := map[string]bool{
		"pain_language":              false,
		"request_intent":             false,
		"workaround_language":        false,
		"product_and_tool_language":  false,
		"travel_language":            false,
		"business_workflow_language": false,
		"pricing_and_payment":        false,
	}
	for _, group := range cfg.RuleGroups {
		if _, ok := requiredGroups[group.Name]; ok {
			requiredGroups[group.Name] = true
		}
	}
	for name, found := range requiredGroups {
		if !found {
			t.Fatalf("required broad candidate group %q is missing", name)
		}
	}

	if len(cfg.SubredditPriors) == 0 {
		t.Fatal("expected subreddit priors without making them an ingest gate")
	}
}
