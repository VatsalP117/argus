# Implementation Plan

## 1. Task Summary

Implement a thin, local learned retrieval fallback prototype on top of existing
candidate fixtures and deterministic v4 scores. The prototype should test
whether a small reranker/classifier can improve retained recall on the observed
fixtures without violating precision and trap gates.

This is an experiment, not a production promotion. It should end with a report
that says one of:

- `ready_for_frozen_validation`
- `needs_more_labels`
- `failed_experiment`

## 2. Current System Understanding

- Argus already has broad candidate retrieval, deterministic scoring, label
  fixtures, and calibration tooling.
- `scripts/dev/calibrate_relevance_fixtures.py` can reconstruct candidates from
  labels, score them with v4, reapply annotations, and evaluate observed
  fixtures.
- `deterministic_v4` improved precision and recall but failed observed recall
  on `comments-2021-01-001` after the passport/citizenship boundary was
  reconciled.
- The remaining misses live mostly in the deterministic middle tier, where
  global boosts would also retain labeled negatives.
- The learned layer should sit after deterministic candidate retrieval and use
  existing evidence fields such as text, matched terms, matched rule groups,
  deterministic scores, tiers, decisions, and decision reasons.
- DuckDB remains the source-of-truth workflow. No separate vector database or
  remote model service is needed.

## 3. Scope

### In Scope

- Add a local script or CLI for learned relevance experiments.
- Extract non-leaky features from reconstructed candidates and v4 score rows.
- Train a lightweight model using only observed fixtures `000` and `001`.
- Evaluate with a discipline that avoids pure train-on-test reporting, such as:
  - leave-one-fixture-out evaluation, and
  - pooled cross-validation for diagnostic reporting.
- Produce retained/evaluate/discard decisions compatible with the existing
  evaluator, preferably by writing score-like Parquet output.
- Compare learned output against deterministic v4 on the same reconciled labels.
- Add focused tests for feature extraction, leakage prevention, deterministic
  output, and threshold selection.
- Publish a report under `docs/reports/`.

### Out of Scope

- Frozen validation on shards `002` or `003`.
- Durable DuckDB mutation or ingestion.
- Default scorer promotion.
- Label changes.
- LLM calls, embeddings, vector search, or cloud services.
- New dependencies unless explicitly approved by `CHANGE_REQUEST.md`.
- A polished model registry or production training system.
- Broad candidate scanner changes.

## 4. Proposed Technical Approach

Build a small Python-based experiment under `scripts/dev/`, using only standard
library plus packages already required by this repo's dev scripts, such as
DuckDB if available.

Suggested artifact:

- `scripts/dev/learned_relevance_experiment.py`

Suggested behavior:

1. Reconstruct observed candidates using the same label inputs as calibration.
2. Score candidates with `configs/relevance/deterministic-v4.yaml`.
3. Join candidate text, matched terms/rule groups, v4 domain scores, tiers,
   decisions, and labels.
4. Build simple, auditable features:
   - deterministic v4 score by domain
   - v4 decision/tier by domain
   - matched rule group indicators
   - matched term indicators
   - short lexical indicators from candidate text with minimum frequency
   - text length buckets
   - proximity decision-reason indicators
5. Exclude leakage features:
   - source ID
   - source URL
   - fixture ID
   - sample stratum, sampling seed, sample rank
   - existing label fields
   - false-positive category
6. Train a small binary classifier for "retain-worthy candidate" using a
   deterministic implementation.

Recommended first model:

- regularized logistic regression implemented locally with deterministic SGD, or
- a simple averaged perceptron / linear classifier implemented locally.

Keep it boring and inspectable. Do not add scikit-learn unless a human approves
a dependency change.

Thresholding:

- The model should output a probability-like or score-like value per
  candidate/domain.
- Select the retain threshold on training folds to satisfy at least `75%`
  precision while maximizing recall.
- Report the selected threshold and retained counts.
- Do not tune directly on a held-out fixture and then claim that fixture as
  generalization.

Evaluation discipline:

- Primary diagnostic:
  - train on `000`, evaluate on `001`
  - train on `001`, evaluate on `000`
- Secondary diagnostic:
  - pooled deterministic k-fold cross-validation across observed fixtures
- Final observed-only candidate:
  - train on both observed fixtures and produce a frozen config/model artifact
    only if cross-fixture diagnostics are credible.

Output compatibility:

- Prefer writing learned scores to Parquet with columns compatible enough for
  existing evaluation scripts, or add a small learned-specific evaluator that
  emits the same metric schema as `calibrate_relevance_fixtures.py`.
- Keep deterministic v4 score outputs available for side-by-side comparison.

Model artifact:

- If the experiment succeeds, write a small JSON model/config artifact under
  `configs/relevance/learned-v1.json` or similar.
- Include feature names, weights, threshold, training fixture IDs, label
  checksums, and v4 config checksum.
- Do not make this artifact the default anywhere.

## 5. Step-by-Step Execution Plan

1. Prepare branch and context.
   - Start from a clean `main` that includes the approved v4 calibration
     artifact.
   - Create `agent/learned-retrieval-fallback`.
   - Read `TASK.md`, this plan, the roadmap, the v4 calibration report, and the
     v4 `CHANGE_REQUEST.md`.

2. Establish deterministic baseline.
   - Run v4 calibration on observed fixtures.
   - Record baseline metrics and checksums in the learned retrieval report.
   - Confirm no command references frozen shards `002` or `003`.

3. Add feature extraction.
   - Build a deterministic feature extractor from reconstructed candidates and
     v4 score rows.
   - Add tests proving leakage fields are excluded.
   - Add tests proving feature ordering is deterministic.

4. Add lightweight model training.
   - Implement a simple local linear classifier.
   - Fix random seed or use deterministic iteration.
   - Add tests with tiny synthetic data proving the model can learn a separable
     pattern and produces stable predictions.

5. Add threshold selection and evaluation.
   - Select threshold from training data/folds only.
   - Emit metrics matching the retrieval gate:
     precision, recall, per-domain precision, trap counts, missing URL count,
     retained/evaluate/discard counts.
   - Add tests for threshold selection edge cases: no positives, no threshold
     meeting precision, ties, and deterministic ordering.

6. Run cross-fixture diagnostics.
   - Train on `000`, evaluate on `001`.
   - Train on `001`, evaluate on `000`.
   - Record whether the model generalizes or merely memorizes one fixture.

7. Run pooled observed diagnostic.
   - Run deterministic k-fold cross-validation across `000` and `001`.
   - If credible, train a final observed-only model on both fixtures.
   - Write the model artifact only if diagnostics beat v4 without trap
     regression.

8. Publish report.
   - Add `docs/reports/learned-retrieval-fallback-2026-06-18.md`.
   - Include:
     - baseline v4 metrics
     - training/evaluation protocol
     - feature list and excluded leakage fields
     - cross-fixture results
     - pooled diagnostic results
     - final observed-only result, if produced
     - exact commands
     - decision: `ready_for_frozen_validation`, `needs_more_labels`, or
       `failed_experiment`

9. Final checks and implementation report.
   - Run focused tests.
   - Run full Go tests.
   - Run Python compilation.
   - Run `git diff --check`.
   - Write `IMPLEMENTATION_REPORT.md`.

## 6. Test Plan

Focused tests:

- Feature extraction excludes leakage fields.
- Feature extraction is deterministic across input row order.
- Text/term/rule-group features are normalized consistently.
- Linear model training is deterministic.
- Threshold selection maximizes recall subject to precision gate.
- Evaluation metrics match known tiny fixtures.
- Script refuses frozen fixture IDs.
- Script fails clearly if labels, annotations, or v4 score outputs are missing.

Regression checks:

```bash
go test ./internal/config ./internal/relevance ./cmd/score-candidates ./cmd/evaluate-relevance
go test ./...
go vet ./...
python3 -m py_compile scripts/dev/*.py
git diff --check
```

Observed experiment command shape:

```bash
python3 scripts/dev/learned_relevance_experiment.py \
  --relevance-config configs/relevance/deterministic-v4.yaml \
  --output-dir .tmp/learned-retrieval-v1 \
  --fixture 'comments-2021-01-000|evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv|evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json' \
  --fixture 'comments-2021-01-001|evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv|evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json'
```

## 7. Acceptance Criteria

- No frozen validation shard was accessed.
- No new external dependency was added.
- Learned experiment is reproducible with exact commands.
- Model features are documented and non-leaky.
- Cross-fixture diagnostics are reported honestly.
- Learned output improves `001` recall over v4 without precision or trap
  collapse, or the report clearly records why it does not.
- If observed targets pass, a frozen model/config artifact is written but not
  promoted.
- If observed targets do not pass, the task records `needs_more_labels` or
  `failed_experiment` and stops.
- `IMPLEMENTATION_REPORT.md` is complete.

## 8. Risks and Guardrails

- Small labels can overfit easily. Cross-fixture diagnostics are mandatory.
- Source IDs, URLs, fixture IDs, and sampling metadata would leak identity.
  Exclude them from model features.
- A model that passes pooled cross-validation but fails train-000/evaluate-001
  is not ready for frozen validation.
- Do not lower gates silently. If the learned layer cannot meet them, say so.
- Do not add a vector database or embeddings to solve a first learned
  prototype.
- Do not keep adding model complexity if simple linear features fail. That is a
  signal to collect more labels or revisit candidate generation.

## 9. Executor Instructions

1. Work on `agent/learned-retrieval-fallback`.
2. Start only after the v4 calibration branch is committed/merged or otherwise
   available cleanly from `main`.
3. Read `TASK.md`, this plan, the roadmap, the v4 report, and v4
   `CHANGE_REQUEST.md`.
4. Do not access frozen shards `002` or `003`.
5. Do not modify labels.
6. Implement the smallest reproducible learned experiment that can answer the
   gate question.
7. If a new dependency or broader architecture is required, write
   `CHANGE_REQUEST.md` and stop.
8. If the learned layer fails, report the failure honestly. Do not tune on
   hidden validation or lower gates.
9. Complete `IMPLEMENTATION_REPORT.md` before review.
