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

	"argus/internal/config"
	"argus/internal/manifest"
	"argus/internal/runmeta"
)

type preflightCountResult struct {
	Status         string  `json:"status"`
	RecordType     string  `json:"record_type"`
	MatchedRows    int64   `json:"matched_rows"`
	ElapsedSeconds float64 `json:"elapsed_seconds"`
	SourceCount    int     `json:"source_count"`
	Error          string  `json:"error,omitempty"`
}

type preflightGroup struct {
	RecordType  string  `json:"record_type"`
	Month       string  `json:"month"`
	EntryCount  int     `json:"entry_count"`
	SourceBytes int64   `json:"source_bytes"`
	MatchedRows int64   `json:"matched_rows"`
	ElapsedSecs float64 `json:"elapsed_seconds"`
}

type preflightReport struct {
	GeneratedAt  string           `json:"generated_at"`
	PipelineName string           `json:"pipeline_name"`
	ManifestID   string           `json:"manifest_id"`
	Groups       []preflightGroup `json:"groups"`
	TotalEntries int              `json:"total_entries"`
	TotalBytes   int64            `json:"total_bytes"`
	TotalRows    int64            `json:"total_rows"`
}

func main() {
	var pipelinePath string
	var manifestPath string
	var outputPath string
	var month string
	var recordType string
	var limitShards int
	var duckDBMemoryLimit string
	var duckDBThreads int
	var duckDBTempDir string

	flag.StringVar(&pipelinePath, "pipeline", "configs/pipelines/pilot-travel-q1-2021.yaml", "path to pipeline config")
	flag.StringVar(&manifestPath, "manifest", "", "path to manifest json")
	flag.StringVar(&outputPath, "output", "", "optional output json path")
	flag.StringVar(&month, "month", "", "optional month filter, format YYYY-MM")
	flag.StringVar(&recordType, "record-type", "", "optional record type filter")
	flag.IntVar(&limitShards, "limit-shards", 0, "optional max shards to process")
	flag.StringVar(&duckDBMemoryLimit, "duckdb-memory-limit", "4GB", "DuckDB memory limit for preflight jobs")
	flag.IntVar(&duckDBThreads, "duckdb-threads", 4, "DuckDB threads for preflight jobs")
	flag.StringVar(&duckDBTempDir, "duckdb-temp-dir", filepath.Join(".duckdb", "tmp"), "DuckDB temp spill directory")
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

	selected := selectEntries(man.Entries, month, recordType, limitShards)
	grouped := map[string][]manifest.Entry{}
	for _, entry := range selected {
		key := entry.RecordType + ":" + entry.Month
		grouped[key] = append(grouped[key], entry)
	}

	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	report := preflightReport{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		PipelineName: cfg.PipelineName,
		ManifestID:   man.ManifestID,
		Groups:       make([]preflightGroup, 0, len(keys)),
	}

	runID := "phase2-preflight-" + time.Now().UTC().Format("20060102T150405Z")
	for _, key := range keys {
		entries := grouped[key]
		urls := make([]string, 0, len(entries))
		var sourceBytes int64
		for _, entry := range entries {
			urls = append(urls, entry.ResolveURL)
			sourceBytes += entry.SizeBytes
		}

		result, err := runDuckDBCount(urls, entries[0].RecordType, cfg.Subreddits, duckDBMemoryLimit, duckDBThreads, duckDBTempDir)
		if err != nil {
			panic(err)
		}

		group := preflightGroup{
			RecordType:  entries[0].RecordType,
			Month:       entries[0].Month,
			EntryCount:  len(entries),
			SourceBytes: sourceBytes,
			MatchedRows: result.MatchedRows,
			ElapsedSecs: result.ElapsedSeconds,
		}
		report.Groups = append(report.Groups, group)
		report.TotalEntries += len(entries)
		report.TotalBytes += sourceBytes
		report.TotalRows += result.MatchedRows
	}

	if outputPath != "" {
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			panic(err)
		}
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			panic(err)
		}
		if err := os.WriteFile(outputPath, data, 0o644); err != nil {
			panic(err)
		}
	}

	rec := runmeta.RunRecord{
		RunID:          runID,
		Phase:          "phase2",
		JobName:        "preflight",
		StartedAt:      report.GeneratedAt,
		FinishedAt:     time.Now().UTC().Format(time.RFC3339),
		Status:         "completed",
		GitSHA:         gitSHA(),
		ConfigHash:     configHash(pipelinePath),
		RecordsSeen:    int64(report.TotalEntries),
		RecordsWritten: report.TotalRows,
		InputRefs:      []string{pipelinePath, manifestPath},
		OutputRefs:     maybeOutputRef(outputPath),
		Notes:          fmt.Sprintf("Preflighted %d entries across %d groups", report.TotalEntries, len(report.Groups)),
	}
	runPath := filepath.Join(cfg.State.RunsDir, "phase2", runID+".json")
	if err := runmeta.Write(runPath, rec); err != nil {
		panic(err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		panic(err)
	}
}

func maybeOutputRef(path string) []string {
	if path == "" {
		return nil
	}
	return []string{path}
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

func runDuckDBCount(urls []string, recordType string, subreddits []string, memoryLimit string, threads int, tempDir string) (preflightCountResult, error) {
	args := []string{
		"scripts/dev/duckdb_count.py",
		"--record-type", recordType,
		"--subreddits", strings.Join(subreddits, ","),
		"--duckdb-memory-limit", memoryLimit,
		"--duckdb-threads", fmt.Sprintf("%d", threads),
		"--duckdb-temp-dir", tempDir,
	}
	for _, item := range urls {
		args = append(args, "--input-url", item)
	}

	cmd := exec.Command("python3", args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return preflightCountResult{}, fmt.Errorf("duckdb count failed for %s: %w: %s", recordType, err, string(output))
	}

	var res preflightCountResult
	if err := json.Unmarshal(output, &res); err != nil {
		return preflightCountResult{}, fmt.Errorf("failed to parse duckdb count result for %s: %w: %s", recordType, err, string(output))
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
