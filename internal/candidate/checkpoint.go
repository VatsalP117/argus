package candidate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"argus/internal/manifest"
)

type ScanCheckpoint struct {
	Status              string     `json:"status"`
	ManifestID          string     `json:"manifest_id"`
	EntryID             string     `json:"entry_id"`
	SourceIdentity      string     `json:"source_identity"`
	CandidateVersion    string     `json:"candidate_version"`
	CandidateConfigHash string     `json:"candidate_config_hash"`
	OutputPath          string     `json:"output_path"`
	OutputSHA256        string     `json:"output_sha256,omitempty"`
	StartedAt           string     `json:"started_at"`
	FinishedAt          string     `json:"finished_at"`
	Result              ScanResult `json:"result"`
}

func LoadScanCheckpoint(path string) (ScanCheckpoint, error) {
	var checkpoint ScanCheckpoint
	data, err := os.ReadFile(path)
	if err != nil {
		return checkpoint, err
	}
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return checkpoint, err
	}
	return checkpoint, nil
}

func WriteScanCheckpoint(path string, checkpoint ScanCheckpoint) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return err
	}

	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func NewScanCheckpoint(
	manifestID string,
	entry manifest.Entry,
	candidateVersion string,
	configHash string,
	startedAt time.Time,
	result ScanResult,
) (ScanCheckpoint, error) {
	outputChecksum := ""
	if result.OutputPath != "" {
		var err error
		outputChecksum, err = FileSHA256(result.OutputPath)
		if err != nil {
			return ScanCheckpoint{}, fmt.Errorf("checksum candidate output: %w", err)
		}
	}
	return ScanCheckpoint{
		Status:              "completed",
		ManifestID:          manifestID,
		EntryID:             entry.EntryID,
		SourceIdentity:      entry.SourceIdentity,
		CandidateVersion:    candidateVersion,
		CandidateConfigHash: configHash,
		OutputPath:          result.OutputPath,
		OutputSHA256:        outputChecksum,
		StartedAt:           startedAt.UTC().Format(time.RFC3339),
		FinishedAt:          time.Now().UTC().Format(time.RFC3339),
		Result:              result,
	}, nil
}

func (checkpoint ScanCheckpoint) Reusable(
	manifestID string,
	entry manifest.Entry,
	candidateVersion string,
	configHash string,
) (bool, error) {
	if checkpoint.Status != "completed" ||
		checkpoint.ManifestID != manifestID ||
		checkpoint.EntryID != entry.EntryID ||
		checkpoint.SourceIdentity != entry.SourceIdentity ||
		checkpoint.CandidateVersion != candidateVersion ||
		checkpoint.CandidateConfigHash != configHash {
		return false, nil
	}

	if checkpoint.Result.RowsCandidates == 0 {
		return checkpoint.OutputPath == "" && checkpoint.OutputSHA256 == "", nil
	}
	if checkpoint.OutputPath == "" || checkpoint.OutputSHA256 == "" {
		return false, nil
	}
	checksum, err := FileSHA256(checkpoint.OutputPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return checksum == checkpoint.OutputSHA256, nil
}

func FileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}
