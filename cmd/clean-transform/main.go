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
	"strings"
	"time"

	"argus/internal/checkpoint"
	"argus/internal/config"
	"argus/internal/runmeta"
)

type cleanResult struct {
	Status       string `json:"status"`
	RowsWritten  int64  `json:"rows_written"`
	BytesWritten int64  `json:"bytes_written"`
	OutputPath   string `json:"output_path"`
	SourcePath   string `json:"source_path"`
	Error        string `json:"error,omitempty"`
}

type duckDBOptions struct {
	MemoryLimit string
	Threads     int
	TempDir     string
}

func main() {
	var pipelinePath string
	var month string
	var recordType string
	var force bool
	var duckDBMemoryLimit string
	var duckDBThreads int
	var duckDBTempDir string

	flag.StringVar(&pipelinePath, "pipeline", "configs/pipelines/pilot-travel-q1-2021.yaml", "path to pipeline config")
	flag.StringVar(&month, "month", "", "optional month filter, format YYYY-MM")
	flag.StringVar(&recordType, "record-type", "", "optional record type filter")
	flag.BoolVar(&force, "force", false, "reprocess clean outputs even if they already exist")
	flag.StringVar(&duckDBMemoryLimit, "duckdb-memory-limit", "4GB", "DuckDB memory limit for clean-transform jobs")
	flag.IntVar(&duckDBThreads, "duckdb-threads", 4, "DuckDB threads for clean-transform jobs")
	flag.StringVar(&duckDBTempDir, "duckdb-temp-dir", filepath.Join(".duckdb", "tmp"), "DuckDB temp spill directory")
	flag.Parse()

	cfg, err := config.LoadPipelineConfig(pipelinePath)
	if err != nil {
		panic(err)
	}

	months := cfg.Months
	if month != "" {
		months = []string{month}
	}

	recordTypes := cfg.RecordTypes
	if recordType != "" {
		recordTypes = []string{recordType}
	}

	startedAt := time.Now().UTC()
	runID := "phase3-clean-" + startedAt.Format("20060102T150405.000000000Z")
	duckdbOpts := duckDBOptions{
		MemoryLimit: duckDBMemoryLimit,
		Threads:     duckDBThreads,
		TempDir:     duckDBTempDir,
	}

	var processed int
	var totalRows int64
	var errorCount int
	var warnings []string
	var outputRefs []string

	for _, rt := range recordTypes {
		for _, m := range months {
			res, err := runMonth(cfg, runID, rt, m, force, duckdbOpts)
			processed++
			if err != nil {
				errorCount++
				warnings = append(warnings, err.Error())
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
		Phase:          "phase3",
		JobName:        "clean_transform",
		StartedAt:      startedAt.Format(time.RFC3339),
		FinishedAt:     time.Now().UTC().Format(time.RFC3339),
		Status:         "completed",
		GitSHA:         gitSHA(),
		ConfigHash:     configHash(pipelinePath),
		RecordsSeen:    int64(processed),
		RecordsWritten: totalRows,
		ErrorCount:     errorCount,
		Warnings:       warnings,
		InputRefs:      []string{pipelinePath},
		OutputRefs:     outputRefs,
		Notes:          fmt.Sprintf("Processed %d clean work units", processed),
	}
	if errorCount > 0 {
		rec.Status = "partial"
	}

	runPath := filepath.Join(cfg.State.RunsDir, "phase3", runID+".json")
	if err := runmeta.Write(runPath, rec); err != nil {
		panic(err)
	}

	fmt.Printf("run complete: %s\n", runID)
	fmt.Printf("work_units_processed: %d\n", processed)
	fmt.Printf("rows_written: %d\n", totalRows)
	fmt.Printf("errors: %d\n", errorCount)
}

func runMonth(cfg config.PipelineConfig, runID, recordType, month string, force bool, duckdbOpts duckDBOptions) (cleanResult, error) {
	parts := splitMonth(month)
	inputGlob := filepath.Join(
		cfg.Output.RawDir,
		recordType,
		"year="+parts[0],
		"month="+parts[1],
		"*.parquet",
	)
	outputPath := filepath.Join(
		cfg.Output.CleanDir,
		recordType,
		"year="+parts[0],
		"month="+parts[1],
		fmt.Sprintf("%s-%s-clean.parquet", recordType, month),
	)
	cpPath := filepath.Join(cfg.State.CheckpointsDir, "phase3", runID, recordType+"-"+month+".json")
	cp := checkpoint.ShardCheckpoint{
		JobName:      "clean_transform",
		RunID:        runID,
		EntryID:      recordType + "-" + month,
		SourcePath:   inputGlob,
		OutputPath:   outputPath,
		StartedAt:    time.Now().UTC().Format(time.RFC3339),
		Status:       "started",
		AttemptCount: 1,
	}

	if !force {
		exists, err := existingUsableOutput(outputPath)
		if err != nil {
			return cleanResult{}, err
		}
		if exists {
			cp.Status = "skipped_existing"
			cp.FinishedAt = time.Now().UTC().Format(time.RFC3339)
			if err := checkpoint.Write(cpPath, cp); err != nil {
				return cleanResult{}, err
			}
			return cleanResult{Status: "skipped_existing", OutputPath: outputPath}, nil
		}
	}

	res, err := runDuckDBClean(inputGlob, outputPath, recordType, runID, duckdbOpts)
	if err != nil {
		cp.Status = "failed"
		cp.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		cp.Error = err.Error()
		if writeErr := checkpoint.Write(cpPath, cp); writeErr != nil {
			return cleanResult{}, writeErr
		}
		return cleanResult{}, err
	}

	cp.Status = res.Status
	cp.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	cp.RowsWritten = res.RowsWritten
	cp.BytesWritten = res.BytesWritten
	cp.OutputPath = res.OutputPath
	if err := checkpoint.Write(cpPath, cp); err != nil {
		return cleanResult{}, err
	}

	return res, nil
}

func splitMonth(month string) [2]string {
	items := strings.SplitN(month, "-", 2)
	if len(items) != 2 {
		panic(fmt.Sprintf("invalid month %q", month))
	}
	return [2]string{items[0], items[1]}
}

func existingUsableOutput(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if info.Size() > 0 {
		return true, nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return false, err
	}
	return false, nil
}

func runDuckDBClean(inputGlob, outputPath, recordType, cleanRunID string, duckdbOpts duckDBOptions) (cleanResult, error) {
	args := []string{
		"scripts/dev/duckdb_clean_transform.py",
		"--input-glob", inputGlob,
		"--output-path", outputPath,
		"--record-type", recordType,
		"--clean-run-id", cleanRunID,
		"--duckdb-memory-limit", duckdbOpts.MemoryLimit,
		"--duckdb-threads", fmt.Sprintf("%d", duckdbOpts.Threads),
		"--duckdb-temp-dir", duckdbOpts.TempDir,
	}

	cmd := exec.Command("python3", args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return cleanResult{}, fmt.Errorf("duckdb clean failed for %s: %w: %s", inputGlob, err, string(output))
	}

	var res cleanResult
	if err := json.Unmarshal(output, &res); err != nil {
		return cleanResult{}, fmt.Errorf("failed to parse duckdb clean result for %s: %w: %s", inputGlob, err, string(output))
	}
	if res.Status == "error" {
		return res, fmt.Errorf("%s", res.Error)
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
