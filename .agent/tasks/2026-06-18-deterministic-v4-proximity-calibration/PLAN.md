# Implementation Plan

## 1. Task Summary

Build a bounded `deterministic_v4` calibration candidate for the retrieval
quality gate. The work should add a small, explainable proximity-aware
conjunction rule capability, use it to improve recall on the already-observed
fixtures, and produce a clear decision about whether deterministic retrieval
should proceed to frozen validation or stop in favor of learned reranking.

This task is planning for implementation only. The executor should not inspect
or use frozen shards `comments-2021-01-002` or `comments-2021-01-003`.

## 2. Current System Understanding

- Argus is local-first and DuckDB-centered.
- The current product bottleneck is retrieval quality, not storage, query
  plumbing, or LLM integration.
- Candidate scanning already produces broad candidates with matched terms and
  matched rule groups.
- `scripts/dev/duckdb_score_candidates.py` scores each candidate per domain
  with:
  - rule-group weights
  - global context boosts
  - global context penalties
  - required terms or groups
  - minimum group counts
  - subreddit prior
- The scorer is additive and whole-text oriented. It cannot currently express
  "anchor term appears near problem/workaround/request language" without adding
  broad phrase boosts or penalties.
- `scripts/dev/calibrate_relevance_fixtures.py` reconstructs candidate fixtures
  from approved labels, scores them with an explicit relevance config, reapplies
  annotations, and evaluates only the approved observed fixtures.
- `deterministic_v3` passed `comments-2021-01-000` but failed recall on
  `comments-2021-01-001`, so v4 should test a more expressive deterministic
  rule model rather than keep stretching global additive weights.

## 3. Scope

### In Scope

- Add a compact config schema for proximity-aware conjunction rules.
- Validate the new config fields in `internal/config/relevance.go`.
- Implement proximity matching in `scripts/dev/duckdb_score_candidates.py`.
- Add focused scorer tests that prove proximity rules can boost true
  domain-specific evidence without boosting far-apart or generic mentions.
- Add `configs/relevance/deterministic-v4.yaml`.
- Calibrate v4 only against `comments-2021-01-000` and
  `comments-2021-01-001`.
- Publish a calibration report under `docs/reports/`.
- Update roadmap or runbook text only if needed to reflect the v4 result.

### Out of Scope

- Inspecting or validating on shards `002` or `003`.
- Promoting v4 as the default scorer.
- Full-month ingestion or durable DuckDB mutation.
- Candidate-scanner changes.
- Database migrations or durable schema changes.
- LLM classification, embeddings, vector search, or new dependencies.
- Broad refactors of scoring, evaluation, or config loading.
- Label rewrites for metric improvement.

## 4. Proposed Technical Approach

Add a new optional field to each relevance domain, tentatively named
`proximity_rules`.

Each proximity rule should be general, compact, and explainable. A suggested
schema:

```yaml
proximity_rules:
  - name: travel_process_problem
    anchors:
      - passport
      - embassy
      - hostel
      - booking
    evidence:
      - rejected
      - canceled
      - stolen
      - blocked
      - appointment
    window_tokens: 12
    weight: 0.20
```

Semantics:

- Normalize candidate text similarly to existing text matching.
- A rule matches when any `anchor` appears within `window_tokens` tokens of any
  `evidence` term.
- Matched rules add `weight` to the domain score.
- Decision reasons include `proximity:<rule-name>`.
- Rules should be boosts first, not hidden eligibility gates, unless testing
  shows a very small `required_proximity_rules` style extension is necessary.

Implementation detail:

- Keep the matching logic in Python where scoring already happens.
- Avoid new packages. A simple token-position helper with regex tokenization is
  enough.
- Register a DuckDB scalar UDF from Python if that keeps SQL readable. If a UDF
  is awkward, compute rule matches in SQL only if the implementation remains
  clear and tested.
- Preserve the existing output schema so downstream export, evaluation, commit,
  and reports keep working.

Calibration strategy:

- Start from `deterministic-v3.yaml`.
- Add proximity boosts for repeated, general false-negative patterns from the
  v3 report:
  - concrete travel process/safety evidence near travel anchors
  - product/tool anchors near failure/workaround language
  - business workflow anchors near manual/reporting/compliance pain
- Avoid lowering B/C thresholds unless the report justifies it explicitly.
- Prefer rules that improve both observed fixtures or improve recall without
  creating new trap leakage.

## 5. Step-by-Step Execution Plan

1. Prepare the branch and task context.
   - Create or switch to `agent/deterministic-v4-proximity-calibration`.
   - Read `TASK.md`, this plan, the canonical roadmap, the v3 report, and the
     v3 `CHANGE_REQUEST.md`.
   - Confirm the working tree and note unrelated untracked files.

2. Establish baseline checks.
   - Run focused tests:
     `go test ./internal/config ./internal/relevance ./cmd/score-candidates ./cmd/evaluate-relevance`
   - Run the v3 calibration command from
     `docs/reports/deterministic-v3-calibration-2026-06-17.md` into a fresh
     ignored temporary directory.
   - Confirm the reproduced outcome still matches the report closely enough to
     calibrate from it.

3. Add config schema support.
   - Add Go structs for proximity rules under `RelevanceDomain`.
   - Validate:
     - rule name is present and unique per domain
     - `anchors` and `evidence` are non-empty
     - terms are non-empty and duplicate-free within each list
     - `window_tokens` is positive and bounded, suggested maximum `50`
     - `weight` is within `(0, 1]`
   - Add config tests for valid v4 config and invalid proximity rules.

4. Add scorer behavior test first.
   - Add synthetic candidates showing:
     - near anchor/evidence pair retains or receives the proximity reason
     - far-apart anchor/evidence pair does not receive the proximity reason
     - generic product mention without nearby failure does not retain
     - concrete travel/app/workflow pain with nearby evidence can retain
   - Assert decisions and `decision_reasons`, not only counts.

5. Implement proximity scoring.
   - Parse proximity rules from the JSON config passed into
     `duckdb_score_candidates.py`.
   - Add deterministic token-window matching.
   - Add matched proximity boosts into the existing score expression.
   - Add `proximity:<rule-name>` to `decision_reasons`.
   - Keep existing score tiers, decisions, and Parquet columns unchanged.

6. Create `deterministic-v4.yaml`.
   - Copy v3 as the starting point.
   - Set `version: deterministic_v4`.
   - Add only a small number of proximity rules justified by grouped v3
     remaining errors.
   - Do not embed source IDs or full fixture sentences.

7. Run bounded calibration iterations.
   - Use only:
     - `comments-2021-01-000`
     - `comments-2021-01-001`
   - Write outputs under `.tmp/relevance-v4-*`.
   - Track exact retained precision, weighted recall, per-domain precision,
     trap counts, retained false-positive mix, and representative remaining
     false negatives.
   - Stop after a small number of coherent iterations if the design is not
     moving the gate.

8. Decide and document.
   - If v4 passes observed calibration, write report status
     `ready_for_frozen_validation` and name the next task as validation on
     shard `002`, then `003` only if `002` passes.
   - If v4 improves but misses narrowly, write report status
     `mixed_calibration` and identify the exact human/planner decision needed.
   - If v4 materially misses, write report status `failed_calibration` and
     recommend activating the learned retrieval fallback roadmap.

9. Final verification.
   - Run:
     `go test ./internal/config ./internal/relevance ./cmd/score-candidates ./cmd/evaluate-relevance`
   - Run `go test ./...`
   - Run `go vet ./...`
   - Run Python compilation for changed scripts, or `python3 -m py_compile scripts/dev/*.py`.
   - Run `git diff --check`.
   - Confirm no command, artifact, or report uses frozen shards `002` or `003`.

10. Complete executor artifacts.
   - Write `IMPLEMENTATION_REPORT.md`.
   - Include files changed, commands run, calibration outputs, deviations, risks,
     and next steps.
   - If the plan becomes wrong, write `CHANGE_REQUEST.md` instead of improvising
     a larger redesign.

## 6. Test Plan

Focused tests:

- `LoadRelevanceConfig` accepts `deterministic-v4.yaml`.
- Config validation rejects missing proximity names, empty anchors/evidence,
  duplicate rule names, invalid windows, and invalid weights.
- Scorer emits `proximity:<rule-name>` when anchor and evidence terms are within
  the configured token window.
- Scorer does not emit proximity reasons when terms are outside the window.
- A concrete travel process/safety candidate can retain because of a proximity
  boost.
- A generic app/product mention does not retain merely because product and pain
  terms both exist far apart.
- Existing v3 trap examples remain controlled.

Regression checks:

```bash
go test ./internal/config ./internal/relevance ./cmd/score-candidates ./cmd/evaluate-relevance
go test ./...
go vet ./...
python3 -m py_compile scripts/dev/*.py
git diff --check
```

Calibration command:

```bash
python3 scripts/dev/calibrate_relevance_fixtures.py \
  --relevance-config configs/relevance/deterministic-v4.yaml \
  --output-dir .tmp/relevance-v4-final \
  --fixture 'comments-2021-01-000|evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv|evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json' \
  --fixture 'comments-2021-01-001|evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv|evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json'
```

## 7. Acceptance Criteria

- Task artifacts exist and are complete.
- v4 config loads through the production config loader.
- Proximity scoring is deterministic, tested, and explainable.
- Existing score output schema remains compatible with export and evaluation.
- Calibration report includes exact commands and config checksum.
- Observed-fixture result is clearly classified as
  `ready_for_frozen_validation`, `mixed_calibration`, or
  `failed_calibration`.
- No frozen validation shard was accessed.
- v2 remains the default scorer in CLI and docs unless a doc explicitly says v4
  is only a calibration candidate.
- No unrelated refactors or broad architecture changes are included.
- Verification commands pass, or failures are documented with exact causes.

## 8. Risks and Guardrails

- Overfitting is the main risk. Keep rules general and pattern-based.
- Proximity rules can quietly become fixture-shaped. Reject long phrases,
  source IDs, and one-off sentence fragments.
- Tokenization choices affect behavior. Keep them simple, documented in tests,
  and stable.
- A Python UDF may affect performance. This is acceptable for bounded scoring
  if tests pass, but report any noticeable slowdown.
- If candidate scanning failed to include relevant rows, v4 scoring cannot fix
  them. Write `CHANGE_REQUEST.md` for scanner work.
- If v4 requires many special rules to pass, treat that as deterministic ceiling
  evidence and recommend learned reranking.
- Any need for a new dependency, schema migration, public API change, or broad
  scorer rewrite is an escalation trigger.

## 9. Executor Instructions

1. Work on `agent/deterministic-v4-proximity-calibration`.
2. Read `TASK.md`, this plan, `docs/plans/argus-roadmap.md`,
   `docs/reports/deterministic-v3-calibration-2026-06-17.md`, and the v3
   `CHANGE_REQUEST.md` before editing.
3. Do not access shards `002` or `003` in any form.
4. Use test-first changes for config validation and scorer behavior.
5. Keep proximity support small and preserve downstream output compatibility.
6. Calibrate only with explicit observed fixture paths.
7. Stop and write `CHANGE_REQUEST.md` if success requires a major scorer
   redesign, threshold collapse, label changes, scanner changes, or learned
   retrieval.
8. Complete `IMPLEMENTATION_REPORT.md` before review.
9. Do not merge. A reviewer must create `REVIEW.md` and approve before merge.

