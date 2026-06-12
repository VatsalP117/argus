# Phase 6 Query Layer Runbook

This runbook defines the first pre-LLM query contract for Argus.

## Why This Exists

Argus already has:

- Parquet as the source of truth
- DuckDB as the execution engine
- deterministic marts for signals, entities, and daily metrics

What was missing was a narrow, reusable query surface between those assets and a future LLM or UI.

The goal of this layer is:

- keep retrieval read-only
- keep lineage visible
- avoid letting an LLM improvise directly against raw parquet paths
- support more than a handful of canned exports

## What Exists Now

The query layer ships as:

- `go run ./cmd/query`
- `scripts/dev/duckdb_query_layer.py`

It exposes four curated temporary views:

- `source_documents`
- `research_signals`
- `entity_mentions`
- `subreddit_metrics_daily`

Those views are backed by the existing local clean and mart parquet outputs for the selected months.

## Query Shapes

Supported `--query-name` values:

- `signal_summary`
- `signal_evidence`
- `entity_summary`
- `subreddit_metrics`
- `source_search`
- `custom_sql`

Structured filters include:

- `--months`
- `--signal-type`
- `--topic-hint`
- `--subreddit`
- `--source-type`
- `--entity-type`
- `--entity-text`
- `--matched-pattern`
- `--contains-text`
- `--limit`

Default output is JSON so the response can feed an LLM or another program without extra parsing.

## Example Queries

Top pain-point groups:

```bash
go run ./cmd/query \
  --query-name signal_summary \
  --signal-type pain_point \
  --limit 10
```

Recent evidence rows for recommendation requests in `travel`:

```bash
go run ./cmd/query \
  --query-name signal_evidence \
  --signal-type recommendation_request \
  --subreddit travel \
  --limit 5
```

Recurring entities:

```bash
go run ./cmd/query \
  --query-name entity_summary \
  --entity-type booking_platform \
  --limit 10
```

Daily metrics for one subreddit:

```bash
go run ./cmd/query \
  --query-name subreddit_metrics \
  --subreddit digitalnomad \
  --limit 20
```

Free-text drilldown across cleaned source documents:

```bash
go run ./cmd/query \
  --query-name source_search \
  --contains-text visa \
  --limit 5
```

Guarded ad hoc SQL:

```bash
go run ./cmd/query \
  --query-name custom_sql \
  --sql-file sql/query_examples/pain_points_by_subreddit.sql \
  --limit 10
```

## Guardrails

The `custom_sql` mode is intentionally constrained.

Allowed:

- one `SELECT` statement
- one `WITH` statement that resolves to a `SELECT`
- reads only from the curated temporary views

Blocked:

- multiple statements
- `CREATE`, `COPY`, `INSERT`, `UPDATE`, `DELETE`, `DROP`, `ATTACH`, `LOAD`, `INSTALL`
- direct `read_parquet(...)` or other external file readers

This keeps the widened scope useful for analyst-style exploration without turning the LLM into an unrestricted database client.

## Output Contract

The command returns JSON like:

```json
{
  "status": "completed",
  "query_name": "signal_summary",
  "output_format": "json",
  "row_count": 3,
  "columns": ["signal_type", "topic_hint", "matched_pattern", "signal_count"],
  "rows": [
    {
      "signal_type": "pain_point",
      "topic_hint": "visa",
      "matched_pattern": "difficult to",
      "signal_count": 10
    }
  ],
  "output_path": ""
}
```

That shape is the intended bridge into an `ask` layer later.

## Pre-LLM Checklist

Done now:

- Parquet remains the source of truth.
- Deterministic marts remain the first retrieval layer.
- A reusable read-only query interface now exists.
- Query results are JSON-friendly and carry lineage-friendly fields.

Should be finished before broad LLM rollout:

1. Create a small evaluation set of real analyst questions plus expected evidence-backed outputs.
2. Define an answer schema for the future `ask` layer:
   - summary
   - claims
   - supporting rows
   - caveats
3. Document which query shape should be chosen for which user intent.
4. Add a few more automated tests around query validation and result-shape assumptions.
5. Decide whether semantic retrieval is needed now or after the first LLM prototype.

Nice to add soon after:

1. A tiny `cmd/ask` orchestrator that maps natural-language prompts into query plans.
2. Optional embeddings over `source_documents.analytical_text` for fuzzy retrieval.
3. Saved evaluation fixtures for regression testing LLM answers.

## First `ask` Interface Recommendation

The first `ask` layer should not generate raw parquet queries.

Recommended flow:

1. Interpret the user question.
2. Pick one or more query-layer calls.
3. Retrieve structured rows plus evidence.
4. Summarize only from retrieved rows.
5. Return citations using `source_id`, `subreddit`, `created_at`, and `source_file`.

Suggested response contract:

```json
{
  "question": "What pain points about visas come up most often?",
  "query_plan": [
    {
      "query_name": "signal_summary",
      "filters": {
        "signal_type": "pain_point",
        "topic_hint": "visa"
      }
    },
    {
      "query_name": "signal_evidence",
      "filters": {
        "signal_type": "pain_point",
        "topic_hint": "visa"
      }
    }
  ],
  "answer": {
    "summary": "Visa friction appears across multiple travel subreddits, with 'difficult to' as the dominant phrase in the frozen slice.",
    "claims": [],
    "evidence": [],
    "caveats": []
  }
}
```

That gives us a safe first LLM layer with room to widen into semantic retrieval later.
