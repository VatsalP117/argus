# Broad Candidate Shard Sample

Date: `2026-06-13`

## Scope

This report covers one complete bounded lifecycle: candidate scan, deterministic scoring, durable commit, reconciliation, validation, and staging cleanup.

- dataset: `open-index/arctic`
- archive revision: `e0041e37cb7bf2a761b6506dd530df9a18716c64`
- manifest: `pilot_travel_q1_2021-7243e041a19b43d4`
- entry: `comments-2021-01-000`
- source object: `14057ec27e0268fb0259bbe6659bca89ea0e4e0c`
- source size: `39,307,034` bytes
- candidate rules: `broad_v1`

## Results

| Metric | Result |
| :-- | --: |
| Source rows seen | 500,000 |
| Candidate rows | 10,909 |
| Rows rejected early | 489,091 |
| Candidate yield | 2.18% |
| Candidate Parquet size | 4,629,711 bytes |
| Represented subreddits | 3,794 |
| Subreddit-prior-only candidates | 205 |

Rule-group matches overlap:

| Rule group | Matches |
| :-- | --: |
| Pain language | 2,667 |
| Product and tool language | 2,616 |
| Comparison and dissatisfaction | 2,397 |
| Travel language | 1,258 |
| Pricing and payment | 1,228 |
| Business workflow language | 682 |
| Workaround language | 531 |
| Request intent | 39 |

The scan retained relevant-looking rows outside the configured subreddit priors, including software/workflow discussions and travel/embassy language from unrelated communities.

## Precision Notes

Stage A is intentionally high recall and remains noisy. Ambiguous terms such as `better than`, `billing`, `extension`, `platform`, and generic pain language admit rows that later relevance scoring should reject.

An initial run produced `11,520` candidates. Excluding URLs from rule matching reduced this to `10,909`, removing `611` obvious URL-driven matches such as `utm_name=ios_app` while preserving original text.

## Deterministic Scoring

`deterministic_v1` produced:

| Metric | Result |
| :-- | --: |
| Domain score rows | 32,727 |
| Retained candidates | 73 |
| C-tier evaluation candidates | 677 |
| Discarded candidates | 10,159 |
| A-tier domain rows | 4 |
| B-tier domain rows | 79 |
| C-tier domain rows | 844 |
| D-tier domain rows | 31,800 |

Bot-like moderation rows are explicitly flagged during scanning and score zero. Generic travel terms require a stronger travel anchor before they can retain.

Manual inspection still found important false positives:

- `Visa` can mean the payment brand rather than a travel document.
- political immigration discussion is not automatically travel-product pain.
- broad workflow and product vocabulary can describe unrelated technical or hobby contexts.

These findings block a wider ingest until a labelled evaluation set calibrates `deterministic_v2`.

## Durable Commit

The transaction wrote:

| Durable record | Count |
| :-- | --: |
| Documents | 73 |
| Domain relevance rows | 219 |
| Signals | 81 |
| Entity terms | 175 |
| Ingest batches | 1 |

Validation results:

- `500,000 = 489,091 + 10,909`
- `10,909 = 73 + 10,836 + 0`
- retained bot rows: `0`
- missing source URLs: `0`
- non-Reddit source URLs: `0`
- missing author hashes: `0`
- durable checksum: `81aaf3e6a9f03d615ebd013e096f87ff171d26983c8aeb370b0350cd37a74384`

Repeating the commit returned `skipped_existing` without duplicate rows.

## Cleanup

After post-write validation, audited cleanup removed:

- candidate staging: `4,629,711` bytes
- score staging: `136,923` bytes
- total: `4,766,634` bytes

Two completed cleanup events remain durable. Repeating cleanup returned `skipped_existing`, and DuckDB still returned all documents, relevance rows, signals, entities, and source backlinks after both Parquet files were deleted.

## Reproducibility

- source identity: `hf-shard-sha256:6a0771bc7ac9ed7ef0675ec70e1d22b12dd6bbfe17394efea98d855cc9abad90`
- candidate config hash: `sha256:bae1bafe84cf987496ea9bd7bd365f633e51ab8c4712286e12c67478851fa10d`
- candidate output hash: `sha256:c403fa91e5d4f52c02b2e1bbdd2c76101db6a4b04fafd5a6844c77b2f0fe52f2`
- score output hash: `sha256:80498248833572ada9fe38bfbb4a51c1f0da917e8afe987809a97915066f24c6`
- relevance config hash: `sha256:93e397778aa38b4564084cd7e21d93251314f5c8cc19ccb926e516473e8385d6`

Unchanged scan, commit, and cleanup retries returned `skipped_existing` at their respective lifecycle stages.

## Decision

The storage lifecycle is approved. Do not widen to another shard or full month yet. Build a labelled relevance set from retained, C-tier, and rejected rows; calibrate `deterministic_v2`; then repeat this same one-shard lifecycle before widening.
