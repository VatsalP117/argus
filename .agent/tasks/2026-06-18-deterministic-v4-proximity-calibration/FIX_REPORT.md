# Fix Report

## Issues Fixed

The `NEEDS_HUMAN_DECISION` review verdict identified a label-boundary conflict
on passport/citizenship/vaccine-passport text between the two training fixtures.
The human decision resolved this:

> For the current travel scorer, only concrete travel/process/document pain is
> travel-positive. Abstract passport/citizenship/vaccine-passport commentary is
> out of scope for the current travel scorer, but should be noted as a possible
> future adjacent research domain.

### What was changed

Seventeen labels were reconciled across both fixtures, all within the
passport/citizenship/vaccine-passport boundary:

- `comments-2021-01-001`: 13 travel-positive labels changed to travel-negative
  (abstract passport/citizenship/vaccine-passport commentary).
- `comments-2021-01-000`: 4 travel-positive labels changed to travel-negative
  (abstract residency policy commentary, passport application success story,
  immigration reform opinion).

Cases kept as travel-positive: 9 concrete travel-document/process pain cases
(visa requirements, asylum, border security, passport/visa for travel, green
card process, airport/travel process).

No scorer code was changed. No config was changed. No labels outside the
passport/citizenship/vaccine-passport boundary were altered.

### Label change details

See `docs/reports/deterministic-v4-calibration-2026-06-18.md` for the full
table of changed source IDs with rationale and false-positive category
assignments.

## Files Changed

- `evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json`
  - Removed 13 source IDs from `positive_source_ids.travel`.
  - Added 9 to `false_positive_categories.lexical_ambiguity`.
  - Added 4 to `false_positive_categories.political_immigration`.
- `evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv`
  - Changed `label_travel` from `1` to `0` for 13 source IDs.
- `evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json`
  - Removed 4 source IDs from `positive_source_ids.travel`.
  - Added 3 to `false_positive_categories.political_immigration`.
- `evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv`
  - Changed `label_travel` from `1` to `0` for 4 source IDs.
- `docs/reports/deterministic-v4-calibration-2026-06-18.md`
  - Updated with reconciled-label results, label change documentation, and
    new decision section.
- `.agent/tasks/2026-06-18-deterministic-v4-proximity-calibration/CHANGE_REQUEST.md`
  - Updated: passport boundary resolved; remaining blocker is deterministic
    recall ceiling.

## Commands Run

```bash
python3 scripts/dev/calibrate_relevance_fixtures.py \
  --relevance-config configs/relevance/deterministic-v4.yaml \
  --output-dir .tmp/relevance-v4-label-reconciled \
  --fixture 'comments-2021-01-000|evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv|evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json' \
  --fixture 'comments-2021-01-001|evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv|evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json'

python3 scripts/dev/calibrate_relevance_fixtures.py \
  --relevance-config configs/relevance/deterministic-v3.yaml \
  --output-dir .tmp/relevance-v3-reconciled \
  --fixture 'comments-2021-01-000|evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv|evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json' \
  --fixture 'comments-2021-01-001|evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv|evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json'

go test ./internal/config ./internal/relevance ./cmd/score-candidates ./cmd/evaluate-relevance
go test ./...
go vet ./...
python3 -m py_compile scripts/dev/*.py
git diff --check
```

## Calibration Results After Reconciliation

| Fixture | Retained | TP | FP | Precision | Recall | Gate |
| :-- | --: | --: | --: | --: | --: | :-- |
| `comments-2021-01-000` | `67` | `57` | `10` | `85.1%` | `59.4%` | pass |
| `comments-2021-01-001` | `38` | `31` | `7` | `81.6%` | `36.5%` | fail |

V4 vs V3 on same reconciled labels:

| Fixture | V3 precision | V4 precision | V3 recall | V4 recall |
| :-- | --: | --: | --: | --: |
| `comments-2021-01-000` | `84.1%` | `85.1%` | `55.2%` | `59.4%` |
| `comments-2021-01-001` | `79.4%` | `81.6%` | `31.8%` | `36.5%` |

V4 does not regress below V3 on either fixture.

## Remaining Issues

The passport/citizenship label-boundary conflict is resolved, but
`comments-2021-01-001` still fails the `50%` recall gate at `36.5%`.

### Exact remaining blocker

`001` has `85` total positives after reconciliation. V4 retains `31` true
positives (`36.5%` recall). The gate requires `43` TP (`50%`).

The `54` remaining false negatives are non-passport cases:

- `5` at score `0.55-0.60` (need `+0.05` to retain)
- `3` at score `0.50-0.55` (need `+0.10`)
- `13` at score `0.45-0.50` (need `+0.15`)
- `5` at score `0.40-0.45` (need `+0.20`)
- `28` at score `<0.40` (hard)

Reaching `43` TP requires retaining `12` more candidates, which means boosting
`0.45-0.50` tier candidates by `+0.15`. The original analysis showed this drags
in `14` evaluate-tier labeled-negative candidates and collapses precision below
`71%`, violating the `75%` precision gate.

This is a deterministic recall ceiling, not a label-boundary issue. Proximity
rules cannot fix it because the remaining FNs lack a general proximity pattern
that separates them from labeled-negative candidates at the same scores.

### Recommendation

The passport boundary is resolved. The remaining blocker is the deterministic
recall ceiling on `001`. The recommended next step is the roadmap's learned
retrieval fallback: keep deterministic candidate retrieval (including V4
proximity rules) as the high-recall front door, and add a lightweight learned
reranker or classifier to recover the `0.45-0.55` tier candidates that
deterministic scoring cannot boost without precision collapse.

Abstract passport/citizenship/vaccine-passport commentary should be noted as a
possible future adjacent research domain, per the human decision.
