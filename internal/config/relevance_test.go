package config

import "testing"

func TestLoadRelevanceConfigDefinesExplainableDomainsAndTiers(t *testing.T) {
	cfg, err := LoadRelevanceConfig("../../configs/relevance/deterministic-v1.yaml")
	if err != nil {
		t.Fatalf("load relevance config: %v", err)
	}

	if cfg.Version != "deterministic_v1" {
		t.Fatalf("unexpected version: %s", cfg.Version)
	}
	if cfg.Tiers.A != 0.80 || cfg.Tiers.B != 0.60 || cfg.Tiers.C != 0.40 {
		t.Fatalf("unexpected tier thresholds: %+v", cfg.Tiers)
	}

	requiredDomains := map[string]bool{
		"travel":           false,
		"saas_opportunity": false,
		"app_opportunity":  false,
	}
	for _, domain := range cfg.Domains {
		if _, ok := requiredDomains[domain.Name]; ok {
			requiredDomains[domain.Name] = true
		}
		if len(domain.GroupWeights) == 0 {
			t.Fatalf("domain %q has no group weights", domain.Name)
		}
		if domain.Name == "travel" && len(domain.RequiredAnyTerms) == 0 {
			t.Fatal("travel domain must require concrete anchor terms")
		}
	}
	for domain, found := range requiredDomains {
		if !found {
			t.Fatalf("required relevance domain %q is missing", domain)
		}
	}
}

func TestLoadRelevanceV2DefinesContextAndEligibilityControls(t *testing.T) {
	cfg, err := LoadRelevanceConfig("../../configs/relevance/deterministic-v2.yaml")
	if err != nil {
		t.Fatalf("load relevance v2 config: %v", err)
	}
	if cfg.Version != "deterministic_v2" {
		t.Fatalf("unexpected version: %s", cfg.Version)
	}

	for _, domain := range cfg.Domains {
		if len(domain.ContextWeights) == 0 {
			t.Fatalf("domain %q has no contextual boosts", domain.Name)
		}
		if len(domain.ContextPenaltyWeights) == 0 {
			t.Fatalf("domain %q has no ambiguity penalties", domain.Name)
		}
		if domain.Name != "travel" && len(domain.RequiredAnyGroups) == 0 {
			t.Fatalf("opportunity domain %q has no required evidence groups", domain.Name)
		}
	}
}
