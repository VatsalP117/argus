package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type CandidateRuleGroup struct {
	Name  string   `yaml:"name" json:"name"`
	Terms []string `yaml:"terms" json:"terms"`
}

type CandidateConfig struct {
	Version           string               `yaml:"version" json:"version"`
	MinimumTextLength int                  `yaml:"minimum_text_length" json:"minimum_text_length"`
	ExcludedExactText []string             `yaml:"excluded_exact_text" json:"excluded_exact_text"`
	SubredditPriors   []string             `yaml:"subreddit_priors" json:"subreddit_priors"`
	RuleGroups        []CandidateRuleGroup `yaml:"rule_groups" json:"rule_groups"`
}

func LoadCandidateConfig(path string) (CandidateConfig, error) {
	var cfg CandidateConfig

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

func (cfg CandidateConfig) Validate() error {
	if strings.TrimSpace(cfg.Version) == "" {
		return fmt.Errorf("candidate config version is required")
	}
	if cfg.MinimumTextLength <= 0 {
		return fmt.Errorf("minimum_text_length must be positive")
	}
	if len(cfg.RuleGroups) == 0 {
		return fmt.Errorf("at least one candidate rule group is required")
	}

	groupNames := map[string]struct{}{}
	for _, group := range cfg.RuleGroups {
		name := strings.TrimSpace(group.Name)
		if name == "" {
			return fmt.Errorf("candidate rule group name is required")
		}
		if _, exists := groupNames[name]; exists {
			return fmt.Errorf("candidate rule group %q is duplicated", name)
		}
		groupNames[name] = struct{}{}
		if len(group.Terms) == 0 {
			return fmt.Errorf("candidate rule group %q must contain terms", name)
		}

		terms := map[string]struct{}{}
		for _, term := range group.Terms {
			normalized := strings.ToLower(strings.TrimSpace(term))
			if normalized == "" {
				return fmt.Errorf("candidate rule group %q contains an empty term", name)
			}
			if _, exists := terms[normalized]; exists {
				return fmt.Errorf("candidate rule group %q contains duplicate term %q", name, term)
			}
			terms[normalized] = struct{}{}
		}
	}
	return nil
}
