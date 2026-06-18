# Implementation Plan

## 1. Task Summary

Turn the product decision "some noise is acceptable if trust is explicit" into a
tiered retention policy and a small implementation slice. The target is not to
make retrieval perfect; it is to make the durable corpus useful, auditable, and
filterable by confidence.

This task should unblock bounded durable ingestion planning without promoting a
perfect binary scorer.

## 2. Current System Understanding

- Scoring already emits `relevance_score`, `relevance_tier`, `decision`, and
  `decision_reasons`.
- Current scorer decisions are binary-ish:
  - score `>= B threshold` -> `retain`
  - score `>= C threshold` -> `evaluate`
  - below `C` -> `discard`
- Durable commit currently appears to focus on retained candidates.
- Query and evidence workflows likely assume retained rows are the trusted
  research corpus.
- V4 showed `A/B`-style retained rows can be reasonably precise, while
  `C/evaluate` rows contain both useful missed evidence and noise.
- Learned retrieval showed that aggressively promoting the middle tier recovers
  recall but damages precision.
- Therefore, `C` should be stored/handled as a distinct low-confidence review
  tier, not merged into the trusted retained tier.

## 3. Scope

### In Scope

- Document tiered retention as the current product policy.
- Identify binary-retention assumptions in:
  - scorer output
  - candidate commit
  - evidence export
  - query defaults
  - runbooks/roadmap
- Add or adjust config/code so retained durable rows can include a controlled
  `C`/review tier while preserving `relevance_tier` and `decision_reasons`.
- Make default query/research behavior exclude `C` unless explicitly requested.
- Add focused tests for tier filtering and/or commit policy.
- Update reports/runbooks to explain that v4 is a tiered front door, not a
  perfect binary classifier.

### Out of Scope

- New scoring/model work.
- Frozen validation.
- Full-month ingest.
- Schema migrations unless the audit proves the current schema cannot represent
  tiered rows.
- Large UI or query UX redesign.
- Label changes.
- New sources beyond Reddit.

## 4. Proposed Technical Approach

Start with an audit, then pick the smallest implementation path.

Likely policy:

- Keep existing score tiers:
  - `A`: high-confidence evidence
  - `B`: trusted retained evidence
  - `C`: review/exploration evidence
  - `D`: discard
- Store `A/B` in durable corpus by default.
- Allow storing `C` only behind an explicit config/pipeline flag such as
  `include_review_tier`, `retain_evaluate_tier`, or `durable_review_tier`.
- Preserve `decision = evaluate` or introduce a clearly named retained status
  only if existing commit code requires `decision = retain`.
- Default query paths should filter to trusted evidence unless a caller opts
  into review-tier evidence.

Preferred implementation order:

1. Avoid schema changes if existing `document_relevance.relevance_tier` and
   `decision` can carry the distinction.
2. Add config-level behavior rather than hardcoding a new default.
3. Keep `C` opt-in for durable commit until storage/yield is measured.
4. Add query filters before enabling any broad commit of `C`.

If the current durable schema or commit pipeline cannot safely store `C` rows
without conflating them with trusted retained rows, stop and write
`CHANGE_REQUEST.md` with the exact schema/pipeline issue.

## 5. Step-by-Step Execution Plan

1. Prepare branch/context.
   - Start from clean `main` after v4 and learned-fallback artifacts are merged
     or intentionally preserved.
   - Create `agent/tiered-retention-policy`.
   - Read this task, roadmap, product plan, v4 report, learned fallback report,
     phase-8 durable commit runbook, commit/query code, and relevant tests.

2. Audit binary assumptions.
   - Search for `decision = 'retain'`, `decision == "retain"`, tier filters,
     and query defaults.
   - Inspect:
     - `cmd/commit-candidates`
     - `internal/candidate/committer.go`
     - `cmd/query`
     - evidence export scripts
     - scorer/evaluator docs
   - Record findings in `IMPLEMENTATION_REPORT.md`.

3. Update policy docs.
   - Update `docs/plans/argus-roadmap.md`.
   - Update `docs/plans/local-market-research-product-plan.md` if needed.
   - Update the relevant runbook, likely
     `docs/runbooks/phase-8-durable-candidate-commit.md` and/or query-layer
     runbook.
   - Include the explicit tier meanings and default query behavior.

4. Add config or code for tier-aware retention.
   - If current commit path only commits `decision = retain`, add a narrow
     opt-in path to include `decision = evaluate` / tier `C` as review-tier
     documents.
   - Ensure document relevance rows preserve original tier and decision.
   - Ensure `C` rows are not silently treated as trusted rows.
   - Prefer an explicit CLI/config flag over changing defaults.

5. Add query/export guardrails.
   - Ensure default research query paths use trusted tiers only (`A/B` or
     `decision = retain`).
   - Add explicit option/config for including review-tier evidence.
   - If the query layer is not ready for this change, document the needed
     follow-up instead of overbuilding.

6. Add tests.
   - Test that default behavior excludes `C`.
   - Test that explicit review-tier opt-in includes `C`.
   - Test that stored/exported relevance metadata preserves tier and decision.
   - Test that no default scorer promotion or threshold change occurs.

7. Verify.
   - Run focused tests for changed packages.
   - Run `go test ./...`.
   - Run Python compilation for changed scripts.
   - Run `git diff --check`.
   - Do not run full-month ingestion.

8. Report.
   - Write `IMPLEMENTATION_REPORT.md`.
   - State whether the task implemented tier-aware retention or produced a
     `CHANGE_REQUEST.md`.
   - Name the next task: bounded shard/month trial of tiered retention, with
     storage/yield and query-quality report.

## 6. Test Plan

Focused tests depend on the audit result, but should include:

- Commit policy excludes `C` by default.
- Commit policy includes `C` only with explicit review-tier opt-in.
- Query defaults exclude review-tier rows.
- Query opt-in includes review-tier rows.
- Evidence export/reporting preserves `relevance_tier`, `decision`, and
  `decision_reasons`.

Regression checks:

```bash
go test ./internal/candidate ./cmd/commit-candidates ./cmd/query ./cmd/export-evidence
go test ./...
go vet ./...
python3 -m py_compile scripts/dev/*.py
git diff --check
```

## 7. Acceptance Criteria

- Product policy says Argus uses confidence tiers, not a single binary truth
  filter.
- `A/B` are trusted evidence; `C` is review/exploration evidence; `D` is discard.
- Defaults protect research quality by excluding `C`.
- Opt-in paths can include `C` for review or bounded retention trials.
- Storage/yield risks are documented.
- No frozen shards are accessed.
- Verification passes.

## 8. Risks and Guardrails

- Storing too much `C` can pollute research results if query defaults are weak.
  Query defaults must be strict before enabling broader retention.
- Storage budget can drift if `C` is retained without yield reporting. The next
  task must measure storage/yield on a bounded shard or month.
- If schema changes are required, stop for a planner decision rather than
  slipping migrations into this task.
- Do not reinterpret failed retrieval experiments as success. The point is not
  "v4 passed"; the point is "v4 can be useful when its tiers are respected."

## 9. Executor Instructions

1. Work on `agent/tiered-retention-policy`.
2. Start from clean `main` after the current learned-retrieval branch is handled.
3. Read `TASK.md`, this plan, the roadmap, product plan, v4 report, learned
   fallback report, and phase-8 durable commit runbook.
4. Audit before editing code.
5. Keep the implementation narrow and tier-focused.
6. Do not access frozen shards or run full-month ingest.
7. If schema changes or broad pipeline rewrites are required, write
   `CHANGE_REQUEST.md` and stop.
8. Complete `IMPLEMENTATION_REPORT.md` before review.
