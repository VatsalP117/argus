package manifest

import (
	"testing"

	"argus/internal/archive"
	"argus/internal/config"
)

func TestBuildProducesDeterministicManifestID(t *testing.T) {
	cfg := config.PipelineConfig{
		PipelineName: "pilot",
		PhaseTarget:  "phase2",
		Domain:       "travel",
		RecordTypes:  []string{"comments"},
		Months:       []string{"2021-01"},
		Subreddits:   []string{"travel", "onebag"},
	}

	shardsByGroup := map[string][]archive.TreeFile{
		"comments:2021-01": {
			{Type: "file", Path: "data/comments/2021/01/001.parquet", Size: 11},
			{Type: "file", Path: "data/comments/2021/01/000.parquet", Size: 10},
		},
	}

	first := Build("open-index/arctic", cfg, cfg.Months, cfg.RecordTypes, shardsByGroup)
	second := Build("open-index/arctic", cfg, cfg.Months, cfg.RecordTypes, shardsByGroup)

	if first.ManifestID != second.ManifestID {
		t.Fatalf("expected deterministic manifest id, got %s vs %s", first.ManifestID, second.ManifestID)
	}
}
