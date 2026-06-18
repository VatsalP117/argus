# Deterministic V4 Calibration Report

Date: `2026-06-18`

Status: `mixed_calibration`

## Scope

This report calibrates `deterministic_v4` using only the two already-observed
training fixtures:

- `comments-2021-01-000`
- `comments-2021-01-001`

Frozen validation fixtures were not accessed:

- `comments-2021-01-002`
- `comments-2021-01-003`

The calibration runner reconstructs bounded candidate fixtures directly from the
approved label CSVs, scores them with an explicit relevance config, regenerates
full-label evaluation CSVs, reapplies the existing annotations, and evaluates
the result without mutating durable DuckDB state or running a full month.

## What V4 Adds

`deterministic_v4` extends the v3 additive scorer with a compact
proximity-aware conjunction capability. Each domain may define
`proximity_rules`; each rule specifies `anchors`, `evidence`, a `window_tokens`
distance, and a `weight`. When any anchor term appears within `window_tokens`
tokens of any evidence term in the candidate text, the rule's weight is added to
the domain score and `proximity:<rule-name>` is emitted in `decision_reasons`.

Three general, pattern-based rules were added:

- `travel_safety_loss`: travel anchors (hostel, hotel, airline, luggage, flight)
  near loss/safety evidence (stolen, theft, robbed, lost, missing).
- `travel_border_security`: border anchors (customs, border, immigration) near
  security-process evidence (evading, vetted, questioned, detained, searched).
- `app_failure_evidence`: app/tool anchors (app, software, plugin, browser,
  switch, controller, phone) near failure evidence (does not work, crashes,
  broken, glitch, bug, error, workaround, fail, compatibility, impossible).

Rules are boosts, not eligibility gates. No source IDs, shard-specific
subreddit allowlists, or fixture-specific sentences were encoded.

## Commands

```bash
go test ./internal/config ./internal/relevance ./cmd/score-candidates ./cmd/evaluate-relevance

python3 scripts/dev/calibrate_relevance_fixtures.py \
  --relevance-config configs/relevance/deterministic-v4.yaml \
  --output-dir .tmp/relevance-v4-final \
  --fixture 'comments-2021-01-000|evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv|evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json' \
  --fixture 'comments-2021-01-001|evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv|evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json'
```

Config checksum:

- `configs/relevance/deterministic-v4.yaml`
  - `sha256:b9c86b936c7f6e532a19887635010012ad5ecfa12edf6b24a212a80ac0755c84`

## Results

### Regenerated full-label fixture metrics

| Fixture | Retained | True positive retained | Exact retained precision | Full-label recall estimate | Gate |
| :-- | --: | --: | --: | --: | :-- |
| `comments-2021-01-000` | `67` | `59` | `88.1%` | `59.0%` | pass |
| `comments-2021-01-001` | `38` | `31` | `81.6%` | `31.6%` | fail |
| **Combined** | **105** | **90** | **85.7%** | **45.2%** | **fail** |

### Comparison to V3

| Fixture | V3 precision | V4 precision | V3 recall | V4 recall | V3 retained | V4 retained |
| :-- | --: | --: | --: | --: | --: | --: |
| `comments-2021-01-000` | `87.3%` | `88.1%` | `55.0%` | `59.0%` | `63` | `67` |
| `comments-2021-01-001` | `79.4%` | `81.6%` | `27.6%` | `31.6%` | `34` | `38` |

V4 improves on V3 across all four metrics on both fixtures, with zero new
retained false positives.

### Per-domain retained precision

| Fixture | Travel | SaaS opportunity | App opportunity |
| :-- | --: | --: | --: |
| `comments-2021-01-000` | `86.5%` | `75.0%` | `90.0%` |
| `comments-2021-01-001` | `76.5%` | `100.0%` (`n=1`) | `75.0%` |

### Trap checks

| Fixture | Payment-brand Visa FP | Promotion/bot FP | Missing source URLs |
| :-- | --: | --: | --: |
| `comments-2021-01-000` | `0` | `0` | `0` |
| `comments-2021-01-001` | `0` | `0` | `0` |

Remaining retained false-positive mix:

- `comments-2021-01-000`: `generic_product_mention=2`, `lexical_ambiguity=3`, `political_immigration=1`
- `comments-2021-01-001`: `generic_product_mention=2`, `lexical_ambiguity=4`, `political_immigration=1`

## Representative Corrected Errors

### New true positives recovered by proximity boosts

These were evaluate-tier in V3 and retain in V4 with zero new false positives.

| Fixture | Source ID | Domain | Proximity rule | Why it now retains |
| :-- | :-- | :-- | :-- | :-- |
| `comments-2021-01-001` | `gho35dt` | travel | `travel_safety_loss` | Hostel theft/safety evidence near hostel anchor. |
| `comments-2021-01-001` | `gho4cvk` | travel | `travel_border_security` | Border security questioning near customs/border anchor. |
| `comments-2021-01-001` | `gho6r29` | travel | `travel_border_security` | Customs evasion near customs/travel anchor. |
| `comments-2021-01-001` | `gho0dil` | app | `app_failure_evidence` | Switch failure near app/software anchor. |
| `comments-2021-01-000` | `ghnnakl` | app | `app_failure_evidence` | Workaround-heavy app failure near app anchor. |
| `comments-2021-01-000` | `ghnpq8n` | app | `app_failure_evidence` | IRS app pending/failure near app anchor. |
| `comments-2021-01-000` | `ghnwgh3` | app | `app_failure_evidence` | Platform/app success-leak failure near app anchor. |
| `comments-2021-01-000` | `ghnxift` | app | `app_failure_evidence` | Messenger app failure near app anchor. |

No new retained false positives were introduced on either fixture.

## Representative Remaining Errors

### Remaining false negatives blocking the 001 recall gate

The 001 recall gap is dominated by 13 passport/citizenship/vaccine-passport
discussions at score `0.55` that are labeled travel-positive in `001` but would
require a passport-adjacency boost to retain. The same passport/citizenship
pattern is labeled travel-negative in `000` (5 passport negatives at
`score >= 0.55`), so boosting passport mentions enough to retain the 001 cases
regresses `000` below its V3 precision and violates the no-regression
constraint.

| Fixture | Source ID | True domain | Score | Why it still misses |
| :-- | :-- | :-- | --: | :-- |
| `comments-2021-01-001` | `ghnyffw` | travel | `0.55` | "most powerful passport" commentary; no proximity signal. |
| `comments-2021-01-001` | `ghnzgse` | travel | `0.55` | Passport renewal narrative; no failure/safety evidence. |
| `comments-2021-01-001` | `ghnzuwc` | travel | `0.55` | Vaccine passport mention; no border-process evidence. |
| `comments-2021-01-001` | `gho27rh` | travel | `0.55` | Vaccine passport opinion; no travel-process evidence. |
| `comments-2021-01-001` | `gho61w0` | travel | `0.55` | Dual citizenship/CCP passport; no travel-process evidence. |
| `comments-2021-01-001` | `gho7x78` | travel | `0.55` | Passport ownership statistics; no travel-process evidence. |

### Why the 001 recall gate (`50%`) is not reachable within constraints

- V4 reaches `31.6%` recall (`31/98` TP). The gate requires `49` TP.
- The remaining `18` TP needed are concentrated in `13` passport/citizenship
  candidates at score `0.55` plus `5` other boundary cases.
- Boosting passport/citizenship mentions by `+0.05` would retain the `13`
  passport FNs in `001`, but `000` has `5` passport-negatives at
  `score >= 0.55` (including `ghnwnyn` at `0.95` and `ghnoyns` at `0.75`) that
  would either stay retained or newly retain, dropping `000` precision below the
  V3 baseline and the `75%` gate.
- The two training fixtures apply opposite label conventions to the same
  passport/citizenship text pattern. This is a label-boundary conflict, not a
  scorer-expressiveness gap.
- Non-passport recoverable FNs (`24` at `score >= 0.45`) would each require
  `+0.15` boosts, which drag in `14` evaluate-tier labeled-negative candidates
  and collapse precision below `71%`.

## Decision

`deterministic_v4` is a **mixed calibration** result.

- V4 improves both observed fixtures on every metric with zero new false
  positives and no trap leakage.
- The proximity-aware conjunction capability is real, general, explainable, and
  tested. It recovers concrete travel-safety, border-security, and app-failure
  evidence that additive V3 weights could not express.
- The 001 recall gate (`50%`) cannot be met without resolving a
  label-boundary conflict between the two training fixtures on
  passport/citizenship text. That resolution is a human/planner decision, not a
  scoring-rule change.

## Exact Human/Planner Decision Needed

The two observed fixtures label passport/citizenship/vaccine-passport
discussions inconsistently:

- `comments-2021-01-001` labels `13` such candidates travel-positive.
- `comments-2021-01-000` labels `5` such candidates travel-negative.

Before any further deterministic or learned retrieval work, decide one of:

1. Treat passport/citizenship commentary as out-of-scope for the travel domain
   (align `001` labels with `000`). This lowers the `001` recall denominator and
   may let V4 pass the gate as-is.
2. Treat passport/citizenship commentary as in-scope travel evidence (align
   `000` labels with `001`). This requires a scanner or source-text context
   change to separate genuine travel-process passport pain from political
   commentary, which is beyond deterministic scoring.
3. Accept that the two fixtures are label-incompatible on this boundary and
   move to the learned retrieval fallback roadmap without further deterministic
   calibration.

Option 3 is the roadmap's recommended branch when deterministic scoring stalls
on a boundary that rules cannot resolve.

## Next Step

See
`.agent/tasks/2026-06-18-deterministic-v4-proximity-calibration/CHANGE_REQUEST.md`
for the escalation.
