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

type enrichResult struct {
	Status             string `json:"status"`
	SignalsWritten     int64  `json:"signals_written"`
	EntityRowsWritten  int64  `json:"entity_rows_written"`
	MetricsRowsWritten int64  `json:"metrics_rows_written"`
	SignalOutputPath   string `json:"signal_output_path"`
	EntityOutputPath   string `json:"entity_output_path"`
	MetricsOutputPath  string `json:"metrics_output_path"`
	SourcePath         string `json:"source_path"`
	Error              string `json:"error,omitempty"`
}

type duckDBOptions struct {
	MemoryLimit string
	Threads     int
	TempDir     string
}

type enrichConfigBundle struct {
	SignalVersion string       `json:"signal_version"`
	DomainName    string       `json:"domain_name"`
	SignalRules   []signalRule `json:"signal_rules"`
	EntityRules   []entityRule `json:"entity_rules"`
}

type signalRule struct {
	SignalType       string `json:"signal_type"`
	Label            string `json:"label"`
	Regex            string `json:"regex"`
	RequireTopicHint bool   `json:"require_topic_hint"`
}

type entityRule struct {
	EntityType string `json:"entity_type"`
	Label      string `json:"label"`
	Regex      string `json:"regex"`
}

func main() {
	var pipelinePath string
	var signalConfigPath string
	var domainConfigPath string
	var month string
	var recordType string
	var force bool
	var duckDBMemoryLimit string
	var duckDBThreads int
	var duckDBTempDir string

	flag.StringVar(&pipelinePath, "pipeline", "configs/pipelines/pilot-travel-q1-2021.yaml", "path to pipeline config")
	flag.StringVar(&signalConfigPath, "signal-config", "configs/signals/deterministic-v1.yaml", "path to signal config")
	flag.StringVar(&domainConfigPath, "domain-config", "", "optional path to domain config")
	flag.StringVar(&month, "month", "", "optional month filter, format YYYY-MM")
	flag.StringVar(&recordType, "record-type", "", "optional record type filter")
	flag.BoolVar(&force, "force", false, "reprocess signal outputs even if they already exist")
	flag.StringVar(&duckDBMemoryLimit, "duckdb-memory-limit", "4GB", "DuckDB memory limit for enrich-signals jobs")
	flag.IntVar(&duckDBThreads, "duckdb-threads", 4, "DuckDB threads for enrich-signals jobs")
	flag.StringVar(&duckDBTempDir, "duckdb-temp-dir", filepath.Join(".duckdb", "tmp"), "DuckDB temp spill directory")
	flag.Parse()

	cfg, err := config.LoadPipelineConfig(pipelinePath)
	if err != nil {
		panic(err)
	}

	if domainConfigPath == "" {
		domainConfigPath = filepath.Join("configs", "domains", cfg.Domain+".yaml")
	}

	domainCfg, err := config.LoadDomainConfig(domainConfigPath)
	if err != nil {
		panic(err)
	}

	signalCfg, err := config.LoadSignalConfig(signalConfigPath)
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
	runID := "phase4-enrich-" + startedAt.Format("20060102T150405.000000000Z")
	duckdbOpts := duckDBOptions{
		MemoryLimit: duckDBMemoryLimit,
		Threads:     duckDBThreads,
		TempDir:     duckDBTempDir,
	}

	bundle := buildConfigBundle(domainCfg, signalCfg)

	var processed int
	var totalSignals int64
	var totalEntities int64
	var totalMetrics int64
	var errorCount int
	var warnings []string
	var outputRefs []string

	for _, rt := range recordTypes {
		for _, m := range months {
			res, err := runMonth(cfg, runID, bundle, rt, m, force, duckdbOpts)
			processed++
			if err != nil {
				errorCount++
				warnings = append(warnings, err.Error())
				continue
			}
			totalSignals += res.SignalsWritten
			totalEntities += res.EntityRowsWritten
			totalMetrics += res.MetricsRowsWritten
			if res.SignalOutputPath != "" {
				outputRefs = append(outputRefs, res.SignalOutputPath)
			}
			if res.EntityOutputPath != "" {
				outputRefs = append(outputRefs, res.EntityOutputPath)
			}
			if res.MetricsOutputPath != "" {
				outputRefs = append(outputRefs, res.MetricsOutputPath)
			}
		}
	}

	rec := runmeta.RunRecord{
		RunID:          runID,
		Phase:          "phase4",
		JobName:        "enrich_signals",
		StartedAt:      startedAt.Format(time.RFC3339),
		FinishedAt:     time.Now().UTC().Format(time.RFC3339),
		Status:         "completed",
		GitSHA:         gitSHA(),
		ConfigHash:     configHash(pipelinePath, signalConfigPath, domainConfigPath),
		RecordsSeen:    int64(processed),
		RecordsWritten: totalSignals,
		ErrorCount:     errorCount,
		Warnings:       warnings,
		InputRefs:      []string{pipelinePath, signalConfigPath, domainConfigPath},
		OutputRefs:     outputRefs,
		Notes:          fmt.Sprintf("Processed %d enrichment work units, wrote %d signals, %d entity mentions, %d metrics rows", processed, totalSignals, totalEntities, totalMetrics),
	}
	if errorCount > 0 {
		rec.Status = "partial"
	}

	runPath := filepath.Join(cfg.State.RunsDir, "phase4", runID+".json")
	if err := runmeta.Write(runPath, rec); err != nil {
		panic(err)
	}

	fmt.Printf("run complete: %s\n", runID)
	fmt.Printf("work_units_processed: %d\n", processed)
	fmt.Printf("signals_written: %d\n", totalSignals)
	fmt.Printf("entity_rows_written: %d\n", totalEntities)
	fmt.Printf("metrics_rows_written: %d\n", totalMetrics)
	fmt.Printf("errors: %d\n", errorCount)
}

func runMonth(cfg config.PipelineConfig, runID string, bundle enrichConfigBundle, recordType, month string, force bool, duckdbOpts duckDBOptions) (enrichResult, error) {
	parts := splitMonth(month)
	inputGlob := filepath.Join(
		cfg.Output.CleanDir,
		recordType,
		"year="+parts[0],
		"month="+parts[1],
		"*.parquet",
	)
	signalOutputPath := filepath.Join(
		cfg.Output.MartsDir,
		"research_signals",
		"year="+parts[0],
		"month="+parts[1],
		fmt.Sprintf("research-signals-%s-%s.parquet", recordType, month),
	)
	entityOutputPath := filepath.Join(
		cfg.Output.MartsDir,
		"entity_mentions",
		"year="+parts[0],
		"month="+parts[1],
		fmt.Sprintf("entity-mentions-%s-%s.parquet", recordType, month),
	)
	metricsOutputPath := filepath.Join(
		cfg.Output.MartsDir,
		"subreddit_metrics_daily",
		"year="+parts[0],
		"month="+parts[1],
		fmt.Sprintf("subreddit-metrics-daily-%s.parquet", month),
	)

	cpPath := filepath.Join(cfg.State.CheckpointsDir, "phase4", runID, recordType+"-"+month+".json")
	cp := checkpoint.ShardCheckpoint{
		JobName:      "enrich_signals",
		RunID:        runID,
		EntryID:      recordType + "-" + month,
		SourcePath:   inputGlob,
		OutputPath:   signalOutputPath,
		StartedAt:    time.Now().UTC().Format(time.RFC3339),
		Status:       "started",
		AttemptCount: 1,
	}

	if !force {
		if usable(signalOutputPath) && usable(entityOutputPath) && usable(metricsOutputPath) {
			cp.Status = "skipped_existing"
			cp.FinishedAt = time.Now().UTC().Format(time.RFC3339)
			if err := checkpoint.Write(cpPath, cp); err != nil {
				return enrichResult{}, err
			}
			return enrichResult{
				Status:            "skipped_existing",
				SignalOutputPath:  signalOutputPath,
				EntityOutputPath:  entityOutputPath,
				MetricsOutputPath: metricsOutputPath,
			}, nil
		}
	}

	configJSONPath, err := writeBundleConfig(runID, recordType, month, bundle)
	if err != nil {
		return enrichResult{}, err
	}

	res, err := runDuckDBEnrich(
		inputGlob,
		signalOutputPath,
		entityOutputPath,
		metricsOutputPath,
		configJSONPath,
		recordType,
		runID,
		month,
		cfg.Output.CleanDir,
		cfg.Output.MartsDir,
		duckdbOpts,
	)
	if err != nil {
		cp.Status = "failed"
		cp.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		cp.Error = err.Error()
		if writeErr := checkpoint.Write(cpPath, cp); writeErr != nil {
			return enrichResult{}, writeErr
		}
		return enrichResult{}, err
	}

	cp.Status = res.Status
	cp.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	cp.RowsWritten = res.SignalsWritten
	cp.OutputPath = res.SignalOutputPath
	if err := checkpoint.Write(cpPath, cp); err != nil {
		return enrichResult{}, err
	}

	return res, nil
}

func buildConfigBundle(domainCfg config.DomainConfig, signalCfg config.SignalConfig) enrichConfigBundle {
	var rules []signalRule
	appendRules := func(signalType string, patterns []string) {
		for _, pattern := range patterns {
			rules = append(rules, signalRule{
				SignalType:       signalType,
				Label:            pattern,
				Regex:            phraseRegex(pattern),
				RequireTopicHint: requiresTopicHint(signalType, pattern),
			})
		}
	}

	appendRules("pain_point", signalCfg.PainPointPatterns)
	appendRules("feature_request", signalCfg.FeatureRequestPatterns)
	appendRules("recommendation_request", signalCfg.RecommendationRequestPatterns)
	appendRules("workaround", signalCfg.WorkaroundPatterns)
	appendRules("comparison", signalCfg.ComparisonPatterns)

	var entityRules []entityRule
	appendEntityRules := func(entityType string, terms []string) {
		for _, term := range terms {
			entityRules = append(entityRules, entityRule{
				EntityType: entityType,
				Label:      strings.ToLower(term),
				Regex:      phraseRegex(term),
			})
		}
	}

	appendEntityRules("domain_term", domainCfg.SeedKeywords)
	appendEntityRules("product", domainCfg.ProductTerms)
	appendEntityRules("airline", domainCfg.AirlineTerms)
	appendEntityRules("booking_platform", domainCfg.BookingPlatforms)
	appendEntityRules("payment_tool", domainCfg.PaymentToolTerms)

	return enrichConfigBundle{
		SignalVersion: signalCfg.Version,
		DomainName:    domainCfg.Name,
		SignalRules:   rules,
		EntityRules:   entityRules,
	}
}

func writeBundleConfig(runID, recordType, month string, bundle enrichConfigBundle) (string, error) {
	path := filepath.Join(".tmp", "phase4-configs", runID, recordType+"-"+month+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func phraseRegex(phrase string) string {
	replacer := strings.NewReplacer(
		`\\`, `\\\\`,
		".", `\.`,
		"+", `\+`,
		"*", `\*`,
		"?", `\?`,
		"(", `\(`,
		")", `\)`,
		"[", `\[`,
		"]", `\]`,
		"{", `\{`,
		"}", `\}`,
		"|", `\|`,
		"^", `\^`,
		"$", `\$`,
	)
	escaped := replacer.Replace(strings.ToLower(strings.TrimSpace(phrase)))
	escaped = strings.Join(strings.Fields(escaped), `\s+`)
	return `(^|[^a-z0-9])` + escaped + `([^a-z0-9]|$)`
}

func requiresTopicHint(signalType, pattern string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))

	if signalType == "comparison" {
		return true
	}

	if signalType != "pain_point" {
		return false
	}

	switch pattern {
	case "annoying", "frustrating", "difficult to", "struggle with":
		return true
	default:
		return false
	}
}

func runDuckDBEnrich(inputGlob, signalOutputPath, entityOutputPath, metricsOutputPath, configJSONPath, recordType, signalRunID, month, cleanDir, martsDir string, duckdbOpts duckDBOptions) (enrichResult, error) {
	args := []string{
		"scripts/dev/duckdb_enrich_signals.py",
		"--input-glob", inputGlob,
		"--signal-output-path", signalOutputPath,
		"--entity-output-path", entityOutputPath,
		"--metrics-output-path", metricsOutputPath,
		"--config-json", configJSONPath,
		"--record-type", recordType,
		"--signal-run-id", signalRunID,
		"--month", month,
		"--clean-dir", cleanDir,
		"--marts-dir", martsDir,
		"--duckdb-memory-limit", duckdbOpts.MemoryLimit,
		"--duckdb-threads", fmt.Sprintf("%d", duckdbOpts.Threads),
		"--duckdb-temp-dir", duckdbOpts.TempDir,
	}

	cmd := exec.Command("python3", args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return enrichResult{}, fmt.Errorf("duckdb enrich failed for %s: %w: %s", inputGlob, err, string(output))
	}

	var res enrichResult
	if err := json.Unmarshal(output, &res); err != nil {
		return enrichResult{}, fmt.Errorf("failed to parse duckdb enrich result for %s: %w: %s", inputGlob, err, string(output))
	}
	if res.Status == "error" {
		return res, fmt.Errorf("%s", res.Error)
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

func usable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}

func gitSHA() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return string(bytesTrimSpace(out))
}

func configHash(paths ...string) string {
	h := sha256.New()
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		_, _ = h.Write(data)
		_, _ = h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
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
