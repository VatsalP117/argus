# Implementation Plan

## 1. Task Summary

Add one narrow scorer capability for `deterministic_v4`: proximity-aware
conjunction rules that can express “travel anchor near process pain” and
“product anchor near broken/workaround evidence” without lowering global
thresholds. Use that capability to recalibrate on the two observed fixtures and
decide whether deterministic retrieval can still progress cleanly.

## 2. Current System Understanding

- Relevance scoring is currently implemented in
  `scripts/dev/duckdb_score_candidates.py` and configured through YAML loaded by
  `internal/config/relevance.go`.
- Current domain logic supports:
  - group weights
  - context boosts
  - context penalties
  - required terms
  - required groups
  - `minimum_group_matches`
  - subreddit prior
- `deterministic_v3` improved precision traps but still under-recovers concrete
  travel and app positives on `comments-2021-01-001`.
- The bounded calibration runner reconstructs candidates from the reviewed label
  fixtures and can be reused for v4 without touching DuckDB durable state.
- Frozen validation shards `002` and `003` remain off-limits until a later
  validation task.

## 3. Scope

### In Scope

- Add one bounded scorer capability for proximity-aware conjunction matching.
- Expose that capability through config validation and the DuckDB scorer.
- Create `configs/relevance/deterministic-v4.yaml`.
- Add focused tests for positive and negative proximity cases.
- Re-run bounded calibration on `000` and `001`.
- Write a v4 calibration report and implementation report.

### Out of Scope

- Candidate-scanner changes
- Durable DuckDB writes or cleanup lifecycle changes
- Validation on shards `002` or `003`
- LLMs, embeddings, classifiers, rerankers, or new dependencies
- Public API or CLI default changes
- Broad redesign of the scorer beyond the single bounded capability in this plan

## 4. Proposed Technical Approach

### Capability shape

Add a config-driven proximity rule list at the domain level. Keep it intentionally
small and explicit. A rule should be able to express:

- a human-readable rule name
- one set of left-hand terms
- one set of right-hand terms
- a bounded maximum word gap
- a positive weight or penalty

Recommended shape:

```yaml
proximity_rules:
  - name: travel_process_blocker
    left_terms: ["passport", "embassy", "hostel", "visa", "airline"]
    right_terms: ["blocked", "stolen", "canceled", "appointment", "rejected"]
    max_word_gap: 8
    weight: 0.25
```

The executor may choose a slightly different field naming scheme if it stays
equally narrow, explainable, and easy to validate.

### Matching model

Implement proximity matching in the scorer using bounded regex or equivalent SQL
text logic over `candidate_match_text`. Do not add Python per-row scoring or a
new parsing dependency. The rule must support either term order:

- left before right within the gap
- right before left within the gap

Emit a decision reason such as `proximity:travel_process_blocker` or
`proximity_penalty:generic_app_no_failure_anchor` so the added behavior remains
auditable.

### Intended usage

Use proximity boosts to recover concrete missed positives without globally
lowering thresholds:

- travel:
  - passport or visa near blocked/appointment/rejection language
  - hostel or airline near theft/cancelation/stranding/problem language
- app:
  - app/product/tool anchor near crash/fail/workaround/load/sync language

Use proximity penalties only where they are clearly general:

- generic product anchor without nearby failure/workaround evidence
- political immigration language without nearby personal travel/process evidence

Prefer a small number of high-signal rules over a long list of brittle phrases.

## 5. Step-by-Step Execution Plan

1. Establish the v3 baseline for comparison.
   - Run focused relevance/config tests and `go test ./...`.
   - Re-run the bounded calibration runner with `deterministic-v3.yaml`.
   - Record the reproduced metrics used as the v4 baseline.

2. Add characterization tests first.
   - Extend scorer tests with synthetic examples for:
     - travel anchor near process blocker retains
     - political immigration without travel-process proximity does not retain
     - app/tool anchor near broken/workaround evidence retains
     - generic product/tool text without nearby failure evidence does not gain
       the same boost
   - Assert both decision and emitted reason strings.

3. Extend the config schema and validation.
   - Add a bounded proximity rule structure in Go config types.
   - Validate:
     - non-empty rule name
     - non-empty left/right term lists
     - positive max gap
     - positive absolute weight within safe bounds
     - no duplicate rule names per domain

4. Extend the DuckDB scorer.
   - Generate SQL for proximity rule matches in either term order.
   - Support boost rules and penalty rules through the same bounded mechanism.
   - Append explainable decision reasons for matched proximity rules.
   - Keep the rest of the scoring path unchanged.

5. Create `deterministic-v4.yaml`.
   - Start from v3.
   - Add only the proximity rules justified by the observed misses/leaks.
   - Keep threshold changes conservative; avoid a broad tier collapse.

6. Run bounded calibration iterations.
   - Score and evaluate only fixtures `000` and `001`.
   - Track exact retained precision, recall estimate, per-domain precision,
     retained count, and trap leakage.
   - Favor rule changes that help both fixtures.
   - Revert any obvious overfitting.

7. Decide the outcome.
   - If v4 meets the task targets, publish the report as
     `ready_for_unseen_validation`.
   - If it still fails materially, stop and write a new `CHANGE_REQUEST.md`
     explaining why the bounded proximity slice was insufficient.

8. Final verification.
   - Run:
     - `go test ./internal/config ./internal/relevance ./cmd/score-candidates ./cmd/evaluate-relevance`
     - `go test ./...`
     - `go vet ./...`
     - `python3 -m py_compile scripts/dev/*.py`
     - `git diff --check`
   - Complete `IMPLEMENTATION_REPORT.md`.
   - Commit small, coherent increments.

## 6. Test Plan

### Focused Tests

- Config validation accepts valid proximity rules.
- Config validation rejects empty rule names, empty term lists, zero gap, and
  duplicate rule names.
- Travel proximity boost retains concrete passport or embassy process pain.
- Political immigration text without proximity corroboration does not retain.
- App proximity boost retains concrete broken-app/workaround evidence.
- Generic product mentions without nearby failure evidence do not get the same
  retain path.
- Decision reasons include the matched proximity rule name.

### Regression Checks

```bash
go test ./internal/config ./internal/relevance ./cmd/score-candidates ./cmd/evaluate-relevance
go test ./...
go vet ./...
python3 -m py_compile scripts/dev/*.py
git diff --check
```

### Calibration Verification

- Re-run the bounded calibration runner with `deterministic-v4.yaml`.
- Confirm outputs mention only fixtures `000` and `001`.
- Re-run once to verify deterministic metrics.

## 7. Acceptance Criteria

- A bounded proximity rule capability exists and is covered by focused tests.
- `deterministic-v4.yaml` loads via the production config loader.
- Calibration remains reproducible through explicit commands.
- The two observed fixtures meet the targets in `TASK.md`, or the task stops
  with a clear `CHANGE_REQUEST.md`.
- No frozen validation shard is accessed.
- No candidate scanner, default scorer selection, or durable DuckDB behavior is
  changed.
- `IMPLEMENTATION_REPORT.md` is complete.

## 8. Risks and Guardrails

- Overfitting remains the biggest risk. Keep the rule count low and the wording
  general.
- Regex-based proximity can get brittle if the executor tries to encode long
  phrases. Avoid sentence memorization.
- If the scorer needs tokenization, parser libraries, or candidate-scanner
  context expansion to make progress, stop and escalate instead of widening the
  task.
- If recall can only be recovered by broad threshold reduction, stop and record
  that explicitly.
- Do not inspect or use shards `002` or `003` in any command, wildcard, or
  output path.

## 9. Executor Instructions

1. Read `TASK.md`, this plan, and the prior v3 task artifacts before editing.
2. Stay on branch `agent/deterministic-v4-proximity-calibration`.
3. Do not access frozen shards `002` or `003`.
4. Implement the smallest proximity-rule capability that can express the target
   behaviors.
5. Prefer test-first changes for the new schema and scorer logic.
6. Reuse the existing calibration runner; do not invent a parallel workflow.
7. If bounded proximity rules still cannot get close without another redesign,
   write `CHANGE_REQUEST.md` and stop.
8. Do not merge the PR. Reviewer approval remains required.
