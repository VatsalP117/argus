# Argus

Argus is a local-first Reddit research platform for market research, startup idea discovery, and pain-point analysis.

The core idea is simple:

- ingest bounded slices of historical Reddit data
- normalize and enrich them
- expose repeatable research workflows with evidence-backed outputs

Argus is deliberately not starting as:

- a full-history ingestion job
- a real-time streaming system
- a polished end-user product
- a warehouse-heavy data platform

## Current Status

Current implementation status:

- Phase 0: scaffolded
- Phase 1: archive discovery and pilot definition prepared
- Phase 2: core manifest and raw ingest path implemented for the frozen Jan-Feb 2021 V0 slice
- Phase 3: clean-layer implementation completed for the frozen Jan-Feb 2021 V0 slice
- Phase 4: deterministic signal enrichment and first research queries implemented for V0
- Phase 5: evidence export workflow implemented for V0

Companion docs:

- [SPEC.md](/Users/vatsalpatel/Desktop/Projects/argus/SPEC.md)
- [IMPLEMENTATION_GUIDE.md](/Users/vatsalpatel/Desktop/Projects/argus/IMPLEMENTATION_GUIDE.md)

## Default Operating Model

Argus v1 is local-first.

Default stack:

- DuckDB for remote archive access and local analytics
- Parquet for raw, clean, and mart layers
- Go for ingestion and pipeline tooling
- checked-in SQL plus configs for repeatable research workflows

## Initial Pilot

The first pilot is intentionally narrow:

- domain: travel
- target window: Q1 2021
- smoke month: 2021-01
- record types: submissions and comments
- research focus:
  - repeated travel pain points
  - request-like conversations suggesting app ideas
  - evidence exports for manual review

See the detailed pilot definition in [docs/research/pilot-definition.md](/Users/vatsalpatel/Desktop/Projects/argus/docs/research/pilot-definition.md).

## V0 POC Freeze

The current V0 proof-of-concept is intentionally frozen to:

- domain: travel
- months:
  - `2021-01`
  - `2021-02`
- record types:
  - comments
  - submissions

This lets the project move forward on enrichment and research workflows without waiting for full Q1 ingest.

## Fastest POC Rerun

To rebuild the current V0 marts, validations, research query outputs, and evidence bundles in one pass:

```bash
scripts/dev/run_v0_poc.sh
```

This writes a timestamped bundle under `data/exports/poc-run-*/`.

## Query The V0 Slice

For read-only querying over curated clean and mart views:

```bash
go run ./cmd/query \
  --query-name signal_summary \
  --signal-type pain_point \
  --limit 10
```

Search the cleaned source documents directly:

```bash
go run ./cmd/query \
  --query-name source_search \
  --contains-text visa \
  --limit 5
```

Run guarded ad hoc SQL against curated views only:

```bash
go run ./cmd/query \
  --query-name custom_sql \
  --sql-file sql/query_examples/pain_points_by_subreddit.sql \
  --limit 10
```

The available temporary views are:

- `source_documents`
- `research_signals`
- `entity_mentions`
- `subreddit_metrics_daily`

See [docs/runbooks/phase-6-query-layer.md](/Users/vatsalpatel/Desktop/Projects/argus/docs/runbooks/phase-6-query-layer.md) for the pre-LLM checklist and query-layer guardrails.

## Ask The V0 Slice With DeepSeek

Argus now includes a first `ask` prototype that:

- plans retrieval queries with an LLM
- runs only the guarded local query layer
- synthesizes an evidence-backed answer from retrieved rows

Create a local `.env` from [.env.example](/Users/vatsalpatel/Desktop/Projects/argus/.env.example) or let the existing local `.env` supply the values:

```bash
cp .env.example .env
```

Then ask a question:

```bash
go run ./cmd/ask \
  --question "What pain points about visas come up most often?" \
  --output-path data/exports/ask-visa.json
```

`cmd/ask` automatically loads `.env` and `.env.local` if present.

See [docs/runbooks/phase-6-llm-ask.md](/Users/vatsalpatel/Desktop/Projects/argus/docs/runbooks/phase-6-llm-ask.md) for the DeepSeek setup, flow, and guardrails.

## Local Requirements

Minimum practical local baseline:

- Apple Silicon Mac or similar modern laptop/desktop
- 16 GB RAM
- about 100 GB free SSD space before serious pilot ingest
- Python 3.9+
- Go 1.25+

DuckDB is currently expected via Python package:

```bash
python3 -m pip install --user duckdb
```

## Repository Layout

Important directories:

- `configs/`: domain, pipeline, and signal configuration
- `docs/`: ADRs, runbooks, and research notes
- `manifests/`: archive discovery outputs and pilot plans
- `sql/`: discovery, validation, and future mart queries
- `data/`: local raw, clean, mart, tmp, and export outputs
- `state/`: run logs and checkpoints

## Rules For Contributors And Agents

- Keep the pilot bounded until research outputs prove value.
- Preserve evidence traceability from every derived finding back to source rows.
- Keep raw, clean, and mart layers separate.
- Prefer checked-in SQL and config over notebook-only logic.
- Do not introduce ClickHouse or a VM until measured bottlenecks justify it.
