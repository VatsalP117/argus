# Change Request

## Summary

`deterministic_v4` successfully adds a proximity-aware conjunction scoring
capability and improves both observed fixtures on every metric with zero new
false positives, but the `comments-2021-01-001` recall gate (`50%`) cannot be
met without resolving a label-boundary conflict between the two training
fixtures.

Current stopping point:

- `comments-2021-01-000`: pass
  - exact retained precision: `88.1%` (V3 was `87.3%`)
  - regenerated full-label recall estimate: `59.0%` (V3 was `55.0%`)
- `comments-2021-01-001`: fail
  - exact retained precision: `81.6%` (V3 was `79.4%`)
  - regenerated full-label recall estimate: `31.6%` (V3 was `27.6%`, gate is
    `50%`)

## Why I Stopped

The 001 recall gap is dominated by 13 passport/citizenship/vaccine-passport
candidates at score `0.55` that are labeled travel-positive in `001`. These
candidates have no travel-process failure, safety, or border-security evidence
that a general proximity rule can detect. They retain only if passport
mentions are broadly boosted.

The same passport/citizenship text pattern is labeled travel-negative in `000`
(5 passport negatives at `score >= 0.55`, including `ghnwnyn` at `0.95` and
`ghnoyns` at `0.75`). Boosting passport mentions enough to retain the 13 `001`
positives either keeps or newly retains these `000` negatives, dropping `000`
below its V3 precision baseline and the `75%` gate.

This is a label-boundary conflict between the two training fixtures, not a
scorer-expressiveness gap. The two fixtures apply opposite conventions to the
same text pattern. No general, pattern-based deterministic rule can satisfy
both simultaneously.

The remaining non-passport recoverable false negatives (24 at
`score >= 0.45`) would each require `+0.15` boosts. Simulated calibration
shows this drags in 14 evaluate-tier labeled-negative candidates and collapses
precision below `71%`, violating the `75%` precision gate.

Continuing with more config-only rule tweaking would require either:

1. Encoding fixture-specific passport-context memorization (forbidden by the
   task constraints).
2. Lowering the retain threshold (threshold collapse, forbidden).
3. Changing the labels (forbidden; requires human review).
4. Changing the scanner to include more source-text context (out of scope).

## Recommended Follow-Up

Have the planner review one of these explicit directions before more executor
work:

1. **Label reconciliation (human decision).** Decide whether
   passport/citizenship/vaccine-passport commentary is in-scope for the travel
   domain. If out-of-scope, reconcile the `001` labels with `000` (remove the
   travel-positive label from the 13 passport-boundary cases). This lowers the
   `001` recall denominator and may let V4 pass the observed gate as-is, after
   which frozen validation on `002` can proceed. If in-scope, reconcile `000`
   labels with `001`, which likely requires scanner or source-text context
   changes beyond deterministic scoring.

2. **Accept the deterministic ceiling and activate the learned retrieval
   fallback roadmap.** The V4 proximity capability is a genuine improvement and
   is worth keeping as the deterministic front door, but the passport/citizenship
   boundary is evidence that purely deterministic rules cannot resolve all
   label-boundary cases. A lightweight learned reranker or classifier on top of
   the existing candidate retrieval and DuckDB evaluation workflow is the
   roadmap's recommended next step when deterministic scoring stalls on a
   boundary that rules cannot resolve.

3. **Freeze V4 as a mixed result and validate on `002` anyway.** V4 improves
   both observed fixtures with no precision regression and no trap leakage. It
   may be worth validating on the frozen shard to see whether the
   passport/citizenship boundary conflict is fixture-specific or systemic
   before committing to the learned retrieval pivot. This requires a separate
   validation task.

The task's and roadmap's decision tree suggests option 2 if the gate cannot be
met, but option 1 is the smallest step if the label boundary is the only
blocker.

## Constraints Preserved

- No access to frozen shards `comments-2021-01-002` or `comments-2021-01-003`
- No CLI default change from V2 to V4
- No durable DuckDB mutation or commit
- No full-month run
- No dependency additions
- No candidate-scanner changes
- No label modifications
- No threshold collapse
- No fixture-specific memorization, source IDs, or long fixture phrases encoded
  as scoring rules
- Proximity rules are general, pattern-based, explainable, and tested
- Existing score output schema unchanged; downstream export, evaluation, commit,
  and reports remain compatible
