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

func TestLoadRelevanceV3DefinesVersionedCalibrationCandidate(t *testing.T) {
	cfg, err := LoadRelevanceConfig("../../configs/relevance/deterministic-v3.yaml")
	if err != nil {
		t.Fatalf("load relevance v3 config: %v", err)
	}
	if cfg.Version != "deterministic_v3" {
		t.Fatalf("unexpected version: %s", cfg.Version)
	}

	foundPenaltyControls := false
	for _, domain := range cfg.Domains {
		if len(domain.ContextPenaltyWeights) > 0 {
			foundPenaltyControls = true
		}
	}
	if !foundPenaltyControls {
		t.Fatal("expected v3 to define ambiguity or trap penalties")
	}
}

func TestRelevanceConfigRejectsImpossibleMinimumGroupMatches(t *testing.T) {
	cfg := RelevanceConfig{
		Version: "test_v1",
		Tiers: RelevanceTiers{A: 0.8, B: 0.6, C: 0.4},
		Domains: []RelevanceDomain{
			{
				Name:                "app_opportunity",
				MinimumGroupMatches: 2,
				GroupWeights: map[string]float64{
					"product_and_tool_language": 0.25,
				},
			},
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for impossible minimum group matches")
	}
}

func TestLoadRelevanceV4DefinesProximityRules(t *testing.T) {
	cfg, err := LoadRelevanceConfig("../../configs/relevance/deterministic-v4.yaml")
	if err != nil {
		t.Fatalf("load relevance v4 config: %v", err)
	}
	if cfg.Version != "deterministic_v4" {
		t.Fatalf("unexpected version: %s", cfg.Version)
	}

	totalRules := 0
	for _, domain := range cfg.Domains {
		for _, rule := range domain.ProximityRules {
			totalRules++
			if rule.Name == "" {
				t.Fatalf("domain %q has proximity rule without name", domain.Name)
			}
			if len(rule.Anchors) == 0 || len(rule.Evidence) == 0 {
				t.Fatalf("proximity rule %q in domain %q must define anchors and evidence", rule.Name, domain.Name)
			}
			if rule.WindowTokens <= 0 || rule.WindowTokens > 50 {
				t.Fatalf("proximity rule %q window_tokens out of range: %d", rule.Name, rule.WindowTokens)
			}
			if rule.Weight <= 0 || rule.Weight > 1 {
				t.Fatalf("proximity rule %q weight out of range: %f", rule.Name, rule.Weight)
			}
		}
	}
	if totalRules == 0 {
		t.Fatal("expected v4 to define at least one proximity rule")
	}
}

func TestRelevanceConfigRejectsInvalidProximityRules(t *testing.T) {
	baseDomain := RelevanceDomain{
		Name: "travel",
		GroupWeights: map[string]float64{
			"travel_language": 0.35,
		},
	}
	validRule := ProximityRule{
		Name:         "travel_safety",
		Anchors:      []string{"hostel"},
		Evidence:     []string{"stolen"},
		WindowTokens: 12,
		Weight:       0.20,
	}

	cases := []struct {
		name   string
		mutate func(rule ProximityRule) ProximityRule
	}{
		{"missing name", func(r ProximityRule) ProximityRule { r.Name = ""; return r }},
		{"empty anchors", func(r ProximityRule) ProximityRule { r.Anchors = []string{}; return r }},
		{"empty evidence", func(r ProximityRule) ProximityRule { r.Evidence = []string{}; return r }},
		{"zero window", func(r ProximityRule) ProximityRule { r.WindowTokens = 0; return r }},
		{"window over max", func(r ProximityRule) ProximityRule { r.WindowTokens = 51; return r }},
		{"zero weight", func(r ProximityRule) ProximityRule { r.Weight = 0; return r }},
		{"weight over one", func(r ProximityRule) ProximityRule { r.Weight = 1.5; return r }},
		{"empty anchor term", func(r ProximityRule) ProximityRule { r.Anchors = []string{"hostel", "  "}; return r }},
		{"duplicate anchor term", func(r ProximityRule) ProximityRule { r.Anchors = []string{"hostel", "hostel"}; return r }},
		{"duplicate evidence term", func(r ProximityRule) ProximityRule { r.Evidence = []string{"stolen", "stolen"}; return r }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			domain := baseDomain
			domain.ProximityRules = []ProximityRule{tc.mutate(validRule)}
			cfg := RelevanceConfig{
				Version: "test_v4",
				Tiers:   RelevanceTiers{A: 0.8, B: 0.6, C: 0.4},
				Domains: []RelevanceDomain{domain},
			}
			if err := cfg.Validate(); err == nil {
				t.Fatalf("expected validation error for %s", tc.name)
			}
		})
	}
}

func TestRelevanceConfigRejectsDuplicateProximityRuleNames(t *testing.T) {
	domain := RelevanceDomain{
		Name: "travel",
		GroupWeights: map[string]float64{
			"travel_language": 0.35,
		},
		ProximityRules: []ProximityRule{
			{Name: "duplicate_rule", Anchors: []string{"hostel"}, Evidence: []string{"stolen"}, WindowTokens: 12, Weight: 0.10},
			{Name: "duplicate_rule", Anchors: []string{"airline"}, Evidence: []string{"lost"}, WindowTokens: 12, Weight: 0.10},
		},
	}
	cfg := RelevanceConfig{
		Version: "test_v4",
		Tiers:   RelevanceTiers{A: 0.8, B: 0.6, C: 0.4},
		Domains: []RelevanceDomain{domain},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for duplicate proximity rule names")
	}
}
