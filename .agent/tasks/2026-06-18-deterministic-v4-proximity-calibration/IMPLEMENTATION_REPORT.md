# Implementation Report

## Summary

Implemented `deterministic_v4` proximity-aware conjunction scoring and
calibrated it against the two observed training fixtures. V4 improves both
fixtures on every metric with zero new false positives and no trap leakage, but
the `comments-2021-01-001` recall gate (`50%`) cannot be met without resolving a
label-boundary conflict between the two training fixtures on
passport/citizenship text. Result is `mixed_calibration`; a `CHANGE_REQUEST.md`
was written for the human/planner decision.

Calibration outcome:

- `comments-2021-01-000`: pass (precision `87.3%`->`88.1%`, recall `55.0%`->`59.0%`)
- `comments-2021-01-001`: fail (precision `79.4%`->`81.6%`, recall `27.6%`->`31.6%`, gate `50%`)

No frozen validation shard (`002` or `003`) was accessed in any form.

## Files Changed

- `internal/config/relevance.go`
  - Added `ProximityRule` struct.
  - Added `ProximityRules` field to `RelevanceDomain`.
  - Added `validateProximityRules` and `validateProximityTerms` helpers.
  - Validates rule names (present, unique per domain), non-empty anchors and
    evidence, duplicate-free terms, `window_tokens` within `[1, 50]`, and
    `weight` within `(0, 1]`.
- `internal/config/relevance_test.go`
  - Added `TestLoadRelevanceV4DefinesProximityRules`.
  - Added `TestRelevanceConfigRejectsInvalidProximityRules` (10 subtests).
  - Added `TestRelevanceConfigRejectsDuplicateProximityRuleNames`.
- `internal/relevance/scorer_test.go`
  - Added `TestProximityBoostFiresForNearAnchorAndEvidence` (near pair retains
    with `proximity:` reason; far-apart and generic-mention do not).
  - Added `TestDeterministicV4RetainsProximityEvidenceWithoutTrapLeakage`
    (hostel-theft, border-security, switch-failure retain via proximity; generic
    app mention and political H1B do not).
  - Added `createProximityFixture` and `createDeterministicV4Fixture` helpers.
- `scripts/dev/duckdb_score_candidates.py`
  - Added `_tokenize`, `_term_positions`, `_proximity_match` helpers.
  - Added `make_proximity_rule_udf` and `register_proximity_udfs`.
  - `build_domain_query` now accepts a domain index and emits per-rule
    `proximity_<i>_<j>(candidate_match_text)` score terms and
    `proximity:<rule-name>` decision reasons.
  - `score` registers proximity UDFs before building domain queries.
  - Existing output schema, tiers, decisions, and Parquet columns unchanged.
- `configs/relevance/deterministic-v4.yaml` (new)
  - Copies V3 as the starting point, sets `version: deterministic_v4`.
  - Adds three general proximity rules: `travel_safety_loss`,
    `travel_border_security`, `app_failure_evidence`.
  - No source IDs, shard-specific subreddit allowlists, or fixture-specific
    sentences encoded.
- `docs/reports/deterministic-v4-calibration-2026-06-18.md` (new)
  - Calibration report with commands, config checksum, per-fixture metrics,
    combined metrics, corrected errors, remaining errors, decision, and the
    exact human/planner decision needed.
- `.agent/tasks/2026-06-18-deterministic-v4-proximity-calibration/CHANGE_REQUEST.md` (new)
  - Documents the label-boundary conflict and three recommended follow-up
    directions.

## Commands Run

```bash
go test ./internal/config ./internal/relevance ./cmd/score-candidates ./cmd/evaluate-relevance
go test ./...
go vet ./...
python3 -m py_compile scripts/dev/*.py
git diff --check

python3 scripts/dev/calibrate_relevance_fixtures.py \
  --relevance-config configs/relevance/deterministic-v3.yaml \
  --output-dir .tmp/relevance-v3-baseline-check \
  --fixture 'comments-2021-01-000|evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv|evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json' \
  --fixture 'comments-2021-01-001|evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv|evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json'

python3 scripts/dev/calibrate_relevance_fixtures.py \
  --relevance-config configs/relevance/deterministic-v4.yaml \
  --output-dir .tmp/relevance-v4-final \
  --fixture 'comments-2021-01-000|evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv|evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json' \
  --fixture 'comments-2021-01-001|evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv|evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json'
```

Config checksum:

- `configs/relevance/deterministic-v4.yaml`
  - `sha256:b9c86b936c7f6e532a19887635010012ad5ecfa12edf6b24a212a80ac0755c84`

## Tests

- `go test ./internal/config ./internal/relevance ./cmd/score-candidates ./cmd/evaluate-relevance` -> pass
- `go test ./...` -> pass
- `go vet ./...` -> clean
- `python3 -m py_compile scripts/dev/*.py` -> clean
- `git diff --check` -> clean

New focused tests:

- `TestLoadRelevanceV4DefinesProximityRules`
- `TestRelevanceConfigRejectsInvalidProximityRules` (10 subtests)
- `TestRelevanceConfigRejectsDuplicateProximityRuleNames`
- `TestProximityBoostFiresForNearAnchorAndEvidence`
- `TestDeterministicV4RetainsProximityEvidenceWithoutTrapLeakage`

## Calibration Outputs

V4 vs V3 (regenerated full-label fixtures):

| Fixture | V3 retained / precision / recall | V4 retained / precision / recall |
| :-- | :-- | :-- |
| `comments-2021-01-000` | `63` / `87.3%` / `55.0%` (pass) | `67` / `88.1%` / `59.0%` (pass) |
| `comments-2021-01-001` | `34` / `79.4%` / `27.6%` (fail) | `38` / `81.6%` / `31.6%` (fail) |

New retained true positives: 4 on `000`, 4 on `001` (8 total).
New retained false positives: 0 on both fixtures.
Trap leakage, visa FP, promotion/bot FP, and missing source URL counts unchanged
from V3 (all zero on both fixtures).

## Deviations From Plan

- The plan suggested a single `proximity_rules` schema with a combined
  `proximity_boost_<domain>` UDF. Implemented as per-rule UDFs
  (`proximity_<domain_index>_<rule_index>`) instead, so each rule emits its own
  `proximity:<rule-name>` decision reason following the same `CASE WHEN ... THEN
  ... ELSE NULL END` pattern as existing group/context/penalty reasons. This
  preserves explainability per requirement 3 and keeps the reasons SQL identical
  in shape to the existing implementation.
- A throwaway simulation script was used during calibration feasibility
  analysis to estimate proximity-rule effects before implementing the real
  scorer. It was written to the OS temp directory (`/var/folders/.../opencode/`),
  not the repo, and is not a deliverable. All reported calibration numbers come
  from the real `calibrate_relevance_fixtures.py` runner using the real scorer.
- The branch already existed and had diverged from `main` (main had the roadmap
  and onboarding course commits; the branch had the V3/V4 task work). Merged
  `main` into the branch at the start so the task could reference
  `docs/plans/argus-roadmap.md`. The merge was clean; all V3-related files were
  identical between branch and main.

## Known Risks

- The 001 recall gate is not met. The blocker is a label-boundary conflict on
  passport/citizenship text between the two training fixtures, not a scorer bug.
  See `CHANGE_REQUEST.md` for the exact decision needed.
- The proximity UDFs are Python functions registered with DuckDB. Performance is
  acceptable for the bounded calibration fixtures (hundreds of rows, three
  domains, three rules). For full-month ingestion scale this would need
  profiling, but full-month ingestion is out of scope and not promoted by this
  task.
- V4 is not promoted as any default scorer. The CLI default remains V2.

## Next Steps

1. Reviewer creates `REVIEW.md` and either approves the mixed-calibration
   result or requests changes.
2. Human/planner decides among the three options in `CHANGE_REQUEST.md`:
   label reconciliation, learned retrieval fallback, or frozen validation of
   the V4 mixed result.
3. Do not merge until review approval. Do not promote V4 as default. Do not
   access frozen shards `002` or `003` until a separate validation task.
