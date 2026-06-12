# Phase 4 Signal Enrichment Runbook

This runbook covers the V0 deterministic enrichment path on the frozen Jan-Feb 2021 pilot slice.

## What Exists Now

Phase 4 currently includes:

- a Go `enrich-signals` CLI
- a DuckDB-backed deterministic signal extraction helper
- entity mention extraction driven by checked-in config
- monthly daily-metrics mart rebuilds
- validation SQL and first research-facing SQL workflows

## Recommended V0 Scope

Use the frozen local slice:

- domain: `travel`
- months:
  - `2021-01`
  - `2021-02`
- record types:
  - `comments`
  - `submissions`

## Run Enrichment

Run one record type and month at a time:

```bash
go run ./cmd/enrich-signals \
  --pipeline configs/pipelines/pilot-travel-q1-2021.yaml \
  --record-type comments \
  --month 2021-01
```

Repeat for:

- `comments 2021-02`
- `submissions 2021-01`
- `submissions 2021-02`

Or rebuild them all with `--force` as needed.

## Expected Outputs

- `data/marts/research_signals/year=YYYY/month=MM/*.parquet`
- `data/marts/entity_mentions/year=YYYY/month=MM/*.parquet`
- `data/marts/subreddit_metrics_daily/year=YYYY/month=MM/*.parquet`
- `state/checkpoints/phase4/`
- `state/runs/phase4/`

## Validation

Run:

```bash
python3 scripts/dev/run_duckdb_sql_file.py \
  --sql-file sql/checks/phase-4-signal-validation.sql
```

What to check:

- major signal types have non-zero rows
- every signal row has source ids and evidence text
- entity mentions are non-zero for at least domain terms
- daily metrics rows exist for both January and February

## First Research Queries

Pain points:

```bash
python3 scripts/dev/run_duckdb_sql_file.py \
  --sql-file sql/marts/v0-pain-point-discovery.sql
```

App-idea style signals:

```bash
python3 scripts/dev/run_duckdb_sql_file.py \
  --sql-file sql/marts/v0-app-idea-discovery.sql
```

## Current Limitations

- rules are phrase-based and intentionally simple
- entity extraction is dictionary-driven, not NER
- daily metrics are month-bounded rebuilds, not a global warehouse job
- no evidence export bundle exists yet
