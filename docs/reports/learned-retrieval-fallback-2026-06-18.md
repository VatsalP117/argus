# Learned Retrieval Fallback Experiment Report

Date: `2026-06-18`

Status: `failed_experiment`

Decision: `failed_experiment` (no frozen model artifact written; no promotion)

## Scope

This report records a bounded local learned reranker experiment on top of the
existing deterministic `v4` candidate retrieval and scoring outputs. The
experiment uses only the two approved observed training fixtures:

- `comments-2021-01-000`
- `comments-2021-01-001`

Frozen validation fixtures were not accessed, inspected, scored, sampled,
trained on, or validated on:

- `comments-2021-01-002`
- `comments-2021-01-003`

The experiment does not mutate durable DuckDB, does not run a full month, does
not modify labels, does not add LLM calls / embeddings / vector DB / cloud
services, does not add new dependencies, and does not promote the learned layer
into default scoring.

## Purpose

`deterministic_v4` reached a deterministic recall ceiling on
`comments-2021-01-001` (full-label recall `36.5%` against a `50%` gate). The
remaining false negatives live mostly in the `0.45-0.55` v4-evaluate tier,
where boosting them deterministically would also retain labeled-negative
candidates and collapse precision below the `75%` gate. The roadmap's decision
tree says to activate the learned retrieval fallback when deterministic scoring
stalls on a ceiling that rules cannot resolve.

This experiment tests whether a tiny, local, deterministic logistic reranker
can recover those v4-evaluate-tier candidates **without precision collapse or
trap regression**.

## Approach

A new pure-stdlib Python experiment sits on top of the existing calibration
path:

- `scripts/dev/learned_relevance_lib.py` — DuckDB-free library: feature
  extraction with leakage guards, deterministic logistic regression, threshold
  selection, and a candidate-level evaluator that mirrors the retrieval gates.
- `scripts/dev/learned_relevance_experiment.py` — orchestrator that reuses
  `calibrate_relevance_fixtures` candidate reconstruction and the existing
  `duckdb_score_candidates.py` v4 scorer, then layers the learned experiment on
  top.
- `scripts/dev/tests/test_learned_relevance.py` — focused unittest suite.

The learned layer is **strictly additive**: it can only promote
`v4-evaluate`-tier candidates to `retain`. It never demotes a `v4-retain`
candidate and never touches `v4-discard` candidates. This guarantees the learned
layer cannot reduce v4's retained set, only extend it.

### Model

Deterministic full-batch logistic regression, implemented locally with no
external dependencies:

- weights initialized to zero (no randomness, no shuffle)
- full-batch gradient descent with L2 regularization
- deterministic early stopping on loss convergence (`tolerance=1e-7`,
  `patience=25`); identical inputs always stop at the same epoch
- hyperparameters: `n_epochs=400`, `learning_rate=0.5`, `l2=0.01`

The model outputs a per-(candidate, domain) score in `[0, 1]`. A single retain
threshold is selected on training folds only.

### Features

All features are auditable and non-leaky. Vocabularies (text tokens, matched
terms) are derived from training folds only; indicator groups (rule groups,
proximity rules, domains, tiers, decisions) are derived from the deterministic
v4 config, not from data.

Feature groups:

- domain one-hot (`travel`, `saas_opportunity`, `app_opportunity`)
- v4 relevance score (clipped to `[0, 1]`)
- v4 tier one-hot (`A`, `B`, `C`, `D`)
- v4 decision one-hot (`retain`, `evaluate`, `discard`)
- matched rule group indicators (from v4 config)
- proximity rule indicators (from v4 `decision_reasons`)
- matched-terms count (capped, normalized)
- matched-rule-group count (capped, normalized)
- text length bucket (`short < 40 tokens`, `medium < 160`, `long`)
- matched-term indicators (vocabulary from training folds, `min_count=3`)
- text-token indicators (vocabulary from training folds, `min_count=5`,
  `max_vocab=120`)

Excluded leakage features (enforced by `LEAKAGE_FIELD_SUBSTRINGS` and unit
tested):

- `source_id`, `source_url`, `source_type`
- `fixture_id`, fixture identity
- `sample_stratum`, `stratum_population`, `sample_rank`, `sampling_seed`
- `label_*` fields
- `false_positive_category` and FP categories
- `subreddit`, row order, row index

### Evaluation discipline

Three diagnostics, reported honestly:

1. **Cross-fixture (leave-one-fixture-out)** — the primary generalization
   signal. Train on `000`, evaluate on `001`; train on `001`, evaluate on `000`.
   Threshold selected on the training fold only.
2. **Pooled k-fold CV** — secondary diagnostic. Candidate-level 5-fold CV
   across both observed fixtures; each candidate held out once and scored with
   its own fold's threshold (out-of-fold).
3. **Final observed-only (in-sample)** — train on both fixtures and evaluate on
   the same rows. Explicitly optimistic / overfit; reported only as a ceiling,
   not as a generalization claim.

## Commands

```bash
# Focused tests
python3 -m unittest discover -s scripts/dev/tests -v

# Experiment (observed fixtures only)
python3 scripts/dev/learned_relevance_experiment.py \
  --relevance-config configs/relevance/deterministic-v4.yaml \
  --output-dir .tmp/learned-retrieval-v1 \
  --fixture 'comments-2021-01-000|evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv|evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json' \
  --fixture 'comments-2021-01-001|evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv|evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json'
```

Machine-readable diagnostics: `.tmp/learned-retrieval-v1/diagnostics.json`.
Per-fixture learned + v4 score parquets: `.tmp/learned-retrieval-v1/<fixture>/`.

## Checksums

Base relevance config:

- `configs/relevance/deterministic-v4.yaml`
  - `sha256:b9c86b936c7f6e532a19887635010012ad5ecfa12edf6b24a212a80ac0755c84`

Label / annotation checksums (unchanged from the v4 calibration report):

- `comments-2021-01-000` labels:
  `sha256:190882bcfaa79238539b13177ef0e23641da3e914cda27b3d9cbc0c7b1875b9e`
- `comments-2021-01-000` annotations:
  `sha256:b63bbbeab6539b61e840580a415c29b545da2623fa99371e6de21281db26ef4b`
- `comments-2021-01-001` labels:
  `sha256:8d9e3d975dba93f8e2c32936a45b3ead4f6295391373c7d3f4e140783c667b4c`
- `comments-2021-01-001` annotations:
  `sha256:9b995c344df49a88075e07b82d911d37704763191c983c13182664bd03ce3b5b`

Frozen shards accessed: none (`comments-2021-01-002`, `comments-2021-01-003`
were not read by any command in this task).

## Baseline (deterministic v4, reproduced by this experiment)

| Fixture | Retained | TP | Precision | Recall | Visa FP | Promo/bot FP | Missing URL |
| :-- | --: | --: | --: | --: | --: | --: | --: |
| `comments-2021-01-000` | `67` | `57` | `85.1%` | `59.4%` | `0` | `0` | `0` |
| `comments-2021-01-001` | `38` | `31` | `81.6%` | `36.5%` | `0` | `0` | `0` |

These match the published v4 calibration report exactly, confirming the
experiment is built on the approved v4 artifact and reconciled labels.

## Cross-fixture results (primary generalization signal)

| Direction | Threshold | Retained | TP | Precision | Recall | Gate | Visa FP | Travel precision |
| :-- | --: | --: | --: | --: | --: | :-- | --: | --: |
| train `000` → eval `001` | `0.2143` | `83` | `51` | `61.4%` | `60.0%` | fail | `1` | `51.2%` |
| train `001` → eval `000` | `0.2758` | `100` | `76` | `76.0%` | `79.2%` | pass | `0` | `68.5%` |

The `001` cross-fixture direction is the one the task is trying to fix.

- Recall improves substantially: `36.5%` → `60.0%`, clearing the `50%` gate.
- But precision collapses: `81.6%` → `61.4%`, below the `75%` gate.
- A `payment_brand_visa` false positive is promoted that v4 did not retain
  (`visa_fp=1`, v4 baseline `0`). This violates the hard "zero retained
  payment-brand Visa false positives" requirement.
- Travel-domain retained precision collapses to `51.2%` (`22/43`).
- Retained false-positive mix on `001`: `lexical_ambiguity=10`,
  `generic_product_mention=8`, `political_immigration=2`,
  `payment_brand_visa=1`.

The reverse direction (`train 001 → eval 000`) passes the gate cleanly
(`76.0%` precision, `79.2%` recall, zero traps), so the model does carry real
signal. The failure is specific to the `001` evaluate tier, where the positives
and negatives are not linearly separable by these features.

## Pooled 5-fold CV (secondary diagnostic)

Candidate-level folds across both observed fixtures (`590` candidates). Each
candidate held out once and evaluated with its own fold's threshold
(out-of-fold).

| Fold | Threshold | Retained | TP | Precision | Recall | Gate |
| :-- | --: | --: | --: | --: | --: | :-- |
| 0 | `0.2288` | `29` | `20` | `69.0%` | `64.5%` | fail |
| 1 | `0.3240` | `32` | `28` | `87.5%` | `70.0%` | pass |
| 2 | `0.2610` | `38` | `28` | `73.7%` | `71.8%` | fail |
| 3 | `0.2465` | `37` | `23` | `62.2%` | `67.6%` | fail |
| 4 | `0.2687` | `36` | `29` | `80.6%` | `78.4%` | fail |

| Pooled | Retained | TP | Precision | Recall | Gate | Visa FP | Promo/bot FP | Missing URL |
| :-- | --: | --: | --: | --: | :-- | --: | --: | --: |
| out-of-fold | `172` | `128` | `74.4%` | `70.7%` | fail | `0` | `0` | `0` |

Pooled precision sits just below the `75%` gate (`74.4%`) with no Visa or
promotion/bot trap leakage and no missing URLs. The gate fails narrowly on
precision, and two of the five folds collapse below the precision gate. This is
consistent with the cross-fixture picture: the linear features have signal but
are not stable enough across folds to hold the precision gate while recovering
the evaluate tier.

## Final observed-only (in-sample, optimistic ceiling)

Train on both fixtures, evaluate on the same rows. **This is train-on-test and
is reported only as an overfit ceiling, not as a generalization result.**

| | Retained | TP | Precision | Recall | Gate |
| :-- | --: | --: | --: | --: | :-- |
| in-sample | `167` | `133` | `79.6%` | `73.5%` | pass |

The in-sample model passes the gate, which confirms the model can fit the
observed data. It does not generalize cross-fixture, so this number must not be
quoted as a real result. No frozen model artifact was written from it.

## Failure Mode Analysis

The learned layer succeeds at the thing it was asked to do — it recovers
recall in the `001` evaluate tier (`36.5%` → `60.0%` cross-fixture). It fails
at the constraint that makes that recovery useful: it cannot do so without
precision collapse and a trap leak.

Concretely, promoting enough `001` evaluate-tier candidates to clear the `50%`
recall gate (`83` retained vs v4's `38`) drags in `32` false positives,
including `1` payment-brand-Visa trap and `18` lexical-ambiguity /
generic-product-mention traps. Travel-domain retained precision falls to
`51.2%`. This is the same precision-collapse failure mode the v4 calibration
report identified for deterministic boosting, now reproduced by a learned
linear model: the `001` `0.45-0.55` tier's positives and negatives are not
linearly separable by text, matched-term, rule-group, and v4-score features.

The reverse direction and the pooled CV show the features carry real signal
(pooled precision `74.4%`, reverse-direction gate passes), but "real signal,
narrowly below the gate, with one cross-fixture direction collapsing" is a
`failed_experiment`, not a `needs_more_labels`, because the `001` cross-fixture
direction violates two hard requirements (precision gate and zero Visa FPs).

## Decision

`failed_experiment`

Criteria check (cross-fixture, train `000` → eval `001`):

| Criterion | Result |
| :-- | :-- |
| improves `001` recall over v4 | yes (`36.5%` → `60.0%`) |
| meets `001` `50%` recall gate cross-fixture | yes |
| holds `75%` precision gate on `001` | **no** (`61.4%`) |
| zero retained payment-brand Visa FPs on `001` | **no** (`1`) |
| no precision collapse on `000` reverse direction | yes (`76.0%`) |

Because two hard requirements fail on the `001` cross-fixture direction, the
experiment does not meet the acceptance targets. No frozen model/config
artifact was written. The learned layer is not promoted anywhere. Gates were
not lowered and no tuning was performed on frozen validation.

## What Was Not Done

- No access to frozen shards `comments-2021-01-002` or `comments-2021-01-003`.
- No new dependencies added (pure Python stdlib + existing `duckdb`/`pyyaml`
  dev dependencies already used by calibration scripts).
- No durable DuckDB mutation; all DuckDB connections were in-memory.
- No full-month ingest.
- No label modifications.
- No LLM calls, embeddings, vector DB, or cloud services.
- No change to v4 scorer behavior.
- No promotion of the learned layer into durable ingest or default scoring.
- No frozen model artifact written (decision is `failed_experiment`).

## Next Steps

This is a `failed_experiment`, so per the task and plan the work stops here.
The roadmap's decision tree says not to keep adding model complexity when a
simple linear learned layer fails. The honest signals point to one of:

1. **Collect more labels.** The pooled CV is close to the gate (`74.4%`
   precision, `70.7%` recall) and the reverse cross-fixture direction passes,
   which suggests the features are weakly sufficient but the `001` evaluate
   tier is too small / too noisy to separate with a linear model. More labels
   across additional shards would let a future learned task train on a larger,
   less fixture-specific pool and validate on a real held-out shard.
2. **Revisit candidate generation.** The `001` evaluate-tier positives that the
   model cannot cleanly separate may need richer candidate evidence (e.g.
   thread/parent context) rather than a smarter scorer. That is a candidate
   scanner change, out of scope here.
3. **Do not silently re-run with more features or a more complex model.** The
   plan explicitly says not to keep adding model complexity if simple linear
   features fail; that is a signal to collect more labels or revisit candidate
   generation.

Any follow-up learned task should be planned separately, keep the frozen
shards untouched until a model is frozen on observed data, and reuse the
evaluation discipline and leakage guards from this experiment.
