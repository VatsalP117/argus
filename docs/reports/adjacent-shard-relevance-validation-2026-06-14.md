# Adjacent Shard Relevance Validation Report

Date: `2026-06-14`

## 1. Source Identity and Scope

This report validates `deterministic_v2` on exactly one unseen adjacent archive shard:

| Field | Value |
| :-- | :-- |
| Entry ID | `comments-2021-01-001` |
| Record type | `comments` |
| Month | `2021-01` |
| Local source path | `data/comments/2021/01/001.parquet` |
| Source size | `38,777,883` bytes |
| Archive revision | `e0041e37cb7bf2a761b6506dd530df9a18716c64` |
| Source identity | `hf-shard-sha256:19b9f30d324da78465c05736696a3c4df565f0f6df8d6ab7a3e5727bf366af79` |
| Manifest | `/tmp/argus-adjacent-comments-pinned.json` |
| Manifest ID | `pilot_travel_q1_2021-7243e041a19b43d4` |

No other shard was scanned. The main `data/argus.duckdb` was not mutated.

## 2. Scan Yield and Reconciliation

```bash
go run ./cmd/scan-candidates \
  --manifest /tmp/argus-adjacent-comments-pinned.json \
  --entry-id comments-2021-01-001 \
  --output-path data/tmp/candidates/adjacent-comments-2021-01-001.parquet
```

| Metric | Value |
| :-- | --: |
| Rows seen | `500,000` |
| Candidate rows | `10,493` |
| Early rejects | `489,507` |
| Candidate yield | `2.10%` |
| Candidate Parquet bytes | `4,499,998` |
| Candidate Parquet SHA256 | `a94de7b7c1deb1a2fd087a66d34e137a88db7b8f37267e6f0cdb01e23696fa04` |
| Checkpoint path | `state/checkpoints/candidate-scan/pilot_travel_q1_2021-7243e041a19b43d4/comments-2021-01-001.json` |
| Subreddit-prior candidates | `173` |

Reconciliation:

```text
rows_seen (500,000) = rows_candidates (10,493) + rows_rejected_early (489,507)  ✓
```

All hard stop conditions passed:

- rows seen is non-zero
- checkpoint source identity matches the manifest
- reconciliation equation holds
- candidate yield (2.10%) is below 5%
- candidate output (4.5 MB) is below 250 MB

Matches by rule group:

| Rule group | Candidates |
| :-- | --: |
| `business_workflow_language` | 636 |
| `comparison_and_dissatisfaction` | 2,281 |
| `pain_language` | 2,591 |
| `pricing_and_payment` | 1,235 |
| `product_and_tool_language` | 2,601 |
| `request_intent` | 40 |
| `travel_language` | 1,128 |
| `workaround_language` | 537 |

## 3. Scoring Distribution

```bash
go run ./cmd/score-candidates \
  --scan-checkpoint state/checkpoints/candidate-scan/pilot_travel_q1_2021-7243e041a19b43d4/comments-2021-01-001.json \
  --relevance-config configs/relevance/deterministic-v2.yaml \
  --output-path data/tmp/candidates/adjacent-comments-2021-01-001-scores-v2.parquet
```

| Metric | Value |
| :-- | --: |
| Scored candidates | `10,493` |
| Scored domain rows | `31,479` (`10,493 × 3`) |
| Retained candidates | `51` |
| Evaluation candidates | `267` |
| Discarded candidates | `10,175` |
| Tier A | `10` |
| Tier B | `41` |
| Tier C | `301` |
| Tier D | `31,127` |
| Score file bytes | `131,234` |
| Score file SHA256 | `6d73d7b1bf796d2fc799244281d7904fa057495e7a963045ca9b84ee767d953a` |
| Relevance config SHA256 | `aa68128123b89ccbead049c7420bd9bb37961df0493230357825785713296bda` |
| Min/Max score | `0.0` / `0.9` |
| Unique candidate/domain pairs | `31,479` |

All hard stop conditions passed:

- scored candidates match scan candidates
- scored domain rows equal `candidate rows × 3`
- retained candidates (51) are below 250
- all scores are within `[0, 1]`
- no duplicate candidate/domain pairs

The `deterministic-v2.yaml` config was not edited after observing this shard.

## 4. Labelled Fixture Composition

```bash
go run ./cmd/export-relevance-eval \
  --scan-checkpoint state/checkpoints/candidate-scan/pilot_travel_q1_2021-7243e041a19b43d4/comments-2021-01-001.json \
  --score-path data/tmp/candidates/adjacent-comments-2021-01-001-scores-v2.parquet \
  --output-path evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv \
  --retain-sample 0 \
  --evaluate-sample 100 \
  --discard-sample 100 \
  --seed adjacent-comments-2021-01-001-v2
```

| Stratum | Population | Sampled | Positive labels |
| :-- | --: | --: | --: |
| Retain | `51` | `51` | `28` |
| Evaluate | `267` | `100` | `59` |
| Discard | `10,175` | `100` | `11` |
| **Total** | **10,493** | **251** | **98** |

Exported population metadata columns:

- `stratum_population`
- `sample_rank`
- `sampling_seed`

Labels were applied with:

```bash
python3 scripts/dev/apply_relevance_annotations.py \
  --input-path evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv \
  --annotations-path evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json \
  --output-path evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv
```

The labels are executor-reviewed engineering labels under `market-research-relevance-v1`. They are not independent human ground truth.

## 5. Exact Retained Precision

| Metric | Value |
| :-- | --: |
| Retained predictions | `51` |
| True-positive retained | `28` |
| False-positive retained | `23` |
| **Exact retained precision** | **54.9%** |

The exact retained precision uses all 51 labelled retained rows.

## 6. Fixture Recall and Weighted Recall Estimate

| Metric | Value |
| :-- | --: |
| Fixture recall (unweighted) | `28.6%` |
| Estimated total relevant candidates | `1,304.78` |
| **Weighted retained recall estimate** | **2.1%** |

The fixture recall is the unweighted recall on the labelled sample:

```text
fixture_recall = true_positive_retained / labeled_positive = 28 / 98 = 28.6%
```

The weighted retained recall estimate extrapolates from each stratum's population and sampled positive rate:

```text
estimated_total_relevant = 51 × 0.5490 + 267 × 0.5900 + 10,175 × 0.1100 = 1,304.78
weighted_recall = 28 / 1,304.78 = 2.1%
```

These two quantities are clearly distinguished. Fixture recall measures the labelled sample; weighted recall estimates population recall.

## 7. Per-Domain Metrics

| Domain | Retained predictions | True positives | Precision | Recall (fixture) |
| :-- | --: | --: | --: | --: |
| Travel | `25` | `14` | `56.0%` | `28.0%` |
| SaaS opportunity | `2` | `1` | `50.0%` | `12.5%` |
| App opportunity | `24` | `11` | `45.8%` | `25.6%` |
| **Candidate (any domain)** | **51** | **28** | **54.9%** | **28.6%** |

Domains with at least 10 retained predictions and precision below 60%:

- `travel`: 56.0%
- `app_opportunity`: 45.8%

## 8. False-Positive Category Counts

Retained false positives by category:

| Category | Count | Share of retained rows |
| :-- | --: | --: |
| `lexical_ambiguity` | `11` | 21.6% |
| `generic_product_mention` | `6` | 11.8% |
| `political_immigration` | `4` | 7.8% |
| `promotion_or_bot` | `2` | 3.9% |
| `payment_brand_visa` | `0` | 0.0% |

The `lexical_ambiguity` category exceeds the 20% share gate (21.6%).

Other trap checks:

| Check | Value | Gate |
| :-- | --: | --: |
| Retained rows missing source URL | `0` | = 0 |
| Retained payment-brand Visa FPs | `0` | = 0 |
| Retained promotion/bot FPs | `2` | = 0 (failed) |

## 9. Representative Relevant Rows with Backlinks

Retained true positives:

| Source ID | Subreddit | Predicted domain | Label | Excerpt | Source URL |
| :-- | :-- | :-- | :-- | :-- | :-- |
| `gho1ap6` | `cozumel` | travel | travel | Airport shuttle, customs, hotel, and tour guidance for Cozumel. | https://www.reddit.com/comments/ko15wb/_/gho1ap6 |
| `gho3444` | `apple` | app_opportunity | app | DisplayLink driver/extension completely breaks external monitors on Big Sur. | https://www.reddit.com/comments/jw9qqh/_/gho3444 |
| `gho303s` | `FinancialCareers` | saas_opportunity | saas | Banks using Excel/Word hacks and outdated regulations for compliance. | https://www.reddit.com/comments/knpytc/_/gho303s |
| `ghnzr0o` | `GalaxyNote20` | app_opportunity | app | In-app camera fails weekly, requiring phone restart. | https://www.reddit.com/comments/kmbcd3/_/ghnzr0o |
| `gho706f` | `Seattle` | travel | travel | Express Entry permanent residency and visa timing discussion. | https://www.reddit.com/comments/knvugx/_/gho706f |

## 10. Representative False Positives and False Negatives with Backlinks

### Representative retained false positives

| Source ID | Subreddit | Predicted domain | FP category | Why it was not relevant | Source URL |
| :-- | :-- | :-- | :-- | :-- | :-- |
| `ghny7nd` | `funny` | travel | `political_immigration` | H1B visa policy discussion, not actionable personal travel. | https://www.reddit.com/comments/ko0rah/_/ghny7nd |
| `ghnyv42` | `flying` | app_opportunity | `lexical_ambiguity` | Career advice about aviation degrees; "software" is career backup, not a software pain. | https://www.reddit.com/comments/ko1orn/_/ghnyv42 |
| `ghnz41y` | `BedBros` | app_opportunity | `lexical_ambiguity` | Sleep schedule advice; "app" and "sync" refer to alarm clocks and body clocks. | https://www.reddit.com/comments/knw31z/_/ghnz41y |
| `gho2efu` | `shakepay` | app_opportunity | `promotion_or_bot` | Referral-code promotion for Shakepay Bitcoin rewards. | https://www.reddit.com/comments/jz8zc3/_/gho2efu |
| `gho7voh` | `nbn` | app_opportunity | `promotion_or_bot` | ISP recommendation including referral code. | https://www.reddit.com/comments/knntn4/_/gho7voh |
| `gho85o4` | `malaysia` | travel | `lexical_ambiguity` | Infrastructure/policy debate about KL-Singapore flights and HSR, not personal travel planning. | https://www.reddit.com/comments/ko2buc/_/gho85o4 |

### Representative false negatives (relevant but not retained)

| Source ID | Subreddit | True domain | Why it is relevant | Source URL |
| :-- | :-- | :-- | :-- | :-- |
| `ghnxzf4` | `electronicmusic` | app | Spotify links open in browser instead of the app. | https://www.reddit.com/comments/knz9s9/_/ghnxzf4 |
| `ghnyc6d` | `TiviMate` | app | TiviMate app bug prevents playback menu from opening. | https://www.reddit.com/comments/kfjaxj/_/ghnyc6d |
| `ghnygj7` | `Twitch` | app | Streaming software limitations on Chromebook. | https://www.reddit.com/comments/ko0p9z/_/ghnygj7 |
| `gho4cvk` | `interestingasfuck` | travel | Personal border-security experience with temporary passport. | https://www.reddit.com/comments/knwvmx/_/gho4cvk |
| `gho59fg` | `ontario` | travel | ArriveCAN app and airline gate-agent testing enforcement for travel. | https://www.reddit.com/comments/knzacw/_/gho59fg |

## 11. Isolated Lifecycle Results

**Not performed.** Quality gates failed, so the isolated temporary DuckDB lifecycle proof was skipped as required by the runbook.

The temporary database path would have been `/tmp/argus-adjacent-v2-validation.duckdb`. No temporary database was created or committed.

## 12. Limitations

1. **Agent-reviewed labels**: The 251 labels are engineering labels produced by a Codex agent using `market-research-relevance-v1`. They are not independent human ground truth.
2. **One-shard sample**: The validation uses a single adjacent shard (`comments-2021-01-001`). Results may not generalize to other shards or the full month.
3. **Stratified sampling uncertainty**: The weighted recall estimate depends on 100-row samples from the evaluate and discard strata. The discard stratum in particular has a large population (`10,175`) and a low positive rate (`11%`), so the estimate has wide uncertainty.
4. **Heuristic labels for non-retained strata**: Retained rows were manually reviewed; evaluate and discard rows were classified with a rule-based helper that can mislabel ambiguous cases (e.g., "passport" jokes, vaccine-passport policy, airline-industry discussion).

## 13. Decision

`failed_validation`

`deterministic_v2` did **not** pass the adjacent-shard quality gates:

| Gate | Required | Observed | Status |
| :-- | --: | --: | :-- |
| Exact retained precision | ≥ 70% | 54.9% | ❌ fail |
| Weighted retained recall estimate | ≥ 60% | 2.1% | ❌ fail |
| Domain precision (≥10 retained) | ≥ 60% | travel 56.0%, app 45.8% | ❌ fail |
| Retained bot rows | = 0 | 0 | ✓ pass |
| Retained missing source URLs | = 0 | 0 | ✓ pass |
| Retained payment-brand Visa FPs | = 0 | 0 | ✓ pass |
| Retained promotion/bot FPs | = 0 | 2 | ❌ fail |
| No single FP category > 20% of retained | ≤ 20% | lexical 21.6% | ❌ fail |

Because the gates failed, no data was committed to `data/argus.duckdb` and no temporary lifecycle proof was run. `deterministic-v2.yaml` was not modified after observing this shard.

### Recommended next step

Start a separate `v3` calibration cycle using a new training/validation split:

1. Reserve at least two unseen shards strictly for validation; do not use them for threshold tuning.
2. Build a larger, independently spot-checked label set across retain, evaluate, and discard strata.
3. Address the dominant error modes observed here:
   - `lexical_ambiguity` (21.6% of retained rows): tighten context requirements for words like "itinerary", "customs", "passport", "airline", "app", and "sync".
   - `promotion_or_bot`: add referral-code and affiliate-language penalties.
   - Low recall in evaluate/discard: lower thresholds or add recall-oriented boosts for concrete bug, workaround, and travel-process language so relevant candidates are not dropped to discard.
4. Re-validate on a fresh adjacent shard before any full-month run.

Do not authorize a full-month run until a successor scorer passes all gates on an unseen shard.
