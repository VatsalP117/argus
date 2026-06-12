package main

import (
	"context"
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
	"argus/internal/llm"
	"argus/internal/runmeta"
)

const (
	defaultDeepSeekBaseURL = "https://api.deepseek.com"
	defaultDeepSeekModel   = "deepseek-v4-flash"
	maxPlanQueries         = 3
	maxQueryLimit          = 10
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

type queryExecution struct {
	QueryName string                   `json:"query_name"`
	Filters   map[string]string        `json:"filters"`
	Limit     int                      `json:"limit"`
	Reason    string                   `json:"reason"`
	Columns   []string                 `json:"columns"`
	Rows      []map[string]interface{} `json:"rows"`
	RowCount  int64                    `json:"row_count"`
}

type plannerResponse struct {
	Intent    string         `json:"intent"`
	QueryPlan []plannedQuery `json:"query_plan"`
	Notes     []string       `json:"notes"`
}

type plannedQuery struct {
	QueryName string            `json:"query_name"`
	Filters   map[string]string `json:"filters"`
	Limit     int               `json:"limit"`
	Reason    string            `json:"reason"`
}

type answerResponse struct {
	Summary  string        `json:"summary"`
	Claims   []answerClaim `json:"claims"`
	Evidence []evidenceRef `json:"evidence"`
	Caveats  []string      `json:"caveats"`
}

type answerClaim struct {
	Statement    string   `json:"statement"`
	EvidenceRefs []string `json:"evidence_refs"`
	Confidence   string   `json:"confidence"`
}

type evidenceRef struct {
	Ref string `json:"ref"`
	Why string `json:"why"`
}

type askOutput struct {
	Status       string           `json:"status"`
	Provider     string           `json:"provider"`
	Model        string           `json:"model"`
	Question     string           `json:"question"`
	Intent       string           `json:"intent"`
	PlannerNotes []string         `json:"planner_notes"`
	QueryPlan    []plannedQuery   `json:"query_plan"`
	QueryResults []queryExecution `json:"query_results"`
	Answer       answerResponse   `json:"answer"`
	OutputPath   string           `json:"output_path"`
	Error        string           `json:"error,omitempty"`
}

type duckDBOptions struct {
	MemoryLimit string
	Threads     int
	TempDir     string
}

func main() {
	var pipelinePath string
	var provider string
	var model string
	var question string
	var questionFile string
	var months string
	var queryLimit int
	var answerOutputPath string
	var includeSQL bool
	var duckDBMemoryLimit string
	var duckDBThreads int
	var duckDBTempDir string

	flag.StringVar(&pipelinePath, "pipeline", "configs/pipelines/v0-travel-jan-feb-2021.yaml", "path to pipeline config")
	flag.StringVar(&provider, "provider", "deepseek", "llm provider")
	flag.StringVar(&model, "model", "", "llm model override")
	flag.StringVar(&question, "question", "", "natural language analyst question")
	flag.StringVar(&questionFile, "question-file", "", "optional path to a file containing the analyst question")
	flag.StringVar(&months, "months", "*", "comma-separated month filters in YYYY-MM format, or *")
	flag.IntVar(&queryLimit, "query-limit", 6, "maximum rows per retrieval query")
	flag.StringVar(&answerOutputPath, "output-path", "", "optional path to write the ask response json")
	flag.BoolVar(&includeSQL, "include-sql", false, "include generated SQL in query execution payloads")
	flag.StringVar(&duckDBMemoryLimit, "duckdb-memory-limit", "4GB", "DuckDB memory limit for ask retrieval jobs")
	flag.IntVar(&duckDBThreads, "duckdb-threads", 4, "DuckDB threads for ask retrieval jobs")
	flag.StringVar(&duckDBTempDir, "duckdb-temp-dir", filepath.Join(".duckdb", "tmp"), "DuckDB temp spill directory")
	flag.Parse()

	if strings.TrimSpace(question) == "" && strings.TrimSpace(questionFile) == "" {
		panic("question or question-file is required")
	}
	if queryLimit <= 0 {
		panic("query-limit must be greater than zero")
	}
	if queryLimit > maxQueryLimit {
		queryLimit = maxQueryLimit
	}

	loadedQuestion := strings.TrimSpace(question)
	if strings.TrimSpace(questionFile) != "" {
		data, err := os.ReadFile(questionFile)
		if err != nil {
			panic(err)
		}
		loadedQuestion = strings.TrimSpace(string(data))
	}
	if loadedQuestion == "" {
		panic("question is empty after loading")
	}

	cfg, err := config.LoadPipelineConfig(pipelinePath)
	if err != nil {
		panic(err)
	}

	startedAt := time.Now().UTC()
	runID := "ask-" + startedAt.Format("20060102T150405.000000000Z")
	selectedProvider := strings.ToLower(strings.TrimSpace(provider))
	if selectedProvider != "deepseek" {
		panic("only provider=deepseek is implemented right now")
	}

	baseURL := envOrDefault("DEEPSEEK_BASE_URL", defaultDeepSeekBaseURL)
	selectedModel := strings.TrimSpace(model)
	if selectedModel == "" {
		selectedModel = envOrDefault("DEEPSEEK_MODEL", defaultDeepSeekModel)
	}
	apiKey := strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	if apiKey == "" {
		panic("DEEPSEEK_API_KEY is required in the environment")
	}

	client := llm.NewClient(baseURL, apiKey)
	duckdbOpts := duckDBOptions{
		MemoryLimit: duckDBMemoryLimit,
		Threads:     duckDBThreads,
		TempDir:     duckDBTempDir,
	}

	output, err := runAskFlow(context.Background(), client, cfg, selectedProvider, selectedModel, loadedQuestion, months, queryLimit, includeSQL, duckdbOpts)
	if err != nil {
		panic(err)
	}
	output.Status = "completed"
	output.Provider = selectedProvider
	output.Model = selectedModel
	output.Question = loadedQuestion

	if strings.TrimSpace(answerOutputPath) != "" {
		if err := writeJSON(answerOutputPath, output); err != nil {
			panic(err)
		}
		output.OutputPath = answerOutputPath
	}

	rec := runmeta.RunRecord{
		RunID:          runID,
		Phase:          "ask",
		JobName:        "llm_ask",
		StartedAt:      startedAt.Format(time.RFC3339),
		FinishedAt:     time.Now().UTC().Format(time.RFC3339),
		Status:         output.Status,
		GitSHA:         gitSHA(),
		ConfigHash:     configHash(pipelinePath, questionFile, loadedQuestion, selectedProvider, selectedModel, months),
		RecordsSeen:    int64(len(output.QueryPlan)),
		RecordsWritten: int64(len(output.QueryResults)),
		InputRefs:      buildAskInputRefs(pipelinePath, questionFile),
		OutputRefs:     buildAskOutputRefs(answerOutputPath),
		Notes:          fmt.Sprintf("Ask provider=%s model=%s queries=%d question=%q", selectedProvider, selectedModel, len(output.QueryPlan), trimForNote(loadedQuestion, 120)),
	}
	runPath := filepath.Join(cfg.State.RunsDir, "ask", runID+".json")
	if err := runmeta.Write(runPath, rec); err != nil {
		panic(err)
	}

	encoded, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(encoded))
}

func runAskFlow(ctx context.Context, client llm.Client, cfg config.PipelineConfig, provider, model, question, months string, queryLimit int, includeSQL bool, duckdbOpts duckDBOptions) (askOutput, error) {
	plan, err := planQueries(ctx, client, model, question, months, queryLimit)
	if err != nil {
		return askOutput{}, err
	}

	validPlan := sanitizePlan(plan.QueryPlan, question, queryLimit)
	if len(validPlan) == 0 {
		validPlan = fallbackPlan(question, queryLimit)
	}

	executions := make([]queryExecution, 0, len(validPlan))
	for _, item := range validPlan {
		res, err := runDuckDBQuery(cfg, item, months, includeSQL, duckdbOpts)
		if err != nil {
			return askOutput{}, err
		}
		executions = append(executions, res)
	}

	answer, err := synthesizeAnswer(ctx, client, model, question, plan.Intent, plan.Notes, executions)
	if err != nil {
		return askOutput{}, err
	}

	return askOutput{
		Intent:       firstNonEmpty(plan.Intent, inferIntent(question)),
		PlannerNotes: plan.Notes,
		QueryPlan:    validPlan,
		QueryResults: executions,
		Answer:       answer,
	}, nil
}

func planQueries(ctx context.Context, client llm.Client, model, question, months string, queryLimit int) (plannerResponse, error) {
	systemPrompt := fmt.Sprintf(`You are planning read-only retrieval queries for Argus.

You must return json.

Available query_name values:
- signal_summary
- signal_evidence
- entity_summary
- subreddit_metrics
- source_search

Allowed filter keys:
- signal-type
- topic-hint
- subreddit
- source-type
- entity-type
- entity-text
- matched-pattern
- contains-text

Rules:
- Return 1 to %d queries.
- Prefer signal_summary plus signal_evidence for questions about pain points, requests, comparisons, or workarounds.
- Prefer entity_summary for recurring products, airlines, booking platforms, or payment tools.
- Prefer subreddit_metrics for trend or density questions.
- Prefer source_search for fuzzy textual drilldown.
- Use * for unknown exact filters.
- Keep each limit at or below %d.
- Do not invent query names or filter names.
- Return compact JSON only.

Output schema:
{
  "intent": "short string",
  "query_plan": [
    {
      "query_name": "signal_summary",
      "filters": {
        "signal-type": "pain_point",
        "topic-hint": "*",
        "subreddit": "*",
        "source-type": "*",
        "entity-type": "*",
        "entity-text": "*",
        "matched-pattern": "*",
        "contains-text": ""
      },
      "limit": 5,
      "reason": "short string"
    }
  ],
  "notes": ["short string"]
}`, maxPlanQueries, queryLimit)

	userPrompt := fmt.Sprintf("Question: %s\nMonths: %s\nReturn json only.", question, months)
	var plan plannerResponse
	_, err := client.CompleteJSON(ctx, llm.JSONRequest{
		Model: model,
		Messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		MaxTokens: 900,
	}, &plan)
	if err != nil {
		return plannerResponse{}, err
	}
	return plan, nil
}

func synthesizeAnswer(ctx context.Context, client llm.Client, model, question, intent string, notes []string, executions []queryExecution) (answerResponse, error) {
	systemPrompt := `You are an evidence-backed analyst for Argus.

You must return json.

Use only the retrieved query results. Do not invent facts.
If the evidence is thin, say so in caveats.
When you cite evidence, refer to rows using refs like q1.r1, q1.r2, q2.r1.

Output schema:
{
  "summary": "short paragraph",
  "claims": [
    {
      "statement": "specific evidence-backed claim",
      "evidence_refs": ["q1.r1"],
      "confidence": "high|medium|low"
    }
  ],
  "evidence": [
    {
      "ref": "q1.r1",
      "why": "why this row matters"
    }
  ],
  "caveats": ["short caveat"]
}`

	type synthesisPayload struct {
		Question string           `json:"question"`
		Intent   string           `json:"intent"`
		Notes    []string         `json:"notes"`
		Queries  []queryExecution `json:"queries"`
	}

	payload := synthesisPayload{
		Question: question,
		Intent:   intent,
		Notes:    notes,
		Queries:  annotateRowsWithRefs(executions),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return answerResponse{}, err
	}

	var answer answerResponse
	_, err = client.CompleteJSON(ctx, llm.JSONRequest{
		Model: model,
		Messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: string(body)},
		},
		MaxTokens: 1400,
	}, &answer)
	if err != nil {
		return answerResponse{}, err
	}
	return normalizeAnswer(answer), nil
}

func annotateRowsWithRefs(executions []queryExecution) []queryExecution {
	out := make([]queryExecution, 0, len(executions))
	for queryIdx, execution := range executions {
		copyExecution := queryExecution{
			QueryName: execution.QueryName,
			Filters:   execution.Filters,
			Limit:     execution.Limit,
			Reason:    execution.Reason,
			Columns:   execution.Columns,
			Rows:      make([]map[string]interface{}, 0, len(execution.Rows)),
			RowCount:  execution.RowCount,
		}
		for rowIdx, row := range execution.Rows {
			enriched := cloneMap(row)
			enriched["ref"] = fmt.Sprintf("q%d.r%d", queryIdx+1, rowIdx+1)
			copyExecution.Rows = append(copyExecution.Rows, enriched)
		}
		out = append(out, copyExecution)
	}
	return out
}

func normalizeAnswer(answer answerResponse) answerResponse {
	answer.Summary = strings.TrimSpace(answer.Summary)
	answer.Caveats = normalizeStringSlice(answer.Caveats)
	for idx := range answer.Claims {
		answer.Claims[idx].Statement = strings.TrimSpace(answer.Claims[idx].Statement)
		answer.Claims[idx].Confidence = normalizeConfidence(answer.Claims[idx].Confidence)
		answer.Claims[idx].EvidenceRefs = normalizeStringSlice(answer.Claims[idx].EvidenceRefs)
	}
	for idx := range answer.Evidence {
		answer.Evidence[idx].Ref = strings.TrimSpace(answer.Evidence[idx].Ref)
		answer.Evidence[idx].Why = strings.TrimSpace(answer.Evidence[idx].Why)
	}
	return answer
}

func sanitizePlan(plan []plannedQuery, question string, defaultLimit int) []plannedQuery {
	valid := make([]plannedQuery, 0, len(plan))
	for _, item := range plan {
		name := strings.TrimSpace(item.QueryName)
		if !allowedQueryName(name) {
			continue
		}
		filters := sanitizeFilters(item.Filters)
		limit := item.Limit
		if limit <= 0 || limit > maxQueryLimit {
			limit = defaultLimit
		}
		valid = append(valid, plannedQuery{
			QueryName: name,
			Filters:   filters,
			Limit:     limit,
			Reason:    strings.TrimSpace(item.Reason),
		})
		if len(valid) >= maxPlanQueries {
			break
		}
	}
	if len(valid) == 0 {
		return fallbackPlan(question, defaultLimit)
	}
	return valid
}

func sanitizeFilters(filters map[string]string) map[string]string {
	allowed := map[string]struct{}{
		"signal-type":     {},
		"topic-hint":      {},
		"subreddit":       {},
		"source-type":     {},
		"entity-type":     {},
		"entity-text":     {},
		"matched-pattern": {},
		"contains-text":   {},
	}
	sanitized := map[string]string{}
	for key, value := range filters {
		if _, ok := allowed[key]; !ok {
			continue
		}
		if key == "contains-text" {
			sanitized[key] = strings.TrimSpace(value)
			continue
		}
		sanitized[key] = normalizeWildcardFilter(value)
	}
	return sanitized
}

func fallbackPlan(question string, queryLimit int) []plannedQuery {
	lowerQuestion := strings.ToLower(question)
	switch {
	case strings.Contains(lowerQuestion, "entity"), strings.Contains(lowerQuestion, "airbnb"), strings.Contains(lowerQuestion, "airline"), strings.Contains(lowerQuestion, "product"), strings.Contains(lowerQuestion, "platform"):
		return []plannedQuery{{QueryName: "entity_summary", Filters: map[string]string{}, Limit: queryLimit, Reason: "Fallback entity summary retrieval"}}
	case strings.Contains(lowerQuestion, "trend"), strings.Contains(lowerQuestion, "density"), strings.Contains(lowerQuestion, "daily"), strings.Contains(lowerQuestion, "which subreddit"):
		return []plannedQuery{{QueryName: "subreddit_metrics", Filters: map[string]string{}, Limit: queryLimit, Reason: "Fallback metrics retrieval"}}
	case strings.Contains(lowerQuestion, "find"), strings.Contains(lowerQuestion, "example"), strings.Contains(lowerQuestion, "quote"), strings.Contains(lowerQuestion, "search"):
		return []plannedQuery{{QueryName: "source_search", Filters: map[string]string{"contains-text": extractKeywordFallback(question)}, Limit: queryLimit, Reason: "Fallback source search retrieval"}}
	default:
		return []plannedQuery{
			{QueryName: "signal_summary", Filters: map[string]string{"signal-type": inferSignalType(question)}, Limit: queryLimit, Reason: "Fallback signal summary retrieval"},
			{QueryName: "signal_evidence", Filters: map[string]string{"signal-type": inferSignalType(question)}, Limit: queryLimit, Reason: "Fallback evidence retrieval"},
		}
	}
}

func inferSignalType(question string) string {
	lowerQuestion := strings.ToLower(question)
	switch {
	case strings.Contains(lowerQuestion, "pain"), strings.Contains(lowerQuestion, "frustrat"), strings.Contains(lowerQuestion, "problem"), strings.Contains(lowerQuestion, "complaint"):
		return "pain_point"
	case strings.Contains(lowerQuestion, "request"), strings.Contains(lowerQuestion, "recommendation"), strings.Contains(lowerQuestion, "app idea"):
		return "recommendation_request"
	case strings.Contains(lowerQuestion, "feature"):
		return "feature_request"
	case strings.Contains(lowerQuestion, "workaround"), strings.Contains(lowerQuestion, "hack"):
		return "workaround"
	case strings.Contains(lowerQuestion, "compare"), strings.Contains(lowerQuestion, "versus"), strings.Contains(lowerQuestion, "vs"):
		return "comparison"
	default:
		return "*"
	}
}

func inferIntent(question string) string {
	lowerQuestion := strings.ToLower(question)
	switch {
	case strings.Contains(lowerQuestion, "pain"), strings.Contains(lowerQuestion, "problem"), strings.Contains(lowerQuestion, "frustrat"):
		return "pain_point_analysis"
	case strings.Contains(lowerQuestion, "request"), strings.Contains(lowerQuestion, "feature"), strings.Contains(lowerQuestion, "recommendation"):
		return "request_analysis"
	case strings.Contains(lowerQuestion, "trend"), strings.Contains(lowerQuestion, "density"):
		return "metrics_analysis"
	case strings.Contains(lowerQuestion, "entity"), strings.Contains(lowerQuestion, "product"), strings.Contains(lowerQuestion, "airbnb"):
		return "entity_analysis"
	default:
		return "general_research"
	}
}

func extractKeywordFallback(question string) string {
	words := strings.Fields(strings.ToLower(question))
	stopwords := map[string]struct{}{
		"what": {}, "which": {}, "where": {}, "when": {}, "does": {}, "with": {}, "from": {}, "that": {}, "this": {}, "have": {}, "show": {}, "find": {}, "about": {}, "into": {}, "their": {}, "there": {}, "would": {}, "could": {}, "should": {}, "people": {}, "posts": {}, "examples": {}, "example": {}, "quotes": {}, "quote": {},
	}
	for _, word := range words {
		word = strings.Trim(word, ".,?!:;\"'()[]{}")
		if len(word) < 4 {
			continue
		}
		if _, ok := stopwords[word]; ok {
			continue
		}
		return word
	}
	return ""
}

func allowedQueryName(name string) bool {
	switch name {
	case "signal_summary", "signal_evidence", "entity_summary", "subreddit_metrics", "source_search":
		return true
	default:
		return false
	}
}

func normalizeWildcardFilter(value string) string {
	cleaned := strings.ToLower(strings.TrimSpace(value))
	if cleaned == "" {
		return "*"
	}
	return cleaned
}

func runDuckDBQuery(cfg config.PipelineConfig, plan plannedQuery, months string, includeSQL bool, duckdbOpts duckDBOptions) (queryExecution, error) {
	args := []string{
		"scripts/dev/duckdb_query_layer.py",
		"--query-name", plan.QueryName,
		"--clean-dir", cfg.Output.CleanDir,
		"--marts-dir", cfg.Output.MartsDir,
		"--months", months,
		"--signal-type", planFilter(plan.Filters, "signal-type"),
		"--topic-hint", planFilter(plan.Filters, "topic-hint"),
		"--subreddit", planFilter(plan.Filters, "subreddit"),
		"--source-type", planFilter(plan.Filters, "source-type"),
		"--entity-type", planFilter(plan.Filters, "entity-type"),
		"--entity-text", planFilter(plan.Filters, "entity-text"),
		"--matched-pattern", planFilter(plan.Filters, "matched-pattern"),
		"--contains-text", strings.TrimSpace(plan.Filters["contains-text"]),
		"--limit", fmt.Sprintf("%d", plan.Limit),
		"--output-format", "json",
		"--duckdb-memory-limit", duckdbOpts.MemoryLimit,
		"--duckdb-threads", fmt.Sprintf("%d", duckdbOpts.Threads),
		"--duckdb-temp-dir", duckdbOpts.TempDir,
	}
	if includeSQL {
		args = append(args, "--include-sql")
	}

	cmd := exec.Command("python3", args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return queryExecution{}, fmt.Errorf("query execution failed for %s: %w: %s", plan.QueryName, err, string(output))
	}

	var res queryResult
	if err := json.Unmarshal(output, &res); err != nil {
		return queryExecution{}, fmt.Errorf("failed to parse query result for %s: %w: %s", plan.QueryName, err, string(output))
	}
	if res.Status == "error" {
		return queryExecution{}, fmt.Errorf("%s", res.Error)
	}

	return queryExecution{
		QueryName: plan.QueryName,
		Filters:   normalizedFiltersForOutput(plan.Filters),
		Limit:     plan.Limit,
		Reason:    plan.Reason,
		Columns:   res.Columns,
		Rows:      res.Rows,
		RowCount:  res.RowCount,
	}, nil
}

func planFilter(filters map[string]string, key string) string {
	value := strings.TrimSpace(filters[key])
	if key == "contains-text" {
		return value
	}
	if value == "" {
		return "*"
	}
	return value
}

func normalizedFiltersForOutput(filters map[string]string) map[string]string {
	if len(filters) == 0 {
		return map[string]string{}
	}
	keys := make([]string, 0, len(filters))
	for key := range filters {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make(map[string]string, len(filters))
	for _, key := range keys {
		out[key] = filters[key]
	}
	return out
}

func normalizeStringSlice(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		cleaned := strings.TrimSpace(item)
		if cleaned == "" {
			continue
		}
		out = append(out, cleaned)
	}
	return out
}

func normalizeConfidence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high", "medium", "low":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "medium"
	}
}

func cloneMap(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func writeJSON(path string, payload any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func buildAskInputRefs(pipelinePath, questionFile string) []string {
	refs := []string{pipelinePath}
	if strings.TrimSpace(questionFile) != "" {
		refs = append(refs, questionFile)
	}
	return refs
}

func buildAskOutputRefs(outputPath string) []string {
	if strings.TrimSpace(outputPath) == "" {
		return nil
	}
	return []string{outputPath}
}

func trimForNote(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen] + "..."
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
