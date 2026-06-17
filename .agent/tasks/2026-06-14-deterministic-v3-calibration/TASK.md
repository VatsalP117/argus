# Deterministic V3 Calibration Task

## Objective

Create and calibrate `deterministic_v3` using only already-observed relevance
fixtures. Improve generalization across travel, SaaS opportunity, and app
opportunity retrieval without inspecting or scoring the reserved validation
shards.

## Background

`deterministic_v2` performed well on its original calibration fixture but failed
on the adjacent `comments-2021-01-001` shard:

- exact retained precision: 54.9% (required: at least 70%)
- weighted retained recall estimate: 2.1% (required: at least 60%)
- travel retained precision: 56.0%
- app retained precision: 45.8%
- two retained promotion or bot false positives
- lexical ambiguity represented 21.6% of retained rows

The failed shard has now been observed and may be used as v3 training data.

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

Do not scan, score, sample, inspect, label, or otherwise use either frozen shard
in this task. Their first use belongs in a separate validation task and PR after
the v3 configuration is frozen.

## Required Outcomes

1. Add a versioned `configs/relevance/deterministic-v3.yaml`.
2. Add focused scorer/config tests for each new rule behavior.
3. Add a reproducible training-fixture calibration command or script that
   evaluates v3 across both observed fixtures without mutating source labels.
4. Publish a v3 calibration report with per-fixture and combined metrics,
   representative corrected errors, remaining errors, and exact commands.
5. Update only the documentation needed to describe v3 as a calibration
   candidate. Do not promote v3 to the default scorer in this task.
6. Preserve explainability through decision reasons for every added boost,
   requirement, or penalty.

## Training Acceptance Targets

These are necessary but not sufficient for later promotion:

- exact retained precision at least 75% on each observed fixture
- weighted retained recall estimate at least 50% on each metadata-bearing
  fixture
- per-domain retained precision at least 65% when the fixture has at least 10
  retained predictions for that domain
- zero retained payment-brand Visa false positives
- zero retained promotion or bot false positives
- zero retained rows with missing source URLs
- no false-positive category above 20% of retained rows
- no regression below v2's 85.1% exact retained precision on
  `comments-2021-01-000`

If these targets cannot be met without fixture-specific memorization, threshold
collapse, or a scorer capability change, stop and write `CHANGE_REQUEST.md`.

## Constraints

- Work only on branch `agent/deterministic-v3-calibration`.
- Do not change database schema, migrations, auth, secrets, infrastructure, or
  public API contracts.
- Do not change the durable default from v2 to v3.
- Do not commit candidates to DuckDB or delete staging.
- Do not run a full month.
- Do not add an LLM, embedding model, classifier, or dependency.
- Do not encode source IDs, subreddit names from individual examples, or long
  fixture-specific sentences as scoring rules.
- Do not modify existing labels merely to improve metrics. Any label correction
  requires an explicit rationale in the calibration report and an annotation
  change that can be reviewed independently.
- Keep commits small and buildable.

## Deliverables

- v3 relevance configuration and focused tests
- reproducible two-fixture calibration tooling
- calibration report
- completed `IMPLEMENTATION_REPORT.md`
- passing focused tests and `go test ./...`
