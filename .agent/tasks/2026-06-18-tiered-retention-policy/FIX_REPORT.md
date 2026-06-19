# Fix Report

## Issues Fixed

### B1. `--include-review-tier` silently ignored on an already-committed batch (blocking)

- `scripts/dev/duckdb_commit_candidates.py`: `existing_result` now accepts
  `include_review_tier` and `score_path`. When a validated batch exists for the
  same `ingest_batch_id`, it recomputes the would-retain count under the
  requested scope from the score Parquet
  (`decision IN ('retain', 'evaluate')` vs `decision = 'retain'`) and compares
  it to the stored `retained_rows`. On mismatch it raises a clear error:
  `ingest batch <id> was already committed with a different retention scope
  (stored retained_rows=<n>, requested scope would retain=<m>);
  --include-review-tier cannot extend an existing batch, drop the batch or use
  a fresh manifest entry`.
- No schema change. No false positives: equal would-retain == stored
  `retained_rows` falls through to the existing `skipped_existing` path, so a
  same-scope retry remains idempotent (including the legitimate case where the
  source has zero `C` rows so both scopes retain the same rows).
- Call site updated to pass `include_review_tier`.

### B1 test (blocking test gap)

- `internal/candidate/committer_test.go`: added
  `TestCommitCandidatesReviewTierRetryOnDefaultBatchErrors`. Commits default
  into a temp DB, retries with `IncludeReviewTier = true` on the same batch and
  asserts the retry errors (not a silent `skipped_existing` with
  `rows_review_tier = 0`); asserts the durable corpus is unchanged; and asserts
  a same-scope retry remains idempotent (`skipped_existing`, unchanged counts).

### decision_reasons assertion (non-blocking, from review)

- `TestCommitCandidatesTieredRetentionReviewTierOptIn` now asserts the `C`-tier
  row preserves a non-empty `decision_reasons` (the DuckDB JSON column returns
  as a JSON string, so the assertion parses it and checks the list is
  non-empty).

### B2. Roadmap self-contradiction: `deterministic_v4` described as future (blocking)

- `docs/plans/argus-roadmap.md`: replaced the `### Immediate next step` and
  `### Expected outcomes` subsections (which described `v4` as not-yet-run)
  with an `### Observed outcome` section reflecting the completed `v4`
  `mixed_calibration` result and the failed learned fallback. Rewrote
  `## 7. Retrieval Decision Tree` from "Use this after `v4` completes" future
  tense to a resolved-outcome form (`v4` did not pass the observed recall gate;
  the tree resolved into the tiered retention policy; frozen-shard validation
  remains a gated follow-up after the tiered-retention trial).

## Files Changed

- `scripts/dev/duckdb_commit_candidates.py`
- `internal/candidate/committer_test.go`
- `docs/plans/argus-roadmap.md`
- `.agent/tasks/2026-06-18-tiered-retention-policy/IMPLEMENTATION_REPORT.md`
  (updated: scope guard in Files Changed, new retry test in Tests, idempotency
  risk in Known Risks, `decision_reasons` assertion)

## Commands Run

```bash
go test ./internal/candidate ./cmd/commit-candidates ./cmd/query ./cmd/export-evidence
go test ./...
go vet ./...
python3 -m py_compile scripts/dev/*.py
git diff --check
```

Focused new tests:

```bash
go test ./internal/candidate -run 'TestCommitCandidatesTieredRetention|TestCommitCandidatesReviewTierRetry' -v -count=1
```

Result: both `PASS`.

## Remaining Issues

None from `REVIEW.md`. All blocking issues (B1, B2) and the non-blocking
`decision_reasons` suggestion are addressed.

All checks pass: focused tests ok; `go test ./...` all ok; `go vet ./...`
clean; `py_compile` ok; `git diff --check` clean. Implementation remains
uncommitted on `agent/tiered-retention-policy`, ready for re-review.
