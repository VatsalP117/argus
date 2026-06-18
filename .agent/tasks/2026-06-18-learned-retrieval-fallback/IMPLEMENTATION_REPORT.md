# Implementation Report

## Summary

Implemented a bounded local learned retrieval fallback experiment on top of the
existing deterministic `v4` candidate + scoring outputs, per
`.agent/tasks/2026-06-18-learned-retrieval-fallback/TASK.md` and `PLAN.md`.

The experiment tests whether a tiny, pure-stdlib, deterministic logistic
reranker can recover `v4-evaluate`-tier candidates that deterministic scoring
cannot retain without precision collapse. The learned layer is strictly
additive (promotes `evaluate` only; never demotes `retain`).

**Outcome: `failed_experiment`.** The learned layer improves `001` recall
cross-fixture (`36.5%` → `60.0%`, clears the `50%` gate) but collapses
precision (`81.6%` → `61.4%`, below the `75%` gate) and leaks a
payment-brand-Visa false positive that v4 did not retain. This violates two
hard acceptance requirements. No frozen model artifact was written. No gates
were lowered. No frozen validation shards were accessed.

The result is reported honestly in
`docs/reports/learned-retrieval-fallback-2026-06-18.md`.

## Files Changed

Added:

- `scripts/dev/learned_relevance_lib.py` — pure-stdlib library: feature
  extraction with leakage guards, deterministic full-batch logistic regression
  with early stopping, threshold selection, and a candidate-level evaluator
  mirroring the retrieval gates (precision, recall, per-domain precision, trap
  counts, missing-URL count, FP-category rate).
- `scripts/dev/learned_relevance_experiment.py` — CLI orchestrator. Reuses the
  existing `calibrate_relevance_fixtures` candidate reconstruction and
  `duckdb_score_candidates.py` v4 scorer (no scorer changes), then runs
  cross-fixture (leave-one-fixture-out), pooled 5-fold CV, and a final
  in-sample observed-only model. Writes per-fixture learned-score parquet
  outputs and a `diagnostics.json`.
- `scripts/dev/tests/test_learned_relevance.py` — focused unittest suite
  (28 tests).
- `scripts/dev/tests/__init__.py` — package marker for test discovery.
- `docs/reports/learned-retrieval-fallback-2026-06-18.md` — observed-fixture
  report with exact commands, metrics, feature list, failure-mode analysis,
  and the `failed_experiment` decision.
- `.agent/tasks/2026-06-18-learned-retrieval-fallback/IMPLEMENTATION_REPORT.md`
  — this file.

No existing source files were modified. No Go files changed. No config files
changed. No label or annotation files changed. No frozen model artifact was
written (`configs/relevance/learned-v1.json` does not exist).

## Commands Run

```bash
# v4 baseline reproduction (matches published v4 report exactly)
python3 scripts/dev/calibrate_relevance_fixtures.py \
  --relevance-config configs/relevance/deterministic-v4.yaml \
  --output-dir .tmp/learned-baseline-v4 \
  --fixture 'comments-2021-01-000|evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv|evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json' \
  --fixture 'comments-2021-01-001|evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv|evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json'

# Focused tests
python3 -m unittest discover -s scripts/dev/tests -v

# Learned experiment (the required command shape)
python3 scripts/dev/learned_relevance_experiment.py \
  --relevance-config configs/relevance/deterministic-v4.yaml \
  --output-dir .tmp/learned-retrieval-v1 \
  --fixture 'comments-2021-01-000|evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv|evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json' \
  --fixture 'comments-2021-01-001|evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv|evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json'
```

Machine-readable outputs (gitignored, under `.tmp/`):

- `.tmp/learned-retrieval-v1/diagnostics.json`
- `.tmp/learned-retrieval-v1/<fixture>/learned-scores.parquet`
- `.tmp/learned-retrieval-v1/<fixture>/v4-scores.parquet`
- `.tmp/learned-retrieval-v1/<fixture>/reconstructed-candidates.parquet`

## Tests

Focused unittest suite: **28 tests, all passing.**

```
python3 -m unittest discover -s scripts/dev/tests -v
Ran 28 tests in 0.187s
OK
```

Coverage:

- `LeakagePreventionTests` — feature names exclude leakage substrings;
  feature vectors do not encode `source_id`/`source_url`; leakage guard
  rejects injected leakage names; frozen fixture IDs refused by both the lib
  and the experiment CLI; non-approved fixture IDs refused.
- `DeterminismTests` — feature extraction independent of input row order;
  feature spec independent of input row order; training fully deterministic
  (identical weights/bias/epochs across repeated runs); training reproducible.
- `FeatureNormalizationTests` — tokenization lowercases and splits;
  term/group indicators consistent; domain/tier/decision indicators correct;
  text-length buckets correct.
- `ThresholdSelectionTests` — maximizes recall subject to precision gate;
  no-threshold-meets-precision returns unselected with a clear reason;
  deterministic tie-break prefers conservative (higher) threshold;
  threshold only promotes `evaluate`-tier candidates (never demotes `retain`).
- `EvaluatorMetricsTests` — known tiny-fixture metrics match hand-computed
  values; trap and missing-URL gates fire; v4 baseline at `threshold=+inf`
  equals zero learned promotion.
- `ModelLearningTests` — separable pattern is learned (positives score above
  negatives); training loss decreases.
- `ExperimentGuardTests` — refuses non-approved fixtures; missing labels /
  annotations / duplicate fixtures raise clear errors; `decide_outcome`
  returns `failed_experiment` on precision collapse + trap leak and
  `ready_for_frozen_validation` when all criteria pass.

## Deviations From Plan

- **Default hyperparameters.** The plan suggested `n_epochs=600`,
  `learning_rate=0.3`. I used `n_epochs=400`, `learning_rate=0.5` with
  deterministic early stopping (`tolerance=1e-7`, `patience=25`). This is a
  smaller decision within the plan's allowed space ("Keep it boring and
  inspectable") and was needed to keep the 8 model trainings
  (2 cross-fixture + 5 CV folds + 1 final) fast enough to run end-to-end in
  ~60s. Early stopping is deterministic (identical inputs always stop at the
  same epoch) and is covered by the determinism tests. The model is still a
  plain regularized logistic regression with zero-init weights and no
  randomness.
- **Predicted-domain rule.** The plan did not specify how `predicted_domain`
  is chosen for retained candidates. I made v4-retain candidates use the
  scorer's own argmax among v4-retain domains (matching the deterministic v4
  evaluator) and only pure learned promotions use the learned-score argmax.
  This avoids mixing v4 and learned score scales and keeps the learned layer's
  effect isolated to promoted candidates.

No other deviations. No new dependencies were added. No `CHANGE_REQUEST.md`
was needed.

## Known Risks

- **Small-label overfitting.** The final observed-only model passes the gate
  in-sample (`79.6%` precision, `73.5%` recall) but fails cross-fixture on the
  `001` direction. The report explicitly labels the in-sample number as an
  overfit ceiling and the cross-fixture number as the honest generalization
  signal.
- **Pooled CV predicted-domain proxy.** For promoted candidates in pooled CV,
  predicted-domain uses v4-score ordering among evaluate-tier domains. This is
  a minor proxy (the per-domain learned scores are used for the promotion
  decision itself via the rescale-to-zero trick; only the domain label uses v4
  ordering). This affects only per-domain precision reporting for promoted
  candidates, not the candidate-level precision/recall/trap gates that drive
  the decision.
- **No frozen validation.** By design, this experiment did not touch shards
  `002`/`003`. A future learned task, if approved, would need to validate on a
  real held-out shard; this experiment's `failed_experiment` decision means
  that step is not warranted now.

## Next Steps

Stop. The experiment is a `failed_experiment`. Per the task and plan, the work
does not proceed to frozen validation and does not add model complexity. The
report's "Next Steps" section records the honest options (collect more labels,
or revisit candidate generation for richer evidence). Any follow-up learned
task should be planned separately and keep the frozen shards untouched until a
model is frozen on observed data.
