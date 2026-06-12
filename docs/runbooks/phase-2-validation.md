# Phase 2 Validation Runbook

This runbook captures the validation path needed to close out Phase 2.

## 1. Run A Remote Preflight

This estimates filtered row counts before a larger ingest:

```bash
go run ./cmd/phase2-preflight \
  --pipeline configs/pipelines/pilot-travel-q1-2021.yaml \
  --manifest manifests/pilot/travel-q1-2021-full-manifest.json \
  --month 2021-01 \
  --output state/runs/phase2/preflight-2021-01.json
```

What to check:

- row counts are non-zero for both submissions and comments
- source byte totals match expectations from the manifest
- runtime is acceptable for the selected month

## 2. Run A Resume Test

Use a bounded batch shape so reruns are deterministic:

```bash
go run ./cmd/ingest-worker \
  --pipeline configs/pipelines/pilot-travel-q1-2021.yaml \
  --manifest manifests/pilot/travel-q1-2021-smoke-manifest.json \
  --record-type submissions \
  --month 2021-01 \
  --limit-shards 8 \
  --group-by-month \
  --batch-size 4 \
  --max-batch-source-bytes 536870912
```

Then rerun with a wider selection:

```bash
go run ./cmd/ingest-worker \
  --pipeline configs/pipelines/pilot-travel-q1-2021.yaml \
  --manifest manifests/pilot/travel-q1-2021-smoke-manifest.json \
  --record-type submissions \
  --month 2021-01 \
  --limit-shards 12 \
  --group-by-month \
  --batch-size 4 \
  --max-batch-source-bytes 536870912
```

Expected behavior:

- previously completed batch outputs are skipped
- only missing batches are created
- per-entry checkpoints exist for every selected manifest entry

## 3. Run Raw Validation SQL

Use DuckDB from Python:

```bash
python3 scripts/dev/run_duckdb_sql_file.py \
  --sql-file sql/checks/phase-2-raw-validation.sql
```

Recommended manual checks:

- row counts are non-zero for both record types
- provenance columns are present and complete
- distinct subreddit count matches the intended pilot subset

## 4. Phase 2 Closure Checklist

Phase 2 is ready to close when:

- manifests are reproducible and stably ordered
- preflight counts are recorded
- smoke ingest succeeds
- resume behavior is verified on a forced partial run
- raw validation SQL passes
- the selected pilot raw layer exists locally and can be regenerated from the manifest
