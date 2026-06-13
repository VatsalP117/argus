package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"argus/internal/archive"
	"argus/internal/config"
)

type Entry struct {
	EntryID         string `json:"entry_id"`
	RecordType      string `json:"record_type"`
	Month           string `json:"month"`
	Year            string `json:"year"`
	MonthPart       string `json:"month_part"`
	ShardPath       string `json:"shard_path"`
	ShardName       string `json:"shard_name"`
	HFURI           string `json:"hf_uri"`
	ResolveURL      string `json:"resolve_url"`
	SizeBytes       int64  `json:"size_bytes"`
	ShardIndex      int    `json:"shard_index"`
	ArchiveRevision string `json:"archive_revision"`
	SourceOID       string `json:"source_oid,omitempty"`
	SourceIdentity  string `json:"source_identity"`
	ProcessingState string `json:"processing_state"`
}

type Summary struct {
	EntryCount int            `json:"entry_count"`
	BytesTotal int64          `json:"bytes_total"`
	ByType     map[string]int `json:"by_type"`
	ByMonth    map[string]int `json:"by_month"`
}

type Manifest struct {
	ManifestID      string                 `json:"manifest_id"`
	GeneratedAt     string                 `json:"generated_at"`
	DatasetRepo     string                 `json:"dataset_repo"`
	ArchiveRevision string                 `json:"archive_revision"`
	PipelineName    string                 `json:"pipeline_name"`
	PhaseTarget     string                 `json:"phase_target"`
	Filters         map[string]interface{} `json:"filters"`
	Summary         Summary                `json:"summary"`
	Entries         []Entry                `json:"entries"`
}

const ProcessingPending = "pending"

func Build(repo, archiveRevision string, cfg config.PipelineConfig, months []string, recordTypes []string, shardsByGroup map[string][]archive.TreeFile) Manifest {
	m := Manifest{
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		DatasetRepo:     repo,
		ArchiveRevision: archiveRevision,
		PipelineName:    cfg.PipelineName,
		PhaseTarget:     cfg.PhaseTarget,
		Filters: map[string]interface{}{
			"months":       months,
			"record_types": recordTypes,
			"subreddits":   cfg.Subreddits,
			"domain":       cfg.Domain,
		},
		Summary: Summary{
			ByType:  map[string]int{},
			ByMonth: map[string]int{},
		},
	}

	for _, recordType := range recordTypes {
		for _, month := range months {
			key := recordType + ":" + month
			shards := shardsByGroup[key]
			parts := strings.Split(month, "-")
			for idx, shard := range shards {
				sourceIdentity := deterministicSourceIdentity(repo, archiveRevision, shard)
				entry := Entry{
					EntryID:         recordType + "-" + month + "-" + strings.TrimSuffix(archive.ShardName(shard.Path), ".parquet"),
					RecordType:      recordType,
					Month:           month,
					Year:            parts[0],
					MonthPart:       parts[1],
					ShardPath:       shard.Path,
					ShardName:       archive.ShardName(shard.Path),
					HFURI:           "hf://datasets/" + repo + "@" + archiveRevision + "/" + shard.Path,
					ResolveURL:      archive.ResolveURLAtRevision(repo, archiveRevision, shard.Path),
					SizeBytes:       shard.Size,
					ShardIndex:      idx,
					ArchiveRevision: archiveRevision,
					SourceOID:       shard.OID,
					SourceIdentity:  sourceIdentity,
					ProcessingState: ProcessingPending,
				}
				m.Entries = append(m.Entries, entry)
				m.Summary.EntryCount++
				m.Summary.BytesTotal += shard.Size
				m.Summary.ByType[recordType]++
				m.Summary.ByMonth[month]++
			}
		}
	}

	sort.Slice(m.Entries, func(i, j int) bool {
		if m.Entries[i].RecordType != m.Entries[j].RecordType {
			return m.Entries[i].RecordType < m.Entries[j].RecordType
		}
		if m.Entries[i].Month != m.Entries[j].Month {
			return m.Entries[i].Month < m.Entries[j].Month
		}
		return m.Entries[i].ShardPath < m.Entries[j].ShardPath
	})

	m.ManifestID = deterministicManifestID(cfg.PipelineName, repo, archiveRevision, months, recordTypes, cfg.Subreddits, m.Entries)
	return m
}

func deterministicSourceIdentity(repo, archiveRevision string, shard archive.TreeFile) string {
	h := sha256.New()
	writeHashPart(h, repo)
	writeHashPart(h, archiveRevision)
	writeHashPart(h, shard.Path)
	writeHashPart(h, strconv.FormatInt(shard.Size, 10))
	writeHashPart(h, shard.OID)
	return "hf-shard-sha256:" + hex.EncodeToString(h.Sum(nil))
}

func deterministicManifestID(pipelineName, repo, archiveRevision string, months, recordTypes, subreddits []string, entries []Entry) string {
	h := sha256.New()
	writeHashPart(h, pipelineName)
	writeHashPart(h, repo)
	writeHashPart(h, archiveRevision)
	for _, item := range months {
		writeHashPart(h, item)
	}
	for _, item := range recordTypes {
		writeHashPart(h, item)
	}
	for _, item := range subreddits {
		writeHashPart(h, strings.ToLower(item))
	}
	for _, entry := range entries {
		writeHashPart(h, entry.EntryID)
		writeHashPart(h, entry.SourceIdentity)
	}
	return pipelineName + "-" + hex.EncodeToString(h.Sum(nil))[:16]
}

func writeHashPart(h interface{ Write([]byte) (int, error) }, value string) {
	_, _ = h.Write([]byte(value))
	_, _ = h.Write([]byte{0})
}

func Write(path string, m Manifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

func Load(path string) (Manifest, error) {
	var m Manifest

	data, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}

	if err := json.Unmarshal(data, &m); err != nil {
		return m, err
	}

	return m, nil
}
