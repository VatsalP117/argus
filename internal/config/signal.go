package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type SignalConfig struct {
	Version                       string   `yaml:"version"`
	SignalTypes                   []string `yaml:"signal_types"`
	PainPointPatterns             []string `yaml:"pain_point_patterns"`
	FeatureRequestPatterns        []string `yaml:"feature_request_patterns"`
	RecommendationRequestPatterns []string `yaml:"recommendation_request_patterns"`
	WorkaroundPatterns            []string `yaml:"workaround_patterns"`
	ComparisonPatterns            []string `yaml:"comparison_patterns"`
	EntityCategories              []string `yaml:"entity_categories"`
}

func LoadSignalConfig(path string) (SignalConfig, error) {
	var cfg SignalConfig

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}
