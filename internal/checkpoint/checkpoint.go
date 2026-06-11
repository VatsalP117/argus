package checkpoint

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type ShardCheckpoint struct {
	JobName      string `json:"job_name"`
	RunID        string `json:"run_id"`
	ManifestID   string `json:"manifest_id"`
	EntryID      string `json:"entry_id"`
	SourcePath   string `json:"source_path"`
	OutputPath   string `json:"output_path"`
	StartedAt    string `json:"started_at"`
	FinishedAt   string `json:"finished_at"`
	Status       string `json:"status"`
	AttemptCount int    `json:"attempt_count"`
	RowsWritten  int64  `json:"rows_written"`
	BytesWritten int64  `json:"bytes_written"`
	Error        string `json:"error,omitempty"`
}

func Write(path string, cp ShardCheckpoint) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}
