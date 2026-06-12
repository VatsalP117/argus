# Phase 2 Runbook: Manifest And Smoke Ingest

This runbook describes the current implementation slice of Phase 2.

## What Exists Now

Phase 2 currently includes:

- a Hugging Face tree-api manifest builder in Go
- a shard-level ingest worker in Go
- a month-grouped batch ingest mode in Go
- a DuckDB-based shard filter and copy helper in Python
- checkpoint and run-record writing

## Prerequisites

1. DuckDB Python package installed:

```bash
python3 -m pip install --user duckdb
```

2. Go toolchain available.

3. Valid pilot config:

```text
configs/pipelines/pilot-travel-q1-2021.yaml
```

## Build A Smoke Manifest

Generate a month-bounded manifest for the smoke month:

```bash
go run ./cmd/manifest-builder \
  --pipeline configs/pipelines/pilot-travel-q1-2021.yaml \
  --month 2021-01 \
  --output manifests/pilot/travel-q1-2021-smoke-manifest.json
```

Expected result:

- a manifest JSON file under `manifests/pilot/`
- a run record under `state/runs/phase2/`

## Run A Tiny Smoke Ingest

Process only a few shards first:

```bash
go run ./cmd/ingest-worker \
  --pipeline configs/pipelines/pilot-travel-q1-2021.yaml \
  --manifest manifests/pilot/travel-q1-2021-smoke-manifest.json \
  --month 2021-01 \
  --record-type submissions \
  --limit-shards 3
```

Then repeat for comments:

```bash
go run ./cmd/ingest-worker \
  --pipeline configs/pipelines/pilot-travel-q1-2021.yaml \
  --manifest manifests/pilot/travel-q1-2021-smoke-manifest.json \
  --month 2021-01 \
  --record-type comments \
  --limit-shards 3
```

## Expected Outputs

- filtered Parquet files under `data/raw/<record_type>/year=YYYY/month=MM/`
- checkpoints under `state/checkpoints/phase2/`
- run records under `state/runs/phase2/`

## Important Notes

- zero-row shard outputs are allowed and are recorded as completed with zero rows
- the worker is shard-level resumable because output files and checkpoints are written per entry
- this is a smoke ingest, not the full quarter ingest

## Run A Month-Grouped Ingest

For a more practical local run, process one whole month and record type at a time in bounded batches:

```bash
go run ./cmd/ingest-worker \
  --pipeline configs/pipelines/pilot-travel-q1-2021.yaml \
  --manifest manifests/pilot/travel-q1-2021-full-manifest.json \
  --record-type submissions \
  --month 2021-01 \
  --batch-size 8 \
  --max-batch-source-bytes 536870912 \
  --group-by-month
```

The grouped mode writes one output file per bounded batch instead of one file per shard.
Each grouped batch uses exact manifest URLs rather than a month wildcard scan.

## Current Runtime Observation

Local filter-on-ingest works for tiny shard smoke runs and has been verified with non-zero travel rows.

However:

- full month grouped extraction still needs measurement, but bounded batches are safer than one large month wildcard scan
- a full Q1 2021 local raw backfill should be treated as a long-running operational job
- the implementation is in place, but full pilot ingest time needs to be measured in a dedicated unattended run

## Immediate Next Step After Smoke

If the smoke ingest works:

1. inspect raw outputs
2. validate row counts
3. widen shard count
4. widen to the rest of `2021-01`
5. only then consider `2021-02` and `2021-03`

Use [phase-2-validation.md](/Users/vatsalpatel/Desktop/Projects/argus/docs/runbooks/phase-2-validation.md) for the formal preflight, resume, and raw validation flow.
