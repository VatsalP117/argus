# Phase 8 Durable Candidate Commit

## Purpose

Score one checksum-validated candidate shard, commit retained evidence transactionally to DuckDB, validate reconciliation, and delete temporary staging only after durable verification.

## Score

```bash
go run ./cmd/score-candidates \
  --scan-checkpoint state/checkpoints/candidate-scan/<manifest-id>/<entry-id>.json
```

The scorer reads `configs/relevance/deterministic-v2.yaml` by default and emits one row per candidate and domain:

- `A` and `B`: retain
- `C`: evaluation pool
- `D`: discard after metrics

Semantic and classifier scores remain null. Context boosts, ambiguity penalties, required evidence groups, and all current decisions are deterministic and explainable.

## Evaluate

Use the checked-in labelled fixture to compare a score output before committing it:

```bash
go run ./cmd/evaluate-relevance \
  --labels evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv \
  --score-path <score-parquet>
```

The command reports candidate and per-domain retained precision/recall. It returns exit status `3` when candidate retained precision is below `70%`.

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

`deterministic_v2` passed the expanded 339-row engineering fixture at `85.1%` retained precision and `80.0%` retained recall. One adjacent bounded shard may be scanned and reviewed next. Do not run a full month until the new-shard yield is reviewed and a small independent human spot-check confirms the agent-reviewed labels.

## Adjacent-Shard Validation Result (2026-06-14)

The adjacent shard `comments-2021-01-001` was scanned, scored with the unchanged `deterministic_v2` config, and labelled. The quality gates were **not** met:

- exact retained precision: **54.9%** (gate ≥ 70%)
- weighted retained recall estimate: **2.1%** (gate ≥ 60%)
- per-domain precision failures: `travel` 56.0%, `app_opportunity` 45.8%
- retained promotion/bot false positives: 2 (gate = 0)
- `lexical_ambiguity` false-positive share: 21.6% (gate ≤ 20%)

Because the gates failed, the isolated temporary DuckDB lifecycle proof was skipped and no data was committed to `data/argus.duckdb`. The scorer config was not edited after observing the shard. The recommended next step is a separate `v3` calibration cycle on a new training/validation split, followed by re-validation on a fresh adjacent shard before any full-month run.
