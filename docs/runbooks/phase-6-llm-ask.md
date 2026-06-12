# Phase 6 LLM Ask Runbook

This runbook covers the first LLM-backed `ask` workflow for Argus.

## Provider Choice

The first implementation is DeepSeek-first, but shaped around an OpenAI-compatible chat-completions interface.

Why:

- the DeepSeek API is documented as OpenAI-compatible
- it can return JSON output for structured planner and answer steps
- it lets Argus keep the query layer and storage model provider-agnostic

## Environment

Create `.env` from `.env.example` and fill in the real key:

```bash
cp .env.example .env
```

Notes:

- `cmd/ask` automatically loads `.env` and `.env.local`
- `DEEPSEEK_BASE_URL` defaults to `https://api.deepseek.com`
- `DEEPSEEK_MODEL` defaults to `deepseek-v4-flash`
- do not commit API keys to the repo, configs, or shell scripts

## Command

Ask a question directly:

```bash
go run ./cmd/ask \
  --question "What pain points about visas come up most often?" \
  --output-path data/exports/ask-visa.json
```

Or load the question from a file:

```bash
go run ./cmd/ask \
  --question-file docs/examples/ask-question.txt
```

Useful flags:

- `--months`
- `--query-limit`
- `--output-path`
- `--include-sql`

## Flow

`cmd/ask` performs four steps:

1. Send the natural-language question to the LLM planner.
2. Validate the returned query plan against allowed query names and filters.
3. Execute the local read-only query layer over curated DuckDB views.
4. Send only the retrieved rows back to the LLM for evidence-backed synthesis.

## Guardrails

The planner can only choose from:

- `signal_summary`
- `signal_evidence`
- `entity_summary`
- `subreddit_metrics`
- `source_search`

The planner cannot:

- run raw SQL directly
- point at arbitrary parquet paths
- issue write statements
- bypass local retrieval validation

The answer step is instructed to:

- use only retrieved rows
- cite row refs like `q1.r1`
- admit when evidence is thin

## Output Shape

The command returns JSON with:

- `question`
- `intent`
- `query_plan`
- `query_results`
- `answer.summary`
- `answer.claims`
- `answer.evidence`
- `answer.caveats`

This makes the first prototype inspectable and easy to debug.

## Current Limitations

- planning quality depends on the model following the JSON contract
- retrieval is still limited to the deterministic clean and mart views
- there is no semantic embedding search yet
- there is no live UI or API wrapper yet
- the current implementation has not been live-verified unless you run it with your own environment variables

## Recommended Next Steps

1. Add a small evaluation set of analyst questions with expected evidence-backed answers.
2. Decide whether to keep planning purely via JSON mode or move to tool-calling later.
3. Add optional semantic retrieval over `source_documents.analytical_text`.
4. Add a tiny API or UI shell around `cmd/ask`.
