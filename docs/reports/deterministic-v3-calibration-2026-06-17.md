# Deterministic V3 Calibration Report

Date: `2026-06-17`

Status: `failed_calibration`

## Scope

This report calibrates `deterministic_v3` using only the two already-observed
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

## Commands

```bash
go test ./internal/config ./internal/relevance ./cmd/score-candidates ./cmd/evaluate-relevance

python3 scripts/dev/calibrate_relevance_fixtures.py \
  --relevance-config configs/relevance/deterministic-v3.yaml \
  --output-dir .tmp/relevance-v3-iterC \
  --fixture 'comments-2021-01-000|evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv|evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json' \
  --fixture 'comments-2021-01-001|evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv|evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json'
```

Config checksum:

- `configs/relevance/deterministic-v3.yaml`
  - `sha256:98c3e0e11d88dcb87548b746f2e7343359600fa1970301b1122c90c62e93450f`

## Results

### Regenerated full-label fixture metrics

These are the quality-gate metrics produced by the bounded calibration runner on
the reconstructed full-label fixtures.

| Fixture | Retained | True positive retained | Exact retained precision | Full-label recall estimate | Gate |
| :-- | --: | --: | --: | --: | :-- |
| `comments-2021-01-000` | `63` | `55` | `87.3%` | `55.0%` | pass |
| `comments-2021-01-001` | `34` | `27` | `79.4%` | `27.6%` | fail |
| **Combined** | **97** | **82** | **84.5%** | **41.4%** | **fail** |

### Per-domain retained precision

| Fixture | Travel | SaaS opportunity | App opportunity |
| :-- | --: | --: | --: |
| `comments-2021-01-000` | `86.5%` | `75.0%` | `87.5%` |
| `comments-2021-01-001` | `71.4%` | `100.0%` (`n=1`) | `73.7%` |

### Trap checks

| Fixture | Payment-brand Visa FP | Promotion/bot FP | Missing source URLs |
| :-- | --: | --: | --: |
| `comments-2021-01-000` | `0` | `0` | `0` |
| `comments-2021-01-001` | `0` | `0` | `0` |

Remaining retained false-positive mix:

- `comments-2021-01-000`: `generic_product_mention=2`, `lexical_ambiguity=3`, `political_immigration=1`
- `comments-2021-01-001`: `generic_product_mention=2`, `lexical_ambiguity=4`, `political_immigration=1`

## Representative Corrected Errors

### Corrected retained false positives

These were retained by the v2 bounded baseline and no longer retain in v3.

| Fixture | Source ID | Prior FP category | Why this is an improvement |
| :-- | :-- | :-- | :-- |
| `comments-2021-01-001` | `ghny7nd` | `political_immigration` | `H1B` labor/policy argument no longer retains as travel pain. |
| `comments-2021-01-001` | `gho0me7` | `generic_product_mention` | Generic Android malware advice no longer retains as app opportunity. |
| `comments-2021-01-001` | `gho1f9p` | `lexical_ambiguity` | Airline price-history commentary no longer retains as travel pain. |
| `comments-2021-01-000` | `ghnod4m` | `political_immigration` | Illegal-immigration policy debate no longer retains as travel. |
| `comments-2021-01-000` | `ghnw8ha` | `lexical_ambiguity` | “Government doesn’t work” no longer retains as app failure. |

## Representative Remaining Errors

### Remaining false negatives

| Fixture | Source ID | True domain | Why it still misses |
| :-- | :-- | :-- | :-- |
| `comments-2021-01-001` | `gho35dt` | travel | Concrete hostel theft/travel safety stays below retain threshold. |
| `comments-2021-01-001` | `ghnyffw` | travel | Passport/travel-document mention remains ambiguous under current travel penalties. |
| `comments-2021-01-001` | `gho6zxy` | travel | Hostel/travel-experience evidence still lands in evaluate. |
| `comments-2021-01-000` | `ghnnakl` | app | Concrete workaround-heavy app failure still does not retain. |
| `comments-2021-01-000` | `ghnqvok` | app | Geolocation-expiry failure remains below the current app retain threshold. |

### Remaining retained false positives

| Fixture | Source ID | FP category | Why it still leaks |
| :-- | :-- | :-- | :-- |
| `comments-2021-01-001` | `gho0pgw` | `generic_product_mention` | Generic Chromecast/controller explanation still looks like broken-app evidence. |
| `comments-2021-01-001` | `ghnyv42` | `lexical_ambiguity` | Career-advice “software backup plan” still trips app dissatisfaction language. |
| `comments-2021-01-001` | `ghnzj8s` | `lexical_ambiguity` | Encoding/receiver discussion still trips `does not work` app cues. |
| `comments-2021-01-000` | `ghnqkyw` | `generic_product_mention` | Operating-system/tool comparison still retains as app opportunity. |
| `comments-2021-01-000` | `ghnvhn3` | `political_immigration` | Migration-rights advocacy still retains as travel. |

## Decision

`deterministic_v3` is **not** ready for unseen validation.

Why it stops here:

- The bounded tracked candidate meets the regenerated full-label gate on
  `comments-2021-01-000`.
- It does **not** meet the recall target on `comments-2021-01-001`
  (`27.6%` vs required `50%`).
- Raising recall further with broad threshold changes would risk threshold
  collapse and contradict the calibration guardrails.
- Additional progress now appears to require a more expressive scorer design
  rather than more ad hoc term tweaking.

## Next Step

See `.agent/tasks/2026-06-14-deterministic-v3-calibration/CHANGE_REQUEST.md`
for the proposed follow-up before continuing calibration.
