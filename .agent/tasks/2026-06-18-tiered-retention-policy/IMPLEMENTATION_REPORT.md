# Implementation Report

## Summary

Implemented the approved tiered retention policy: Argus now treats retention as
tiered (`A`/`B`/`C`/`D`) rather than a strict binary truth filter, and the
durable commit path commits only trusted `A`/`B` evidence by default while
offering an explicit `--include-review-tier` opt-in to also commit `C`-tier
review/exploration evidence without treating it as trusted.

No schema change and no broad pipeline rewrite were required. The existing
`document_relevance` schema already carries `relevance_tier`, `decision`, and
`decision_reasons`, and the scorer already emits the four tiers and three
decisions. The only binary-retention assumption in the commit path was a single
`decision = 'retain'` filter, which is now tier-aware.

## Policy

Tier meanings (documented in roadmap, product plan, and runbooks):

- `A`: strong evidence. Default for summaries and high-confidence answers.
- `B`: trusted retained evidence. Usable in research with evidence backing.
- `C`: weak/borderline review or exploration evidence. Stored only with explicit
  opt-in and not trusted by default.
- `D`: discard. Never retained.

Defaults:

- Durable commit commits only `A`/`B` (`decision = 'retain'`) by default.
- `C` (`decision = 'evaluate'`) is committed only behind an explicit
  `--include-review-tier` opt-in, as a review/exploration pool.
- Default research/query behavior uses `A`/`B` only. Exploratory or review
  workflows may explicitly opt into `C`.
- `D` is always excluded.

Quality gate interpretation (documented, not lowered):

- The existing evaluation gates (retained precision `>= 70%`, retained recall,
  zero payment-brand Visa false positives, zero promotion/bot false positives,
  lexical-ambiguity share `<= 20%`) still measure the `decision = 'retain'`
  trusted tier (`A`/`B`). They are not redefined.
- `C` is not judged by the trusted-tier gate. `C` exists for recall recovery,
  manual review, and future training data, not for trusted answers.
- This is not "v4 passed." It is "v4 is a useful tiered front door when its
  tiers are respected."

## Audit Findings

Binary `retain/evaluate/discard` assumptions were audited across commit, query,
export, scorer/evaluator, and runbooks.

- Scorer (`scripts/dev/duckdb_score_candidates.py`): already emits four tiers
  (`A`/`B`/`C`/`D`) and three decisions (`retain`/`evaluate`/`discard`) from the
  configured thresholds. No binary assumption; matches the policy exactly.
- Durable schema (`sql/migrations/001_initial_durable_schema.sql`):
  `document_relevance` already has `relevance_tier`, `decision`, and
  `decision_reasons`. No schema change needed.
- Durable commit (`scripts/dev/duckdb_commit_candidates.py`): the only binary
  assumption. The retained-candidate filter was
  `scores.decision = 'retain'`. Now tier-aware.
- Evaluation gate (`scripts/dev/duckdb_evaluate_relevance.py`): measures
  `decision = 'retain'` precision/recall. This is the trusted-tier quality gate
  and is intentionally preserved (not lowered).
- Query layer (`scripts/dev/duckdb_query_layer.py`): reads v0 clean/mart
  Parquet, which does not carry `relevance_tier`/`decision`. The tiered default
  is enforced at the durable commit gate; a follow-up task should add a
  tier-aware query surface over the durable `document_relevance` table.
- Evidence export (`scripts/dev/duckdb_export_evidence.py`): reads signal
  marts; no tier columns in scope. Documented as follow-up.
- Cleanup (`scripts/dev/duckdb_cleanup_staging.py`): reconciles by
  `ingest_batch_id`, not by decision. Consistent with tier-aware commit.
- Runbooks/roadmap/product plan: updated (see Files Changed).

## Files Changed

Code/config:

- `scripts/dev/duckdb_commit_candidates.py`
  - Added `include_review_tier` from commit metadata.
  - Replaced the hardcoded `decision = 'retain'` retained-candidate filter
    with `retain_decision_predicate`, which is
    `scores.decision IN ('retain', 'evaluate')` when `include_review_tier` is
    set and `scores.decision = 'retain'` otherwise.
  - Added `rows_review_tier` (count of retained documents whose strongest
    decision is `evaluate`) to both the new-commit and `skipped_existing`
    result paths.
  - Added a retention-scope guard in `existing_result`: when a validated batch
    already exists for the same `ingest_batch_id`, it recomputes the would-retain
    count under the requested scope from the score Parquet and raises a clear
    error if it differs from the stored `retained_rows`. This prevents
    `--include-review-tier` from being silently ignored on an already-committed
    batch. A retry with the same scope still returns `skipped_existing`
    (genuinely idempotent).
- `internal/candidate/committer.go`
  - Added `IncludeReviewTier` to `CommitOptions`.
  - Added `IncludeReviewTier` to `commitMetadata` (serialized to the Python
    adapter).
  - Added `RowsReviewTier` to `CommitResult`.
- `cmd/commit-candidates/main.go`
  - Added `--include-review-tier` flag (default `false`) and wired it into
    `candidate.CommitOptions`.

Tests:

- `internal/candidate/committer_test.go`
  - Added `TestCommitCandidatesTieredRetentionReviewTierOptIn`: builds a
    four-candidate fixture with one `C`-tier (`decision = 'evaluate'`) travel
    candidate, scores it, and commits twice into separate temporary DuckDB
    databases. Default commit excludes `C` (`rows_retained = 2`,
    `rows_review_tier = 0`, no `travel-evaluate` document). Opt-in commit
    includes `C` (`rows_retained = 3`, `rows_review_tier = 1`), preserves
    `relevance_tier = 'C'`, `decision = 'evaluate'`, and a non-empty
    `decision_reasons`, and reconciliation stays valid.
  - Added `TestCommitCandidatesReviewTierRetryOnDefaultBatchErrors`: commits
    default into a temp DB, then retries with `IncludeReviewTier = true` on the
    same batch and asserts the retry errors (not a silent `skipped_existing`
    with `rows_review_tier = 0`); asserts the durable corpus is unchanged; and
    asserts a same-scope retry remains idempotent (`skipped_existing`,
    unchanged counts).
  - Added `createTieredRetentionCandidateFixture` and `inspectRelevanceRow`
    helpers.

Docs:

- `docs/plans/argus-roadmap.md`: updated current retrieval state, added
  `Tiered retention policy` section, updated `Current Next Goal`.
- `docs/plans/local-market-research-product-plan.md`: clarified `C` tier
  meaning and added `Tiered retention and trust policy` section.
- `docs/runbooks/phase-8-durable-candidate-commit.md`: added tiered retention
  policy section and documented `--include-review-tier` commit behavior.
- `docs/runbooks/phase-6-query-layer.md`: added tiered retention and query
  defaults section with the documented follow-up.

## Commands Run

Focused tests:

```bash
go test ./internal/candidate ./cmd/commit-candidates ./cmd/query ./cmd/export-evidence
```

Result: `ok argus/internal/candidate`, `ok argus/cmd/query`; `cmd/commit-candidates`
and `cmd/export-evidence` have no test files.

New tiered retention test:

```bash
go test ./internal/candidate -run TestCommitCandidatesTieredRetention -v -count=1
```

Result: `PASS` (`TestCommitCandidatesTieredRetentionReviewTierOptIn`).

Full suite:

```bash
go test ./...
```

Result: all packages `ok` (no failures).

Vet:

```bash
go vet ./...
```

Result: clean (no output).

Python compile:

```bash
python3 -m py_compile scripts/dev/*.py
```

Result: `PY_COMPILE_OK`.

Whitespace:

```bash
git diff --check
```

Result: exit `0` (no whitespace errors).

## Tests

- `TestCommitCandidatesIsTransactionalReconciledAndIdempotent` (existing, still
  passing): proves default commit is transactional, reconciled, idempotent, and
  retains only `decision = 'retain'` candidates.
- `TestCommitCandidatesTieredRetentionReviewTierOptIn` (new):
  - Default commit excludes `C`-tier review evidence (`rows_retained = 2`,
    `rows_review_tier = 0`, `travel-evaluate` document absent).
  - Opt-in commit includes `C`-tier review evidence (`rows_retained = 3`,
    `rows_review_tier = 1`).
  - `C`-tier relevance row preserves `relevance_tier = 'C'`,
    `decision = 'evaluate'`, and a non-empty `decision_reasons`.
  - Source and staging reconciliation equations remain valid under opt-in.
  - No default scorer promotion or threshold change occurs (scorer config
    unchanged).
- `TestCommitCandidatesReviewTierRetryOnDefaultBatchErrors` (new):
  - Default commit into a temp DB succeeds; retrying with
    `IncludeReviewTier = true` on the same batch errors instead of silently
    skipping with `rows_review_tier = 0`.
  - The default-committed durable corpus is not mutated by the failed retry.
  - A same-scope retry remains idempotent (`skipped_existing`, unchanged
    `rows_retained` and `rows_review_tier`).

## Deviations From Plan

- The plan considered adding query/export guardrails directly. The query layer
  reads v0 clean/mart Parquet, which has no `relevance_tier`/`decision`
  columns, so a tier filter cannot be enforced there without a new query
  surface over the durable `document_relevance` table. Per the plan's
  "document the needed follow-up instead of overbuilding" instruction, this is
  documented in `phase-6-query-layer.md` rather than built in this task. The
  tiered default is enforced at the durable commit gate.
- No `CHANGE_REQUEST.md` was needed: no schema change or broad pipeline rewrite
  was required.

## Known Risks

- Storing `C` rows via `--include-review-tier` increases durable storage use.
  `C` is opt-in and must be measured in a bounded shard/month trial before
  wider use. The next task must report retained rows by tier, bytes per
  retained document, and query-quality impact.
- `--include-review-tier` cannot extend an already-committed batch. A retry
  against a validated batch with a different retention scope now raises a clear
  error (the would-retain count under the requested scope differs from the
  stored `retained_rows`); the operator must use a fresh manifest entry or drop
  the batch. A retry with the same scope remains idempotent
  (`skipped_existing`).
- The query layer does not yet enforce `A`/`B`-only defaults directly; it relies
  on the durable commit gate. A follow-up tier-aware query surface is needed
  before `C` is committed broadly.
- `C` rows are committed into the same `documents` table as `A`/`B` rows. They
  are distinguishable only via `document_relevance.decision`/`relevance_tier`.
  Consumers that ignore `decision` and treat all `documents` rows as trusted
  would include `C`. The runbook and product plan document that default
  research/query behavior must filter to `decision = 'retain'`.

## Next Steps

1. Bounded shard/month trial of tiered retention: run one bounded shard with
   `--include-review-tier`, then report retained rows by tier, bytes per
   retained document, and the effect of `C` on query results.
2. Add a tier-aware query surface over the durable `documents` /
   `document_relevance` tables with an explicit `--include-review-tier`/
   `--tiers` filter so the query layer enforces `A`/`B`-only defaults and an
   explicit `C` opt-in directly.
3. Do not widen `C`-tier durable commit until the trial report is reviewed.
