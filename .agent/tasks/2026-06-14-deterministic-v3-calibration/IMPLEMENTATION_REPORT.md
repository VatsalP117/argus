# Implementation Report

## Summary

Implemented a bounded `deterministic_v3` calibration workflow, added a new
versioned config, extended the scorer/config schema with optional
`minimum_group_matches`, and added focused scorer/config tests. The final
tracked v3 candidate is reproducible and explainable, but it does not meet the
`comments-2021-01-001` recall target, so the work stops as
`failed_calibration` with a written `CHANGE_REQUEST.md`.

## Files Changed

- `scripts/dev/calibrate_relevance_fixtures.py`
- `configs/relevance/deterministic-v3.yaml`
- `scripts/dev/duckdb_score_candidates.py`
- `internal/config/relevance.go`
- `internal/config/relevance_test.go`
- `internal/relevance/scorer_test.go`
- `docs/reports/deterministic-v3-calibration-2026-06-17.md`
- `.agent/tasks/2026-06-14-deterministic-v3-calibration/CHANGE_REQUEST.md`
- `.agent/tasks/2026-06-14-deterministic-v3-calibration/IMPLEMENTATION_REPORT.md`

## Commands Run

```bash
go test ./...

python3 -m py_compile scripts/dev/calibrate_relevance_fixtures.py
python3 scripts/dev/calibrate_relevance_fixtures.py --help

python3 scripts/dev/calibrate_relevance_fixtures.py \
  --relevance-config configs/relevance/deterministic-v2.yaml \
  --output-dir .tmp/relevance-v2-baseline-check \
  --fixture 'comments-2021-01-000|evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv|evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json' \
  --fixture 'comments-2021-01-001|evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv|evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json'

go test ./internal/config ./internal/relevance ./cmd/score-candidates ./cmd/evaluate-relevance
python3 -m py_compile scripts/dev/*.py

python3 scripts/dev/calibrate_relevance_fixtures.py \
  --relevance-config configs/relevance/deterministic-v3.yaml \
  --output-dir .tmp/relevance-v3-iterA \
  --fixture 'comments-2021-01-000|evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv|evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json' \
  --fixture 'comments-2021-01-001|evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv|evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json'

python3 scripts/dev/calibrate_relevance_fixtures.py \
  --relevance-config configs/relevance/deterministic-v3.yaml \
  --output-dir .tmp/relevance-v3-iterB \
  --fixture 'comments-2021-01-000|evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv|evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json' \
  --fixture 'comments-2021-01-001|evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv|evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json'

python3 scripts/dev/calibrate_relevance_fixtures.py \
  --relevance-config configs/relevance/deterministic-v3.yaml \
  --output-dir .tmp/relevance-v3-iterC \
  --fixture 'comments-2021-01-000|evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv|evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json' \
  --fixture 'comments-2021-01-001|evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv|evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json'
```

## Tests

- `go test ./internal/config ./internal/relevance ./cmd/score-candidates ./cmd/evaluate-relevance` passed
- `python3 -m py_compile scripts/dev/*.py` passed
- Final calibration result:
  - `comments-2021-01-000`: pass
  - `comments-2021-01-001`: fail
  - overall: `failed_calibration`

## Deviations From Plan

- I did not regenerate the original full candidate parquet for
  `comments-2021-01-000` because it is no longer present in the tracked
  workspace. Instead, the runner reconstructs bounded candidates directly from
  the approved labelled fixtures.
- I stopped before any candidate-scanner or broader scorer redesign and wrote
  `CHANGE_REQUEST.md` once the tracked config still failed the `001` recall
  target.

## Known Risks

- The bounded reconstructed-candidate protocol is excellent for repeatable
  label-fixture calibration, but its full-label recall estimate is not the same
  as population-weighted recall on the original shard.
- The tracked v3 config still under-recovers `travel` and `app_opportunity`
  positives on `comments-2021-01-001`.
- The untracked local workspace contains prior `.tmp/relevance-v3-iter*`
  artifacts from earlier experimentation; I did not rely on frozen shards, but
  I did inspect those local bounded outputs to avoid blind retuning.

## Next Steps

Do not validate on shard `002` or `003` yet. Review
`CHANGE_REQUEST.md`, decide whether to authorize a more expressive deterministic
scorer capability, and then resume calibration in a follow-up task.
