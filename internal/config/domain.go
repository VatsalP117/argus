package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type DomainConfig struct {
	Name             string   `yaml:"name"`
	Description      string   `yaml:"description"`
	Subreddits       []string `yaml:"subreddits"`
	SeedKeywords     []string `yaml:"seed_keywords"`
	PainPointPhrases []string `yaml:"pain_point_phrases"`
	RequestPhrases   []string `yaml:"request_phrases"`
	ProductTerms     []string `yaml:"product_terms"`
	AirlineTerms     []string `yaml:"airline_terms"`
	BookingPlatforms []string `yaml:"booking_platform_terms"`
	PaymentToolTerms []string `yaml:"payment_tool_terms"`
}

func LoadDomainConfig(path string) (DomainConfig, error) {
	var cfg DomainConfig

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}
