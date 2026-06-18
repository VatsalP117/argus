# Learned Retrieval Fallback Task

## Objective

Build a bounded learned retrieval fallback prototype that sits on top of the
existing broad candidate retrieval and deterministic v4 scoring outputs.

The goal is to test whether a lightweight local reranker/classifier can recover
the ambiguous `0.45-0.55` candidates that deterministic scoring cannot boost
without precision collapse.

This task should produce an observed-fixture result and a clear go/no-go
decision for later frozen validation. It must not touch frozen validation shards.

## Background

`deterministic_v4` added proximity-aware conjunction rules and improved both
observed fixtures after label reconciliation, but it still failed the recall
gate on `comments-2021-01-001`:

- `comments-2021-01-000`: pass
  - precision: `85.1%`
  - recall: `59.4%`
- `comments-2021-01-001`: fail
  - precision: `81.6%`
  - recall: `36.5%`
  - gate: `50%`

The remaining false negatives are not concentrated in the resolved
passport/citizenship boundary. They are mostly non-passport cases in the
`0.45-0.55` score range. Boosting that tier deterministically would also retain
labeled-negative candidates and collapse precision below the `75%` gate.

The roadmap says that when deterministic scoring stalls this way, Argus should
stop scoring-rule churn and plan a lightweight learned reranking or
classification layer.

## Training Inputs

Use only the reconciled observed fixtures:

- `comments-2021-01-000`
  - `evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv`
  - `evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json`
- `comments-2021-01-001`
  - `evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv`
  - `evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json`

## Frozen Validation Inputs

Do not inspect, scan, score, sample, label, train on, validate on, or otherwise
use:

- `comments-2021-01-002`
- `comments-2021-01-003`

Frozen validation belongs in a separate task only after the learned config/model
is frozen.

## Required Outcomes

1. Add a local learned reranking/classification prototype that uses existing
   fixture labels and v4 score outputs.
2. Avoid new infrastructure and new external services.
3. Prefer no new dependencies. If a dependency is absolutely required, stop and
   write `CHANGE_REQUEST.md`.
4. Generate reproducible observed-fixture evaluation outputs.
5. Compare learned retrieval against deterministic v4 on the same reconciled
   labels.
6. Publish a report with exact commands, metrics, feature/model description,
   failure modes, and go/no-go decision.
7. Do not promote the learned layer into durable ingest or default scoring.

## Acceptance Targets

Observed-fixture success requires:

- retained precision at least `75%` on each observed fixture
- retained recall at least `50%` on each observed fixture
- per-domain retained precision at least `65%` when a fixture has at least 10
  retained predictions for that domain
- zero retained payment-brand Visa false positives
- zero retained promotion or bot false positives
- zero retained rows with missing source URLs
- no false-positive category above `20%` of retained rows
- a documented improvement over deterministic v4 on `comments-2021-01-001`
  recall without precision collapse

If the learned prototype cannot meet these targets without obvious overfitting,
data leakage, or a more complex system, record `failed_experiment` and stop.

## Constraints

- Work only on branch `agent/learned-retrieval-fallback`.
- Do not access frozen shards `002` or `003`.
- Do not change database schema, migrations, auth, secrets, infrastructure, or
  public API contracts.
- Do not mutate durable DuckDB, commit candidates, delete staging, or run a full
  month.
- Do not add LLM classification, embeddings, a vector database, or online model
  calls.
- Do not change v4 scorer behavior unless a narrow bug is found and documented.
- Do not modify labels in this task.
- Do not train and evaluate on the same rows without a held-out or
  cross-validation discipline.
- Do not use source IDs, URLs, row order, fixture IDs, sampling metadata, or
  other leakage features as model inputs.

## Deliverables

- Learned retrieval prototype code and focused tests
- Observed-fixture evaluation report under `docs/reports/`
- Machine-readable output artifacts under an ignored temporary directory
- Completed `IMPLEMENTATION_REPORT.md`
- Passing focused checks, `go test ./...`, Python compilation, and
  `git diff --check`
