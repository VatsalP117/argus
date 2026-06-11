package runmeta

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type RunRecord struct {
	RunID          string   `json:"run_id"`
	Phase          string   `json:"phase"`
	JobName        string   `json:"job_name"`
	StartedAt      string   `json:"started_at"`
	FinishedAt     string   `json:"finished_at"`
	Status         string   `json:"status"`
	GitSHA         string   `json:"git_sha"`
	ConfigHash     string   `json:"config_hash"`
	RecordsSeen    int64    `json:"records_seen"`
	RecordsWritten int64    `json:"records_written"`
	ErrorCount     int      `json:"error_count"`
	Warnings       []string `json:"warnings"`
	InputRefs      []string `json:"input_refs"`
	OutputRefs     []string `json:"output_refs"`
	Notes          string   `json:"notes"`
}

func Write(path string, rec RunRecord) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}
