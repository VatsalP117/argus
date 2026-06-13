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

	first := Build("open-index/arctic", "revision-123", cfg, cfg.Months, cfg.RecordTypes, shardsByGroup)
	second := Build("open-index/arctic", "revision-123", cfg, cfg.Months, cfg.RecordTypes, shardsByGroup)

	if first.ManifestID != second.ManifestID {
		t.Fatalf("expected deterministic manifest id, got %s vs %s", first.ManifestID, second.ManifestID)
	}
}

func TestBuildPinsSourceIdentityAndInitialProcessingState(t *testing.T) {
	cfg := config.PipelineConfig{
		PipelineName: "pilot",
		PhaseTarget:  "phase2",
		Domain:       "travel",
		RecordTypes:  []string{"comments"},
		Months:       []string{"2021-01"},
	}
	shardsByGroup := map[string][]archive.TreeFile{
		"comments:2021-01": {
			{
				Type: "file",
				Path: "data/comments/2021/01/000.parquet",
				Size: 10,
				OID:  "source-object-abc",
			},
		},
	}

	built := Build("open-index/arctic", "revision-123", cfg, cfg.Months, cfg.RecordTypes, shardsByGroup)
	if built.ArchiveRevision != "revision-123" {
		t.Fatalf("unexpected archive revision: %s", built.ArchiveRevision)
	}
	if len(built.Entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(built.Entries))
	}

	entry := built.Entries[0]
	if entry.ArchiveRevision != "revision-123" {
		t.Fatalf("unexpected entry archive revision: %s", entry.ArchiveRevision)
	}
	if entry.SourceOID != "source-object-abc" {
		t.Fatalf("unexpected source oid: %s", entry.SourceOID)
	}
	if entry.SourceIdentity == "" {
		t.Fatal("expected deterministic source identity")
	}
	if entry.ProcessingState != ProcessingPending {
		t.Fatalf("expected pending state, got %s", entry.ProcessingState)
	}
	if entry.ResolveURL != "https://huggingface.co/datasets/open-index/arctic/resolve/revision-123/data/comments/2021/01/000.parquet" {
		t.Fatalf("unexpected pinned resolve url: %s", entry.ResolveURL)
	}
}
