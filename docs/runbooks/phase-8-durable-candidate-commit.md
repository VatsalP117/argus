# Phase 8 Durable Candidate Commit

## Purpose

Score one checksum-validated candidate shard, commit retained evidence transactionally to DuckDB, validate reconciliation, and delete temporary staging only after durable verification.

## Score

```bash
go run ./cmd/score-candidates \
  --scan-checkpoint state/checkpoints/candidate-scan/<manifest-id>/<entry-id>.json
```

The scorer reads `configs/relevance/deterministic-v1.yaml` and emits one row per candidate and domain:

- `A` and `B`: retain
- `C`: evaluation pool
- `D`: discard after metrics

Semantic and classifier scores remain null. All current decisions are deterministic and explainable.

## Commit

```bash
go run ./cmd/commit-candidates \
  --manifest /tmp/argus-jan-comments-pinned.json \
  --entry-id comments-2021-01-000
```

The command:

1. verifies manifest, candidate, score, and configuration checksums
2. creates a stable private author-hash salt under `state/secrets/`
3. rejects duplicate source IDs or mismatched source identities
4. inserts documents, relevance, signals, and entity terms in one transaction
5. writes scan, staging, batch, and reconciliation metadata
6. validates durable row counts and checksum before committing

An unchanged validated retry returns `skipped_existing`.

## Cleanup

Use the `ingest_batch_id` returned by the commit:

```bash
go run ./cmd/cleanup-staging \
  --ingest-batch-id <ingest-batch-id>
```

Cleanup recomputes:

- candidate and score file checksums
- staged row counts
- durable document count and checksum
- durable relevance rows against retained score rows
- source and staging reconciliation equations

It records one cleanup event per file, removes candidate and score Parquet, then marks the batch cleaned. Tampered, missing, unvalidated, or inconsistent staging is not deleted.

## Current Gate

Do not scan another shard yet. `deterministic_v1` is a calibration model, not a production relevance model. Build and label the evaluation set before widening ingestion.
