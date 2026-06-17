# Deterministic V4 Proximity Calibration Task

## Objective

Create a bounded `deterministic_v4` relevance candidate by adding one more
expressive deterministic scorer capability: proximity-aware conjunction rules
for travel and app ambiguity. Recalibrate only on the already-observed fixtures
and determine whether this narrower scorer upgrade is enough to recover recall
without collapsing precision.

## Why This Task Exists

`deterministic_v3` proved that additive boosts, penalties, and
`minimum_group_matches` were not enough:

- `comments-2021-01-000`: pass
- `comments-2021-01-001`: fail
  - exact retained precision: `79.4%`
  - regenerated full-label recall estimate: `27.6%`

The v3 `CHANGE_REQUEST.md` concluded that further progress likely requires a
more expressive deterministic rule model rather than more config-only tuning.

## Chosen Follow-Up Direction

Pursue a single bounded scorer upgrade:

1. add proximity-aware conjunction rules
2. use them to reward concrete travel-process pain and concrete broken-app
   evidence
3. use them to suppress ambiguous policy/generic-product matches only where
   proximity evidence is absent

Do not broaden this into scanner changes, unseen validation, or a general
classifier project.

## Training Inputs

- `comments-2021-01-000`
  - `evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv`
  - `evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json`
- `comments-2021-01-001`
  - `evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv`
  - `evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json`

## Frozen Validation Inputs

The following remain validation-only and must not be accessed in this task:

- `comments-2021-01-002`
- `comments-2021-01-003`

## Required Outcomes

1. Add a versioned `configs/relevance/deterministic-v4.yaml`.
2. Extend the scorer/config schema with one bounded proximity-aware capability,
   no broader redesign.
3. Add focused tests that characterize proximity boosts and penalties plus
   decision reasons.
4. Reuse the bounded calibration runner to score and evaluate v4 on the two
   observed fixtures.
5. Publish a v4 calibration report with exact commands, metrics, corrected
   examples, remaining errors, and a clear decision.
6. Keep v2 and v3 defaults unchanged.

## Acceptance Targets

These targets apply to the observed training fixtures only:

- exact retained precision at least `75%` on each fixture
- weighted or regenerated full-label recall estimate at least `50%` on each
  fixture
- per-domain retained precision at least `65%` when retained count is at least
  `10`
- zero retained payment-brand Visa false positives
- zero retained promotion or bot false positives
- zero retained rows with missing source URLs
- no false-positive category above `20%` of retained rows
- no regression below v3 on `comments-2021-01-000`
- no frozen validation shard access

If the bounded proximity upgrade still fails these targets, stop and write a
new `CHANGE_REQUEST.md` rather than adding more ad hoc scorer complexity.

## Constraints

- Work on branch `agent/deterministic-v4-proximity-calibration`.
- Do not change database schema, migrations, auth, infra, secrets, or public
  API contracts.
- Do not change CLI defaults from v2 or promote v4 in this task.
- Do not mutate durable DuckDB data or run `commit-candidates`.
- Do not run a full month.
- Do not add dependencies.
- Do not touch candidate scanning.
- Do not inspect shards `002` or `003` directly or indirectly.
- Do not modify labels unless a separate human-reviewed label-correction task is
  explicitly approved.
- Keep commits small and buildable.

## Deliverables

- `deterministic-v4.yaml`
- bounded scorer/config changes for proximity rules
- focused scorer/config tests
- updated calibration artifacts and report
- completed `IMPLEMENTATION_REPORT.md`
- passing focused checks and `go test ./...`
