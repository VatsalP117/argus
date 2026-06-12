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

type queryResult struct {
	Status       string                   `json:"status"`
	QueryName    string                   `json:"query_name"`
	OutputFormat string                   `json:"output_format"`
	RowCount     int64                    `json:"row_count"`
	Columns      []string                 `json:"columns"`
	Rows         []map[string]interface{} `json:"rows"`
	OutputPath   string                   `json:"output_path"`
	SQL          string                   `json:"sql,omitempty"`
	Error        string                   `json:"error,omitempty"`
}

type duckDBOptions struct {
	MemoryLimit string
	Threads     int
	TempDir     string
}

func main() {
	var pipelinePath string
	var queryName string
	var months string
	var signalType string
	var topicHint string
	var subreddit string
	var sourceType string
	var entityType string
	var entityText string
	var matchedPattern string
	var containsText string
	var sqlFile string
	var limit int
	var outputFormat string
	var outputPath string
	var includeSQL bool
	var duckDBMemoryLimit string
	var duckDBThreads int
	var duckDBTempDir string

	flag.StringVar(&pipelinePath, "pipeline", "configs/pipelines/v0-travel-jan-feb-2021.yaml", "path to pipeline config")
	flag.StringVar(&queryName, "query-name", "signal_summary", "query name: signal_summary, signal_evidence, entity_summary, subreddit_metrics, source_search, custom_sql")
	flag.StringVar(&months, "months", "*", "comma-separated month filters in YYYY-MM format, or *")
	flag.StringVar(&signalType, "signal-type", "*", "optional signal type filter")
	flag.StringVar(&topicHint, "topic-hint", "*", "optional topic hint filter")
	flag.StringVar(&subreddit, "subreddit", "*", "optional subreddit filter")
	flag.StringVar(&sourceType, "source-type", "*", "optional source type filter")
	flag.StringVar(&entityType, "entity-type", "*", "optional entity type filter")
	flag.StringVar(&entityText, "entity-text", "*", "optional normalized entity filter")
	flag.StringVar(&matchedPattern, "matched-pattern", "*", "optional matched pattern filter")
	flag.StringVar(&containsText, "contains-text", "", "optional case-insensitive source text search filter")
	flag.StringVar(&sqlFile, "sql-file", "", "path to a single read-only SELECT/WITH sql file for query-name=custom_sql")
	flag.IntVar(&limit, "limit", 50, "maximum rows to return")
	flag.StringVar(&outputFormat, "output-format", "json", "output format: json or csv")
	flag.StringVar(&outputPath, "output-path", "", "optional file path for json or required file path for csv")
	flag.BoolVar(&includeSQL, "include-sql", false, "include the generated SQL in the JSON output")
	flag.StringVar(&duckDBMemoryLimit, "duckdb-memory-limit", "4GB", "DuckDB memory limit for query jobs")
	flag.IntVar(&duckDBThreads, "duckdb-threads", 4, "DuckDB threads for query jobs")
	flag.StringVar(&duckDBTempDir, "duckdb-temp-dir", filepath.Join(".duckdb", "tmp"), "DuckDB temp spill directory")
	flag.Parse()

	if limit <= 0 {
		panic("limit must be greater than zero")
	}
	if outputFormat != "json" && outputFormat != "csv" {
		panic("output-format must be json or csv")
	}
	if outputFormat == "csv" && strings.TrimSpace(outputPath) == "" {
		panic("output-path is required when output-format=csv")
	}

	cfg, err := config.LoadPipelineConfig(pipelinePath)
	if err != nil {
		panic(err)
	}

	startedAt := time.Now().UTC()
	runID := "query-" + startedAt.Format("20060102T150405.000000000Z")
	duckdbOpts := duckDBOptions{
		MemoryLimit: duckDBMemoryLimit,
		Threads:     duckDBThreads,
		TempDir:     duckDBTempDir,
	}

	res, err := runDuckDBQuery(
		cfg,
		queryName,
		months,
		signalType,
		topicHint,
		subreddit,
		sourceType,
		entityType,
		entityText,
		matchedPattern,
		containsText,
		sqlFile,
		limit,
		outputFormat,
		outputPath,
		includeSQL,
		duckdbOpts,
	)
	if err != nil {
		panic(err)
	}

	rec := runmeta.RunRecord{
		RunID:          runID,
		Phase:          "query",
		JobName:        "query_layer",
		StartedAt:      startedAt.Format(time.RFC3339),
		FinishedAt:     time.Now().UTC().Format(time.RFC3339),
		Status:         res.Status,
		GitSHA:         gitSHA(),
		ConfigHash:     configHash(pipelinePath, sqlFile, queryName, months, signalType, topicHint, subreddit, sourceType, entityType, entityText, matchedPattern, containsText, outputFormat),
		RecordsSeen:    res.RowCount,
		RecordsWritten: res.RowCount,
		InputRefs:      buildInputRefs(pipelinePath, sqlFile),
		OutputRefs:     buildOutputRefs(res.OutputPath),
		Notes:          fmt.Sprintf("Query=%s months=%s signal_type=%s topic_hint=%s subreddit=%s output_format=%s row_count=%d", queryName, normalizeFilter(months), normalizeFilter(signalType), normalizeFilter(topicHint), normalizeFilter(subreddit), outputFormat, res.RowCount),
	}

	runPath := filepath.Join(cfg.State.RunsDir, "query", runID+".json")
	if err := runmeta.Write(runPath, rec); err != nil {
		panic(err)
	}

	encoded, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(encoded))
}

func runDuckDBQuery(
	cfg config.PipelineConfig,
	queryName, months, signalType, topicHint, subreddit, sourceType, entityType, entityText, matchedPattern, containsText, sqlFile string,
	limit int,
	outputFormat, outputPath string,
	includeSQL bool,
	duckdbOpts duckDBOptions,
) (queryResult, error) {
	args := []string{
		"scripts/dev/duckdb_query_layer.py",
		"--query-name", queryName,
		"--clean-dir", cfg.Output.CleanDir,
		"--marts-dir", cfg.Output.MartsDir,
		"--months", months,
		"--signal-type", normalizeFilter(signalType),
		"--topic-hint", normalizeFilter(topicHint),
		"--subreddit", normalizeFilter(subreddit),
		"--source-type", normalizeFilter(sourceType),
		"--entity-type", normalizeFilter(entityType),
		"--entity-text", normalizeFilter(entityText),
		"--matched-pattern", normalizeFilter(matchedPattern),
		"--contains-text", strings.TrimSpace(containsText),
		"--limit", fmt.Sprintf("%d", limit),
		"--output-format", outputFormat,
		"--duckdb-memory-limit", duckdbOpts.MemoryLimit,
		"--duckdb-threads", fmt.Sprintf("%d", duckdbOpts.Threads),
		"--duckdb-temp-dir", duckdbOpts.TempDir,
	}
	if strings.TrimSpace(sqlFile) != "" {
		args = append(args, "--sql-file", sqlFile)
	}
	if strings.TrimSpace(outputPath) != "" {
		args = append(args, "--output-path", outputPath)
	}
	if includeSQL {
		args = append(args, "--include-sql")
	}

	cmd := exec.Command("python3", args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return queryResult{}, fmt.Errorf("duckdb query failed: %w: %s", err, string(output))
	}

	var res queryResult
	if err := json.Unmarshal(output, &res); err != nil {
		return queryResult{}, fmt.Errorf("failed to parse duckdb query result: %w: %s", err, string(output))
	}
	if res.Status == "error" {
		return res, fmt.Errorf("%s", res.Error)
	}
	return res, nil
}

func buildInputRefs(pipelinePath, sqlFile string) []string {
	refs := []string{pipelinePath}
	if strings.TrimSpace(sqlFile) != "" {
		refs = append(refs, sqlFile)
	}
	return refs
}

func buildOutputRefs(outputPath string) []string {
	if strings.TrimSpace(outputPath) == "" {
		return nil
	}
	return []string{outputPath}
}

func normalizeFilter(value string) string {
	cleaned := strings.ToLower(strings.TrimSpace(value))
	if cleaned == "" {
		return "*"
	}
	return cleaned
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
		if strings.TrimSpace(part) == "" {
			continue
		}
		data, err := os.ReadFile(part)
		if err == nil {
			hash.Write(data)
			continue
		}
		hash.Write([]byte(part))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
