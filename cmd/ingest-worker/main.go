package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"argus/internal/checkpoint"
	"argus/internal/config"
	"argus/internal/manifest"
	"argus/internal/runmeta"
)

type ingestResult struct {
	Status       string `json:"status"`
	RowsWritten  int64  `json:"rows_written"`
	BytesWritten int64  `json:"bytes_written"`
	OutputPath   string `json:"output_path"`
	SourcePath   string `json:"source_path"`
	Error        string `json:"error,omitempty"`
}

type ingestGroup struct {
	GroupID    string
	RecordType string
	Month      string
	Year       string
	MonthPart  string
	Entries    []manifest.Entry
}

func main() {
	var pipelinePath string
	var manifestPath string
	var month string
	var recordType string
	var limitShards int
	var force bool
	var groupByMonth bool

	flag.StringVar(&pipelinePath, "pipeline", "configs/pipelines/pilot-travel-q1-2021.yaml", "path to pipeline config")
	flag.StringVar(&manifestPath, "manifest", "", "path to manifest json")
	flag.StringVar(&month, "month", "", "optional month filter, format YYYY-MM")
	flag.StringVar(&recordType, "record-type", "", "optional record type filter")
	flag.IntVar(&limitShards, "limit-shards", 0, "optional max shards to process")
	flag.BoolVar(&force, "force", false, "reprocess shards even if output already exists")
	flag.BoolVar(&groupByMonth, "group-by-month", false, "process all selected shards for a month and record type into one output file")
	flag.Parse()

	if manifestPath == "" {
		panic("manifest path is required")
	}

	cfg, err := config.LoadPipelineConfig(pipelinePath)
	if err != nil {
		panic(err)
	}

	man, err := manifest.Load(manifestPath)
	if err != nil {
		panic(err)
	}

	runID := "phase2-ingest-" + time.Now().UTC().Format("20060102T150405Z")
	startedAt := time.Now().UTC()
	selected := selectEntries(man.Entries, month, recordType, limitShards)

	var processed int
	var totalRows int64
	var errorCount int
	var warnings []string
	var outputRefs []string

	if groupByMonth {
		for _, group := range groupEntriesByMonth(selected) {
			res, err := runGroup(cfg, runID, man.ManifestID, group, force)
			processed++
			if err != nil {
				errorCount++
				warnings = append(warnings, err.Error())
				if !cfg.Quality.QuarantineBadShards {
					panic(err)
				}
				continue
			}
			totalRows += res.RowsWritten
			if res.OutputPath != "" {
				outputRefs = append(outputRefs, res.OutputPath)
			}
		}
	} else {
		for _, entry := range selected {
			res, err := runSingleEntry(cfg, runID, man.ManifestID, entry, force)
			processed++
			if err != nil {
				errorCount++
				warnings = append(warnings, err.Error())
				if !cfg.Quality.QuarantineBadShards {
					panic(err)
				}
				continue
			}
			totalRows += res.RowsWritten
			if res.OutputPath != "" {
				outputRefs = append(outputRefs, res.OutputPath)
			}
		}
	}

	rec := runmeta.RunRecord{
		RunID:          runID,
		Phase:          "phase2",
		JobName:        "ingest_worker",
		StartedAt:      startedAt.Format(time.RFC3339),
		FinishedAt:     time.Now().UTC().Format(time.RFC3339),
		Status:         "completed",
		GitSHA:         gitSHA(),
		ConfigHash:     configHash(pipelinePath),
		RecordsSeen:    int64(processed),
		RecordsWritten: totalRows,
		ErrorCount:     errorCount,
		Warnings:       warnings,
		InputRefs:      []string{pipelinePath, manifestPath},
		OutputRefs:     outputRefs,
		Notes:          fmt.Sprintf("Processed %d work units", processed),
	}
	if errorCount > 0 {
		rec.Status = "partial"
	}

	runPath := filepath.Join(cfg.State.RunsDir, "phase2", runID+".json")
	if err := runmeta.Write(runPath, rec); err != nil {
		panic(err)
	}

	fmt.Printf("run complete: %s\n", runID)
	fmt.Printf("work_units_processed: %d\n", processed)
	fmt.Printf("rows_written: %d\n", totalRows)
	fmt.Printf("errors: %d\n", errorCount)
}

func selectEntries(entries []manifest.Entry, month, recordType string, limitShards int) []manifest.Entry {
	selected := make([]manifest.Entry, 0, len(entries))
	for _, entry := range entries {
		if month != "" && entry.Month != month {
			continue
		}
		if recordType != "" && entry.RecordType != recordType {
			continue
		}
		selected = append(selected, entry)
		if limitShards > 0 && len(selected) >= limitShards {
			break
		}
	}
	return selected
}

func groupEntriesByMonth(entries []manifest.Entry) []ingestGroup {
	byKey := map[string]*ingestGroup{}
	for _, entry := range entries {
		key := entry.RecordType + ":" + entry.Month
		group, ok := byKey[key]
		if !ok {
			group = &ingestGroup{
				GroupID:    key,
				RecordType: entry.RecordType,
				Month:      entry.Month,
				Year:       entry.Year,
				MonthPart:  entry.MonthPart,
			}
			byKey[key] = group
		}
		group.Entries = append(group.Entries, entry)
	}

	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	groups := make([]ingestGroup, 0, len(keys))
	for _, key := range keys {
		groups = append(groups, *byKey[key])
	}
	return groups
}

func runSingleEntry(cfg config.PipelineConfig, runID, manifestID string, entry manifest.Entry, force bool) (ingestResult, error) {
	outputPath := filepath.Join(
		cfg.Output.RawDir,
		entry.RecordType,
		"year="+entry.Year,
		"month="+entry.MonthPart,
		strings.TrimSuffix(entry.ShardName, ".parquet")+"-filtered.parquet",
	)

	cpPath := filepath.Join(cfg.State.CheckpointsDir, "phase2", runID, entry.EntryID+".json")
	cp := checkpoint.ShardCheckpoint{
		JobName:      "ingest_worker",
		RunID:        runID,
		ManifestID:   manifestID,
		EntryID:      entry.EntryID,
		SourcePath:   entry.ShardPath,
		OutputPath:   outputPath,
		StartedAt:    time.Now().UTC().Format(time.RFC3339),
		Status:       "started",
		AttemptCount: 1,
	}

	if !force {
		if _, err := os.Stat(outputPath); err == nil {
			cp.Status = "skipped_existing"
			cp.FinishedAt = time.Now().UTC().Format(time.RFC3339)
			if err := checkpoint.Write(cpPath, cp); err != nil {
				return ingestResult{}, err
			}
			return ingestResult{Status: "skipped_existing", OutputPath: outputPath}, nil
		}
	}

	res, err := runDuckDBCopy([]string{entry.ResolveURL}, cfg.Subreddits, manifestID, entry.ShardPath, entry.RecordType, outputPath)
	if err != nil {
		cp.Status = "failed"
		cp.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		cp.Error = err.Error()
		if writeErr := checkpoint.Write(cpPath, cp); writeErr != nil {
			return ingestResult{}, writeErr
		}
		return ingestResult{}, err
	}

	cp.Status = res.Status
	cp.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	cp.RowsWritten = res.RowsWritten
	cp.BytesWritten = res.BytesWritten
	cp.OutputPath = res.OutputPath
	if err := checkpoint.Write(cpPath, cp); err != nil {
		return ingestResult{}, err
	}

	return res, nil
}

func runGroup(cfg config.PipelineConfig, runID, manifestID string, group ingestGroup, force bool) (ingestResult, error) {
	outputPath := filepath.Join(
		cfg.Output.RawDir,
		group.RecordType,
		"year="+group.Year,
		"month="+group.MonthPart,
		fmt.Sprintf("%s-%s-filtered.parquet", group.RecordType, group.Month),
	)

	cpPath := filepath.Join(cfg.State.CheckpointsDir, "phase2", runID, group.RecordType+"-"+group.Month+".json")
	cp := checkpoint.ShardCheckpoint{
		JobName:      "ingest_worker_grouped",
		RunID:        runID,
		ManifestID:   manifestID,
		EntryID:      group.RecordType + "-" + group.Month,
		SourcePath:   group.GroupID,
		OutputPath:   outputPath,
		StartedAt:    time.Now().UTC().Format(time.RFC3339),
		Status:       "started",
		AttemptCount: 1,
	}

	if !force {
		if _, err := os.Stat(outputPath); err == nil {
			cp.Status = "skipped_existing"
			cp.FinishedAt = time.Now().UTC().Format(time.RFC3339)
			if err := checkpoint.Write(cpPath, cp); err != nil {
				return ingestResult{}, err
			}
			return ingestResult{Status: "skipped_existing", OutputPath: outputPath}, nil
		}
	}

	glob := fmt.Sprintf(
		"hf://datasets/%s/data/%s/%s/%s/*.parquet",
		cfg.Archive.Repo,
		group.RecordType,
		group.Year,
		group.MonthPart,
	)

	res, err := runDuckDBCopy([]string{glob}, cfg.Subreddits, manifestID, group.GroupID, group.RecordType, outputPath)
	if err != nil {
		cp.Status = "failed"
		cp.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		cp.Error = err.Error()
		if writeErr := checkpoint.Write(cpPath, cp); writeErr != nil {
			return ingestResult{}, writeErr
		}
		return ingestResult{}, err
	}

	cp.Status = res.Status
	cp.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	cp.RowsWritten = res.RowsWritten
	cp.BytesWritten = res.BytesWritten
	cp.OutputPath = res.OutputPath
	if err := checkpoint.Write(cpPath, cp); err != nil {
		return ingestResult{}, err
	}

	return res, nil
}

func runDuckDBCopy(urls []string, subreddits []string, manifestID, sourcePath, recordType, outputPath string) (ingestResult, error) {
	args := []string{
		"scripts/dev/duckdb_filter_copy.py",
		"--output-path", outputPath,
		"--record-type", recordType,
		"--manifest-id", manifestID,
		"--source-path", sourcePath,
		"--subreddits", strings.Join(subreddits, ","),
	}
	for _, item := range urls {
		args = append(args, "--input-url", item)
	}

	cmd := exec.Command("python3", args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ingestResult{}, fmt.Errorf("duckdb copy failed for %s: %w: %s", sourcePath, err, string(output))
	}

	var res ingestResult
	if err := json.Unmarshal(output, &res); err != nil {
		return ingestResult{}, fmt.Errorf("failed to parse duckdb result for %s: %w: %s", sourcePath, err, string(output))
	}
	if res.Status == "error" {
		return res, fmt.Errorf(res.Error)
	}
	return res, nil
}

func gitSHA() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return string(bytesTrimSpace(out))
}

func configHash(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "unavailable"
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func bytesTrimSpace(in []byte) []byte {
	start := 0
	end := len(in)
	for start < end && (in[start] == ' ' || in[start] == '\n' || in[start] == '\r' || in[start] == '\t') {
		start++
	}
	for end > start && (in[end-1] == ' ' || in[end-1] == '\n' || in[end-1] == '\r' || in[end-1] == '\t') {
		end--
	}
	return in[start:end]
}
