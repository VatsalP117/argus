# Implementation Plan

## 1. Task Summary

Build a reviewable `deterministic_v3` calibration candidate using the two
already-observed comment-shard fixtures. The work should correct the dominant v2
false-positive modes and recover concrete travel/app/workflow evidence from the
evaluate and discard strata while preserving deterministic, explainable scoring.

This PR ends after training calibration. It must not inspect the two frozen
validation shards or promote v3 as the default.

## 2. Current System Understanding

- Candidate scanning is broad and subreddit membership is only a relevance
  feature.
- `duckdb_score_candidates.py` scores each candidate independently for travel,
  SaaS opportunity, and app opportunity using:
  - matched rule-group weights
  - exact/context text boosts
  - context penalties
  - required terms or groups
  - subreddit prior
- Scores map to deterministic A/B/C/D tiers and retain/evaluate/discard
  decisions.
- `evaluate-relevance` now derives populations and strata from score Parquet,
  requires all score-derived retained rows to be labelled, and validates exact
  candidate/domain cardinality.
- The original `comments-2021-01-000` fixture was used to tune v2. The adjacent
  `comments-2021-01-001` fixture subsequently failed and is now observed, so
  both fixtures are training material for v3.
- `comments-2021-01-002` and `comments-2021-01-003` are reserved for later,
  independent validation.
- Existing labels are engineering labels, not independent human ground truth.
  Ambiguous examples need a documented spot-check before using them to justify
  narrow rule changes.

## 3. Scope

### In Scope

- Declare and enforce the training/validation split in task documentation and
  the calibration report.
- Add reproducible tooling to score and evaluate v3 against both observed
  fixtures.
- Add deterministic scorer capabilities only if a general rule cannot be
  expressed safely with the current configuration schema.
- Add `deterministic-v3.yaml`.
- Add focused unit/integration tests for new behavior.
- Calibrate against both observed fixtures.
- Document per-fixture and combined results, corrected examples, remaining
  errors, and the go/no-go decision for unseen validation.

### Out of Scope

- Reading, scanning, scoring, or labelling shard 002 or 003.
- Changing CLI defaults from v2 to v3.
- Durable DuckDB commit or cleanup lifecycle.
- Full-month ingestion.
- LLM classification, embeddings, semantic retrieval, or new dependencies.
- Database migrations or schema changes.
- Broad candidate scanner changes unless a demonstrated training false negative
  is absent from candidate Parquet entirely. That condition requires a
  `CHANGE_REQUEST.md`.

## 4. Proposed Technical Approach

### Calibration protocol

Create one reproducible calibration entry point, preferably a small script under
`scripts/dev/`, that:

1. accepts explicit candidate/checkpoint inputs for the observed fixtures
2. scores each fixture with an explicit relevance config
3. evaluates each metadata-bearing fixture with the standard quality gate
4. emits machine-readable per-fixture results
5. never overwrites labels or annotations
6. writes generated score files only under ignored temporary paths

The older shard-000 fixture may need regeneration with population metadata
before weighted metrics are available. Regeneration must preserve the existing
source-ID sample and labels, or the executor must document why an exact
population-aware reconstruction is impossible. Do not silently compare
incompatible metrics.

### Scorer changes

Prefer configuration-only changes. If the current additive term model cannot
express a general rule, make the smallest schema extension needed and cover it
with validation and scorer tests. Acceptable examples include:

- requiring evidence from two independent groups for high-confidence retention
- requiring a product/tool noun near concrete failure or workaround language
- applying generic promotion/referral penalties across SaaS and app domains
- applying travel ambiguity penalties unless personal process/action evidence
  is also present

Do not add source-specific allowlists/denylists or memorize complete fixture
sentences.

### Error-mode strategy

- Travel precision:
  - penalize policy-only or political immigration discussion
  - require stronger personal travel/process context for ambiguous terms such
    as itinerary, customs, passport, airline, and immigration
  - preserve concrete embassy, visa appointment/rejection, border, booking,
    luggage, accommodation, and transit pain
- App precision:
  - distinguish concrete software behavior from generic uses of app, sync,
    extension, notification, and software
  - require a product/tool indicator plus failure, workaround, request, or
    comparison evidence for retention
- SaaS precision:
  - preserve concrete professional workflows, manual work, spreadsheets,
    reporting, compliance, and fragmented tools
  - suppress promotions, consumer-only generic products, and referral language
- Recall:
  - use labelled evaluate/discard positives to add general concrete-problem
    patterns
  - avoid solving recall by globally lowering B/C thresholds

## 5. Step-by-Step Execution Plan

1. Establish the baseline.
   - Run `go test ./...`.
   - Reproduce v2 metrics for both observed fixtures where compatible.
   - Record candidate and score input checksums used for calibration.
   - Confirm no command references or opens frozen shards 002 or 003.

2. Add focused characterization tests.
   - Convert representative false positives and false negatives from the
     reports into small synthetic scorer fixtures.
   - Cover travel political/lexical ambiguity, promotion/referral content,
     generic app mentions, concrete broken-app behavior, concrete business
     workflow pain, and concrete travel-process pain.
   - Ensure tests assert decisions and decision reasons, not only scores.

3. Add calibration tooling.
   - Implement an explicit two-fixture calibration runner or documented command
     sequence.
   - Keep inputs explicit; do not discover arbitrary shards by wildcard.
   - Produce JSON suitable for report generation and review.
   - Fail on missing labels, score cardinality errors, or incompatible fixture
     metadata.

4. Create the initial v3 config.
   - Copy v2 to a new versioned file.
   - Change only rules justified by grouped training errors.
   - Keep tier changes conservative and separately documented.
   - Preserve all v2 signal mappings unless a tested reason requires change.

5. Iterate with bounded calibration.
   - Score both observed fixtures after each coherent rule set.
   - Track per-fixture precision, recall estimate, domain precision, retained
     count, and trap leakage.
   - Prefer changes that improve both fixtures.
   - Revert changes that improve only one fixture through obvious
     over-specialization.

6. Perform the engineering label spot-check.
   - Select a deterministic 50-row review set from ambiguous training examples:
     20 retained false positives, 15 evaluate/discard positives, and 15
     boundary/contradictory cases across both fixtures.
   - Produce a review CSV with excerpts and backlinks.
   - Do not auto-change labels. Record any proposed corrections separately in
     the report.
   - If label corrections materially affect acceptance, stop for human review
     rather than declaring success.

7. Publish the calibration report.
   - Include training split and frozen validation split.
   - Include v2 versus v3 metrics per fixture and combined where statistically
     valid.
   - Include exact configuration checksum and commands.
   - Include representative corrected and remaining errors with backlinks.
   - State either `ready_for_unseen_validation` or `failed_calibration`.
   - Never state that v3 is production-ready or promoted.

8. Update narrow documentation.
   - Add the v3 training result to the phase-8 runbook or product plan.
   - Keep v2 as the documented default.
   - Name the next task: validate the frozen v3 config on shard 002, followed by
     shard 003 only if shard 002 passes.

9. Final verification and report.
   - Run focused relevance/config tests.
   - Run calibration commands from a clean temporary output directory.
   - Run `go test ./...`, `go vet ./...`, Python compilation, and
     `git diff --check`.
   - Complete `IMPLEMENTATION_REPORT.md`.
   - Create small, coherent commits.

## 6. Test Plan

### Focused tests

- Config validation accepts v3 and rejects invalid new rule fields.
- Genuine work-visa/embassy/booking pain remains eligible.
- Payment-brand Visa and political-only immigration do not retain.
- Ambiguous customs/itinerary/airline mentions do not retain without personal
  travel/process evidence.
- Concrete broken-app and workaround examples retain.
- Generic app/product mentions do not retain.
- Referral and affiliate promotions do not retain.
- Concrete manual business workflow pain remains eligible.
- Decision reasons contain the rule or penalty responsible for the outcome.
- Calibration runner is deterministic and refuses missing/incompatible inputs.

### Regression checks

```bash
go test ./internal/config ./internal/relevance ./cmd/score-candidates ./cmd/evaluate-relevance
go test ./...
go vet ./...
python3 -m py_compile scripts/dev/*.py
git diff --check
```

### Calibration verification

- Run v3 against both training fixtures using explicit paths.
- Re-run and confirm identical source IDs, counts, and metrics.
- Confirm generated output paths and logs contain no references to shard 002 or
  shard 003.

## 7. Acceptance Criteria

- All required task artifacts are complete.
- `deterministic-v3.yaml` loads through the production config loader.
- New behavior is covered by focused tests.
- Calibration is reproducible with explicit commands.
- Each observed fixture meets all targets in `TASK.md`, or the report clearly
  records `failed_calibration`.
- No frozen validation shard was accessed.
- V2 remains the CLI and durable-commit default.
- No source IDs or fixture-specific long phrases are embedded in scoring code or
  configuration.
- No unrelated files or behavior changed.
- Full verification passes.
- `IMPLEMENTATION_REPORT.md` contains commands, results, deviations, and risks.

## 8. Risks and Guardrails

- Overfitting is the primary risk. Require improvements across both observed
  fixtures and reject source-specific rules.
- Agent-reviewed labels may be wrong. Keep the deterministic review sample and
  require human confirmation before changing labels that affect acceptance.
- Weighted recall on shard 000 may not be available from its legacy fixture.
  Do not fabricate population weights; either regenerate metadata compatibly or
  report exact metrics separately.
- A global threshold reduction can inflate recall while destroying precision.
  Treat broad tier changes as an escalation point.
- If candidate scanning excluded known relevant rows before scoring, this task
  cannot fix them. Write `CHANGE_REQUEST.md` for scanner work.
- Any need for a new dependency, database migration, public API change, or broad
  scorer redesign is an escalation trigger.

## 9. Executor Instructions

1. Read `TASK.md`, this plan, and the canonical product plan before editing.
2. Verify the current branch is `agent/deterministic-v3-calibration`.
3. Do not access shard 002 or 003, including through wildcard commands.
4. Use test-first changes for scorer capabilities and error-mode fixtures.
5. Prefer configuration changes over code changes.
6. Commit coherent progress in small, passing commits.
7. If the plan cannot meet its targets without a major redesign or label
   changes, write `CHANGE_REQUEST.md` and stop.
8. Complete `IMPLEMENTATION_REPORT.md` before opening the PR.
9. Do not merge the PR. The reviewer will create `REVIEW.md` after examining the
   review package against `main`.

