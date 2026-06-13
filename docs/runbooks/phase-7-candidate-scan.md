# Phase 7 Broad Candidate Scan

## Purpose

Scan one pinned archive shard with high-recall research rules without requiring subreddit membership. Output is temporary candidate Parquet, not durable retained data.

## Preconditions

```bash
go run ./cmd/db-migrate
go run ./cmd/db-status
```

`can_start_new_batch` must be `true`.

## Build A Pinned Manifest

```bash
go run ./cmd/manifest-builder \
  --pipeline configs/pipelines/pilot-travel-q1-2021.yaml \
  --month 2021-01 \
  --record-type comments \
  --output /tmp/argus-jan-comments-pinned.json
```

Do not use the older checked-in manifests for new ingestion. They predate archive revision pinning.

## Scan One Entry

```bash
go run ./cmd/scan-candidates \
  --manifest /tmp/argus-jan-comments-pinned.json \
  --entry-id comments-2021-01-000
```

Defaults:

- rules: `configs/candidates/broad-v1.yaml`
- staging: `data/tmp/candidates/<manifest-id>/<entry-id>-candidates.parquet`
- checkpoint: `state/checkpoints/candidate-scan/<manifest-id>/<entry-id>.json`
- DuckDB memory: `4GB`
- DuckDB threads: `4`

The scanner records:

- source rows seen
- candidate rows staged
- rows rejected early
- matches by rule group
- subreddit-prior-only candidates
- source identity and archive revision in every staged row
- candidate config and output checksums in the checkpoint

## Retry Behavior

Run the same command again. A valid completed checkpoint and matching Parquet checksum return:

```text
status: skipped_existing
```

Use `--force` only when intentionally replacing the staged output. A changed config, source identity, missing output, or checksum mismatch is not silently accepted.

## Current Boundary

Do not delete candidate staging yet. `cmd/commit-candidates`, durable post-write validation, and cleanup auditing are not implemented.

Do not scan a full month. The next implementation step is transactional candidate commit.
