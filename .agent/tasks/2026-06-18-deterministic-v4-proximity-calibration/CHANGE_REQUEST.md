# Change Request

## Summary

The passport/citizenship label-boundary conflict documented in the original
change request is **resolved** by a human decision. Abstract
passport/citizenship/vaccine-passport commentary is now out of scope for the
travel scorer. Seventeen labels were reconciled across both observed fixtures.

After reconciliation, `deterministic_v4` still improves both fixtures versus
V3 on the same labels, but `comments-2021-01-001` still fails the `50%` recall
gate. The remaining blocker is a **deterministic recall ceiling**, not a
label-boundary conflict.

Current stopping point (after label reconciliation):

- `comments-2021-01-000`: pass
  - exact retained precision: `85.1%` (V3 on same labels: `84.1%`)
  - regenerated full-label recall estimate: `59.4%` (V3 on same labels: `55.2%`)
- `comments-2021-01-001`: fail
  - exact retained precision: `81.6%` (V3 on same labels: `79.4%`)
  - regenerated full-label recall estimate: `36.5%` (V3 on same labels: `31.8%`,
    gate is `50%`)

## What Was Resolved

The human decision (2026-06-18):

> For the current travel scorer, only concrete travel/process/document pain is
> travel-positive. Abstract passport/citizenship/vaccine-passport commentary is
> out of scope for the current travel scorer, but should be noted as a possible
> future adjacent research domain.

Thirteen labels in `comments-2021-01-001` and four labels in
`comments-2021-01-000` were changed from travel-positive to travel-negative.
All were abstract passport/citizenship/vaccine-passport commentary, policy
discourse, statistics, opinion, or identity discussion without concrete
travel-process pain. Nine concrete travel-document/process pain cases were kept
as travel-positive. See `FIX_REPORT.md` and the calibration report for the full
table of changes with rationale.

## Why I Stopped (Updated)

The passport/citizenship boundary is no longer the blocker. The remaining
blocker is a deterministic recall ceiling on `comments-2021-01-001`:

- V4 reaches `36.5%` recall (`31/85` TP). The gate requires `50%` (`43` TP).
- The `54` remaining false negatives are non-passport cases at `0.45-0.55`
  scores that lack a general proximity pattern separating them from
  labeled-negative candidates at the same scores.
- Reaching `43` TP requires boosting `0.45-0.50` tier candidates by `+0.15`,
  which drags in `14` evaluate-tier labeled-negative candidates and collapses
  precision below `71%`, violating the `75%` precision gate.
- This is not fixable by more proximity rules, threshold changes, or label
  changes. It is evidence that additive deterministic scoring has reached its
  recall ceiling on this fixture.

## Recommended Follow-Up

The passport boundary is resolved. The remaining blocker requires a planner
decision:

1. **Activate the learned retrieval fallback roadmap.** Keep deterministic
   candidate retrieval (including V4 proximity rules) as the high-recall front
   door. Add a lightweight learned reranker or classifier on top of the existing
   candidate retrieval and DuckDB evaluation workflow to recover the
   `0.45-0.55` tier candidates that deterministic scoring cannot boost without
   precision collapse. This is the roadmap's recommended branch when
   deterministic scoring stalls on a ceiling that rules cannot resolve.

2. **Lower the recall gate for `001`.** If `36.5%` recall is acceptable for
   this fixture given the deterministic ceiling, explicitly authorize it and
   proceed to frozen validation on `002`. This is a product decision, not a
   scorer decision.

3. **Freeze V4 as a mixed result and validate on `002` anyway.** V4 improves
   both observed fixtures with no precision regression and no trap leakage. It
   may be worth validating on the frozen shard to see whether the recall ceiling
   is fixture-specific or systemic before committing to the learned retrieval
   pivot. This requires a separate validation task.

The roadmap's decision tree suggests option 1 when deterministic scoring stalls
on a ceiling that rules cannot resolve.

## Constraints Preserved

- No access to frozen shards `comments-2021-01-002` or `comments-2021-01-003`
- No CLI default change from V2 to V4
- No durable DuckDB mutation or commit
- No full-month run
- No dependency additions
- No candidate-scanner changes
- No scorer code changes (only label reconciliation per human decision)
- No threshold collapse
- No fixture-specific memorization, source IDs, or long fixture phrases encoded
  as scoring rules
- Proximity rules are general, pattern-based, explainable, and tested
- Existing score output schema unchanged; downstream export, evaluation, commit,
  and reports remain compatible
- Label changes were only within the passport/citizenship/vaccine-passport
  boundary, per the human decision
