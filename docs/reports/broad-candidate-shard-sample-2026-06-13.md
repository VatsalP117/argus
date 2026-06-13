# Broad Candidate Shard Sample

Date: `2026-06-13`

## Scope

This report covers only the Stage A candidate scan. It does not validate relevance scoring, durable commit, or staging cleanup.

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
| Candidate Parquet size | 4,628,452 bytes |
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

## Reproducibility

- source identity: `hf-shard-sha256:6a0771bc7ac9ed7ef0675ec70e1d22b12dd6bbfe17394efea98d855cc9abad90`
- candidate config hash: `sha256:bae1bafe84cf987496ea9bd7bd365f633e51ab8c4712286e12c67478851fa10d`
- staged output hash: `sha256:c21e60a8e8dbebd8cd9c98c1ea525ecf4a9b3a9039581ee2069c20883eb4097a`

An unchanged retry returned `skipped_existing` after validating the checkpoint and staged output checksum.

## Decision

Proceed to transactional `cmd/commit-candidates` and deterministic relevance scoring. Do not widen to a full month yet, and do not delete this staging output until durable commit, reconciliation, and post-write validation exist.
