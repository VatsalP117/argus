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

	"argus/internal/config"
	"argus/internal/runmeta"
)

type exportResult struct {
	Status              string `json:"status"`
	SummaryRowsWritten  int64  `json:"summary_rows_written"`
	EvidenceRowsWritten int64  `json:"evidence_rows_written"`
	SummaryOutputPath   string `json:"summary_output_path"`
	EvidenceOutputPath  string `json:"evidence_output_path"`
	Error               string `json:"error,omitempty"`
}

type duckDBOptions struct {
	MemoryLimit string
	Threads     int
	TempDir     string
}

func main() {
	var pipelinePath string
	var signalType string
	var topicHint string
	var maxGroups int
	var examplesPerGroup int
	var duckDBMemoryLimit string
	var duckDBThreads int
	var duckDBTempDir string

	flag.StringVar(&pipelinePath, "pipeline", "configs/pipelines/v0-travel-jan-feb-2021.yaml", "path to pipeline config")
	flag.StringVar(&signalType, "signal-type", "pain_point", "signal type to export")
	flag.StringVar(&topicHint, "topic-hint", "", "optional topic hint filter")
	flag.IntVar(&maxGroups, "max-groups", 25, "maximum summary groups to export")
	flag.IntVar(&examplesPerGroup, "examples-per-group", 5, "maximum evidence examples per topic/pattern group")
	flag.StringVar(&duckDBMemoryLimit, "duckdb-memory-limit", "4GB", "DuckDB memory limit for export-evidence jobs")
	flag.IntVar(&duckDBThreads, "duckdb-threads", 4, "DuckDB threads for export-evidence jobs")
	flag.StringVar(&duckDBTempDir, "duckdb-temp-dir", filepath.Join(".duckdb", "tmp"), "DuckDB temp spill directory")
	flag.Parse()

	cfg, err := config.LoadPipelineConfig(pipelinePath)
	if err != nil {
		panic(err)
	}

	startedAt := time.Now().UTC()
	runID := "phase5-export-" + startedAt.Format("20060102T150405.000000000Z")
	outputDir := filepath.Join(cfg.Output.ExportsDir, runID)
	summaryPath := filepath.Join(outputDir, "summary.csv")
	evidencePath := filepath.Join(outputDir, "evidence.csv")
	signalGlob := filepath.Join(cfg.Output.MartsDir, "research_signals", "year=*", "month=*", "*.parquet")
	duckdbOpts := duckDBOptions{
		MemoryLimit: duckDBMemoryLimit,
		Threads:     duckDBThreads,
		TempDir:     duckDBTempDir,
	}

	res, err := runDuckDBExport(signalGlob, summaryPath, evidencePath, signalType, topicHint, maxGroups, examplesPerGroup, duckdbOpts)
	if err != nil {
		panic(err)
	}

	rec := runmeta.RunRecord{
		RunID:          runID,
		Phase:          "phase5",
		JobName:        "export_evidence",
		StartedAt:      startedAt.Format(time.RFC3339),
		FinishedAt:     time.Now().UTC().Format(time.RFC3339),
		Status:         res.Status,
		GitSHA:         gitSHA(),
		ConfigHash:     configHash(pipelinePath, signalType, topicHint),
		RecordsSeen:    res.SummaryRowsWritten,
		RecordsWritten: res.EvidenceRowsWritten,
		InputRefs:      []string{pipelinePath, signalGlob},
		OutputRefs:     []string{res.SummaryOutputPath, res.EvidenceOutputPath},
		Notes:          fmt.Sprintf("Exported %d summary rows and %d evidence rows for signal_type=%s topic_hint=%s", res.SummaryRowsWritten, res.EvidenceRowsWritten, signalType, normalizeTopicHint(topicHint)),
	}

	runPath := filepath.Join(cfg.State.RunsDir, "phase5", runID+".json")
	if err := runmeta.Write(runPath, rec); err != nil {
		panic(err)
	}

	fmt.Printf("run complete: %s\n", runID)
	fmt.Printf("signal_type: %s\n", signalType)
	fmt.Printf("topic_hint: %s\n", normalizeTopicHint(topicHint))
	fmt.Printf("summary_rows_written: %d\n", res.SummaryRowsWritten)
	fmt.Printf("evidence_rows_written: %d\n", res.EvidenceRowsWritten)
	fmt.Printf("summary_output: %s\n", res.SummaryOutputPath)
	fmt.Printf("evidence_output: %s\n", res.EvidenceOutputPath)
}

func normalizeTopicHint(topicHint string) string {
	if strings.TrimSpace(topicHint) == "" {
		return "*"
	}
	return strings.ToLower(strings.TrimSpace(topicHint))
}

func runDuckDBExport(signalGlob, summaryPath, evidencePath, signalType, topicHint string, maxGroups, examplesPerGroup int, duckdbOpts duckDBOptions) (exportResult, error) {
	args := []string{
		"scripts/dev/duckdb_export_evidence.py",
		"--signal-glob", signalGlob,
		"--summary-output-path", summaryPath,
		"--evidence-output-path", evidencePath,
		"--signal-type", signalType,
		"--topic-hint", normalizeTopicHint(topicHint),
		"--max-groups", fmt.Sprintf("%d", maxGroups),
		"--examples-per-group", fmt.Sprintf("%d", examplesPerGroup),
		"--duckdb-memory-limit", duckdbOpts.MemoryLimit,
		"--duckdb-threads", fmt.Sprintf("%d", duckdbOpts.Threads),
		"--duckdb-temp-dir", duckdbOpts.TempDir,
	}

	cmd := exec.Command("python3", args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return exportResult{}, fmt.Errorf("duckdb export failed for %s: %w: %s", signalGlob, err, string(output))
	}

	var res exportResult
	if err := json.Unmarshal(output, &res); err != nil {
		return exportResult{}, fmt.Errorf("failed to parse duckdb export result for %s: %w: %s", signalGlob, err, string(output))
	}
	if res.Status == "error" {
		return res, fmt.Errorf("%s", res.Error)
	}
	return res, nil
}

func gitSHA() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func configHash(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		data, err := os.ReadFile(part)
		if err == nil {
			hash.Write(data)
			continue
		}
		hash.Write([]byte(part))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
