package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type RelevanceTiers struct {
	A float64 `yaml:"a" json:"a"`
	B float64 `yaml:"b" json:"b"`
	C float64 `yaml:"c" json:"c"`
}

type RelevanceDomain struct {
	Name                 string             `yaml:"name" json:"name"`
	GroupWeights         map[string]float64 `yaml:"group_weights" json:"group_weights"`
	RequiredAnyTerms     []string           `yaml:"required_any_terms" json:"required_any_terms"`
	SubredditPriorWeight float64            `yaml:"subreddit_prior_weight" json:"subreddit_prior_weight"`
}

type SignalMapping struct {
	RuleGroup  string  `yaml:"rule_group" json:"rule_group"`
	SignalType string  `yaml:"signal_type" json:"signal_type"`
	Score      float64 `yaml:"score" json:"score"`
}

type RelevanceConfig struct {
	Version        string            `yaml:"version" json:"version"`
	Tiers          RelevanceTiers    `yaml:"tiers" json:"tiers"`
	Domains        []RelevanceDomain `yaml:"domains" json:"domains"`
	SignalMappings []SignalMapping   `yaml:"signal_mappings" json:"signal_mappings"`
}

func LoadRelevanceConfig(path string) (RelevanceConfig, error) {
	var cfg RelevanceConfig

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (cfg RelevanceConfig) Validate() error {
	if strings.TrimSpace(cfg.Version) == "" {
		return fmt.Errorf("relevance config version is required")
	}
	if !(1 >= cfg.Tiers.A && cfg.Tiers.A > cfg.Tiers.B &&
		cfg.Tiers.B > cfg.Tiers.C && cfg.Tiers.C > 0) {
		return fmt.Errorf("relevance tiers must descend from A through C within (0, 1]")
	}
	if len(cfg.Domains) == 0 {
		return fmt.Errorf("at least one relevance domain is required")
	}

	domains := map[string]struct{}{}
	for _, domain := range cfg.Domains {
		name := strings.TrimSpace(domain.Name)
		if name == "" {
			return fmt.Errorf("relevance domain name is required")
		}
		if _, exists := domains[name]; exists {
			return fmt.Errorf("relevance domain %q is duplicated", name)
		}
		domains[name] = struct{}{}
		if len(domain.GroupWeights) == 0 {
			return fmt.Errorf("relevance domain %q must define group weights", name)
		}
		if domain.SubredditPriorWeight < 0 || domain.SubredditPriorWeight > 1 {
			return fmt.Errorf("relevance domain %q subreddit prior weight must be within [0, 1]", name)
		}
		for group, weight := range domain.GroupWeights {
			if strings.TrimSpace(group) == "" {
				return fmt.Errorf("relevance domain %q contains an empty group name", name)
			}
			if weight <= 0 || weight > 1 {
				return fmt.Errorf("relevance domain %q group %q weight must be within (0, 1]", name, group)
			}
		}
		requiredTerms := map[string]struct{}{}
		for _, term := range domain.RequiredAnyTerms {
			normalized := strings.ToLower(strings.TrimSpace(term))
			if normalized == "" {
				return fmt.Errorf("relevance domain %q contains an empty required term", name)
			}
			if _, exists := requiredTerms[normalized]; exists {
				return fmt.Errorf("relevance domain %q contains duplicate required term %q", name, term)
			}
			requiredTerms[normalized] = struct{}{}
		}
	}

	mappings := map[string]struct{}{}
	for _, mapping := range cfg.SignalMappings {
		if strings.TrimSpace(mapping.RuleGroup) == "" || strings.TrimSpace(mapping.SignalType) == "" {
			return fmt.Errorf("signal mappings require rule_group and signal_type")
		}
		if _, exists := mappings[mapping.RuleGroup]; exists {
			return fmt.Errorf("signal mapping for rule group %q is duplicated", mapping.RuleGroup)
		}
		mappings[mapping.RuleGroup] = struct{}{}
		if mapping.Score <= 0 || mapping.Score > 1 {
			return fmt.Errorf("signal mapping %q score must be within (0, 1]", mapping.RuleGroup)
		}
	}
	return nil
}
