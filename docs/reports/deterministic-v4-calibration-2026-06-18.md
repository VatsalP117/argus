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

## Label Reconciliation

A human decision resolved the passport/citizenship label-boundary conflict
documented in the original `CHANGE_REQUEST.md`:

> For the current travel scorer, only concrete travel/process/document pain is
> travel-positive. Abstract passport/citizenship/vaccine-passport commentary is
> out of scope for the current travel scorer, but should be noted as a possible
> future adjacent research domain.

Seventeen labels were reconciled across both fixtures. All changes were within
the passport/citizenship/vaccine-passport boundary. No other labels were
altered.

### Fixture `comments-2021-01-001` — 13 labels changed from travel-positive to travel-negative

| Source ID | Category | Rationale |
| :-- | :-- | :-- |
| `ghnyffw` | `lexical_ambiguity` | "most powerful passport" statistics; no travel-process pain. |
| `ghnzgse` | `lexical_ambiguity` | Passport renewal personal narrative; no blocker, rejection, or pain. |
| `ghnzuwc` | `lexical_ambiguity` | Vaccine passport opinion; no travel-process evidence. |
| `ghnzyj9` | `lexical_ambiguity` | "green passport" = vaccine passport; no travel-process evidence. |
| `gho27rh` | `lexical_ambiguity` | "in favour of a vaccine passport" opinion; no travel-process evidence. |
| `gho3a2c` | `lexical_ambiguity` | Vaccine passport social/political commentary; no travel-process evidence. |
| `gho6xp8` | `lexical_ambiguity` | Passport expiring/citizenship status confusion; no concrete travel pain. |
| `gho6yri` | `lexical_ambiguity` | Metaphorical passport mention in relationship drama. |
| `gho7x78` | `lexical_ambiguity` | Passport ownership statistics; no travel-process pain. |
| `gho2jxh` | `political_immigration` | Irish identity/citizenship commentary; no travel-process pain. |
| `gho2pav` | `political_immigration` | CCP passport/dual citizenship policy commentary. |
| `gho61w0` | `political_immigration` | Dual citizenship policy commentary. |
| `gho6n9l` | `political_immigration` | Chinese-Australian citizenship terminology discussion. |

### Fixture `comments-2021-01-000` — 4 labels changed from travel-positive to travel-negative

| Source ID | Category | Rationale |
| :-- | :-- | :-- |
| `ghnrtqg` | (uncategorized) | Passport application success story; no pain, blocker, or rejection. |
| `ghnoe6b` | `political_immigration` | Residency/tax policy commentary about a political figure. |
| `ghnpjw5` | `political_immigration` | Same residency/tax policy commentary as `ghnoe6b`. |
| `ghnroa4` | `political_immigration` | Immigration reform policy opinion; no personal travel-process pain. |

### Cases kept as travel-positive (concrete travel-document/process pain)

| Fixture | Source ID | Why it stays travel-positive |
| :-- | :-- | :-- |
| `comments-2021-01-001` | `gho16zq` | Getting passport + plane ticket to travel to specific countries. |
| `comments-2021-01-001` | `gho85ro` | Getting passport/visa for an international move. |
| `comments-2021-01-001` | `gho4cvk` | Border security questioning with temporary passport. |
| `comments-2021-01-000` | `ghnp10c` | Visa requirements for entering a country with restrictions. |
| `comments-2021-01-000` | `ghnqsd4` | Green card impossibility (60-100 year wait); immigration-process pain. |
| `comments-2021-01-000` | `ghnr4u1` | Seeking asylum; passport expired. |
| `comments-2021-01-000` | `ghnshr2` | H1B visa chances as a passport holder; diversity visa. |
| `comments-2021-01-000` | `ghnsxlz` | Airport, passport, boarding pass, plane — concrete travel process. |
| `comments-2021-01-000` | `ghnu5io` | Which passport to use when entering/exiting countries. |

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
  --output-dir .tmp/relevance-v4-label-reconciled \
  --fixture 'comments-2021-01-000|evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv|evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json' \
  --fixture 'comments-2021-01-001|evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv|evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json'
```

Config checksum:

- `configs/relevance/deterministic-v4.yaml`
  - `sha256:b9c86b936c7f6e532a19887635010012ad5ecfa12edf6b24a212a80ac0755c84`

Label file checksums (after reconciliation):

- `evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json`
  - `sha256:9b995c344df49a88075e07b82d911d37704763191c983c13182664bd03ce3b5b`
- `evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv`
  - `sha256:8d9e3d975dba93f8e2c32936a45b3ead4f6295391373c7d3f4e140783c667b4c`
- `evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json`
  - `sha256:b63bbbeab6539b61e840580a415c29b545da2623fa99371e6de21281db26ef4b`
- `evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv`
  - `sha256:190882bcfaa79238539b13177ef0e23641da3e914cda27b3d9cbc0c7b1875b9e`

## Results

### Regenerated full-label fixture metrics (after label reconciliation)

| Fixture | Retained | True positive retained | Exact retained precision | Full-label recall estimate | Gate |
| :-- | --: | --: | --: | --: | :-- |
| `comments-2021-01-000` | `67` | `57` | `85.1%` | `59.4%` | pass |
| `comments-2021-01-001` | `38` | `31` | `81.6%` | `36.5%` | fail |
| **Combined** | **105** | **88** | **83.8%** | **40.9%** | **fail** |

### Comparison to V3 (same reconciled labels)

| Fixture | V3 precision | V4 precision | V3 recall | V4 recall | V3 retained | V4 retained |
| :-- | --: | --: | --: | --: | --: | --: |
| `comments-2021-01-000` | `84.1%` | `85.1%` | `55.2%` | `59.4%` | `63` | `67` |
| `comments-2021-01-001` | `79.4%` | `81.6%` | `31.8%` | `36.5%` | `34` | `38` |

V4 still improves on V3 across all four metrics on both fixtures with the
reconciled labels. V4 does not regress below V3 on `comments-2021-01-000`
precision (`85.1%` vs `84.1%`).

### Per-domain retained precision

| Fixture | Travel | SaaS opportunity | App opportunity |
| :-- | --: | --: | --: |
| `comments-2021-01-000` | `81.1%` | `75.0%` | `90.0%` |
| `comments-2021-01-001` | `76.5%` | `100.0%` (`n=1`) | `75.0%` |

### Trap checks

| Fixture | Payment-brand Visa FP | Promotion/bot FP | Missing source URLs |
| :-- | --: | --: | --: |
| `comments-2021-01-000` | `0` | `0` | `0` |
| `comments-2021-01-001` | `0` | `0` | `0` |

Remaining retained false-positive mix:

- `comments-2021-01-000`: `generic_product_mention=2`, `lexical_ambiguity=3`, `political_immigration=3`
- `comments-2021-01-001`: `generic_product_mention=2`, `lexical_ambiguity=4`, `political_immigration=1`

The `000` political-immigration count increased from `1` to `3` because
`ghnoe6b` and `ghnpjw5` (residency policy commentary) are retained by the scorer
at score `0.85` but are now labeled travel-negative. This is expected: the
scorer has not changed, only the labels. The `3/67 = 4.5%` rate is well below
the `20%` category gate.

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

No new retained false positives were introduced on either fixture beyond the
two `000` cases (`ghnoe6b`, `ghnpjw5`) that became false positives due to label
reconciliation rather than scorer changes.

## Remaining Errors

### Remaining false negatives blocking the 001 recall gate

After label reconciliation, `001` has `85` total positives and `31` retained
true positives, yielding `36.5%` recall against the `50%` gate. The remaining
`54` false negatives break down as:

| Score bucket | Count | Boost needed to retain |
| :-- | --: | :-- |
| `0.55-0.60` | `5` | `+0.05` |
| `0.50-0.55` | `3` | `+0.10` |
| `0.45-0.50` | `13` | `+0.15` |
| `0.40-0.45` | `5` | `+0.20` |
| `<0.40` | `28` | `+0.20+` (hard) |

By true domain: `travel=23`, `app=25`, `saas=6`.

Recovering the `5` candidates at `0.55-0.60` would raise recall to `36/85 =
42.4%`. Recovering through `0.50` would raise it to `39/85 = 45.9%`. Reaching
the `50%` gate requires `43` TP, meaning `12` more candidates must be retained.
This requires boosting candidates at `0.45-0.50` by `+0.15`, which the original
analysis showed drags in `14` evaluate-tier labeled-negative candidates and
collapses precision below `71%`.

### Why the 001 recall gate (`50%`) is still not reachable

- The passport/citizenship label boundary is resolved. Those cases are no longer
  positives and no longer count against recall.
- The remaining recall gap is a deterministic ceiling: too many genuine
  positives sit at `0.45-0.55` where boosting them to retain requires
  `+0.10` to `+0.15` score increases, which would also retain labeled-negative
  candidates at the same scores and collapse precision below the `75%` gate.
- This is not a label-boundary issue or a scorer-expressiveness issue that
  proximity rules can fix. It is evidence that additive deterministic scoring
  has reached its recall ceiling on this fixture.

## Decision

`deterministic_v4` remains a **mixed calibration** result after label
reconciliation.

- The passport/citizenship label-boundary conflict is resolved. Both fixtures
  now apply the same convention: abstract passport/citizenship commentary is
  not travel-positive; concrete travel-document/process pain is.
- V4 improves both observed fixtures on every metric versus V3 on the same
  reconciled labels, with no trap leakage and no precision gate violation.
- The `001` recall gate (`50%`) is still not met (`36.5%`). The remaining gap
  is a deterministic ceiling, not a label-boundary conflict.
- Abstract passport/citizenship/vaccine-passport commentary should be noted as
  a possible future adjacent research domain, per the human decision.

## Next Step

The passport/citizenship boundary is resolved. The remaining blocker is a
deterministic recall ceiling on `comments-2021-01-001`.

See
`.agent/tasks/2026-06-18-deterministic-v4-proximity-calibration/CHANGE_REQUEST.md`
for the updated escalation. The recommended direction is now the roadmap's
learned retrieval fallback: keep deterministic candidate retrieval as the
high-recall front door and add a lightweight learned reranker or classifier to
recover the `0.45-0.55` tier candidates that deterministic scoring cannot
boost without precision collapse.
