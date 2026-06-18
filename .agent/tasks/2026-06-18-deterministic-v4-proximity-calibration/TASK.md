# Deterministic V4 Proximity Calibration Task

## Objective

Create and calibrate `deterministic_v4` using only the already-observed
relevance fixtures. Add a constrained, explainable scorer capability for
proximity-aware conjunction evidence so Argus can test whether deterministic
retrieval can recover more relevant evidence without collapsing precision.

This is the active retrieval-quality gate in the canonical roadmap.

## Background

`deterministic_v3` improved trap handling but failed observed calibration on the
adjacent training fixture:

- `comments-2021-01-000`: pass
- `comments-2021-01-001`: fail
  - exact retained precision: 79.4%
  - regenerated full-label recall estimate: 27.6%

The v3 change request concluded that more global additive boosts and penalties
are likely near their limit. The approved follow-up direction is one bounded
deterministic experiment: add proximity-aware conjunction rules, calibrate only
on observed fixtures `000` and `001`, and decide whether deterministic scoring
is still worth pursuing.

## Training Inputs

- `comments-2021-01-000`
  - `evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv`
  - `evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json`
- `comments-2021-01-001`
  - `evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv`
  - `evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json`

## Frozen Validation Inputs

The following shard identities are validation-only:

- `comments-2021-01-002`
- `comments-2021-01-003`

Do not scan, score, sample, inspect, label, query, or otherwise use either
frozen shard in this task. Their first use belongs in a separate validation task
after `deterministic_v4` is frozen.

## Required Outcomes

1. Add a versioned `configs/relevance/deterministic-v4.yaml`.
2. Add the smallest scorer/config schema extension needed for proximity-aware
   conjunction evidence.
3. Preserve explainability by emitting decision reasons for every conjunction
   boost or requirement that affects a score.
4. Add focused tests for config validation and scorer behavior.
5. Reuse the existing observed-fixture calibration runner unless a small,
   justified extension is required.
6. Publish a v4 calibration report with commands, config checksum, per-fixture
   metrics, combined metrics, corrected errors, remaining errors, and a clear
   decision.
7. Do not promote v4 to any default scorer in this task.

## Calibration Acceptance Targets

These targets apply to each observed fixture unless explicitly noted:

- exact retained precision at least 75%
- weighted retained recall estimate at least 50% on metadata-bearing fixtures
- per-domain retained precision at least 65% when a fixture has at least 10
  retained predictions for that domain
- zero retained payment-brand Visa false positives
- zero retained promotion or bot false positives
- zero retained rows with missing source URLs
- no false-positive category above 20% of retained rows
- no regression below v3's exact retained precision on
  `comments-2021-01-000`

If these targets cannot be met without fixture-specific memorization, threshold
collapse, or awkward rule growth, stop and write `CHANGE_REQUEST.md`.

## Constraints

- Work only on branch `agent/deterministic-v4-proximity-calibration`.
- Do not change database schema, migrations, auth, secrets, infrastructure, or
  public API contracts.
- Do not change CLI defaults from v2 to v4.
- Do not mutate durable DuckDB, commit candidates, or delete staging.
- Do not run a full month.
- Do not add an LLM, embedding model, classifier, vector database, or new
  dependency.
- Do not encode source IDs, shard-specific subreddit allowlists, or long
  fixture-specific sentences as scoring rules.
- Do not modify existing labels just to improve metrics. Any label issue must
  be documented separately and escalated for human review if it affects the
  gate.
- Keep commits small, focused, and buildable.

## Deliverables

- `deterministic_v4` relevance configuration
- proximity/conjunction scorer support with focused tests
- observed-only calibration report
- completed `IMPLEMENTATION_REPORT.md`
- passing focused tests, `go test ./...`, Python compilation, and
  `git diff --check`

