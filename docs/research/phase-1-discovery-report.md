# Phase 1 Discovery Report

## Date

2026-06-11

## Scope

This report records what was actually validated during Phase 1 for Argus.

## Archive Confirmations

Confirmed from the dataset card and local access validation:

- dataset: `open-index/arctic`
- shape: Reddit comments and submissions
- layout: monthly Parquet shards
- path pattern:
  - `data/comments/<year>/<month>/<shard>.parquet`
  - `data/submissions/<year>/<month>/<shard>.parquet`
- access pattern works with DuckDB:
  - `read_parquet('hf://datasets/open-index/arctic/...')`

## Published Dataset Scale

Published on the dataset page at validation time:

- total items: `12.1B`
- comments: `9.7B`
- submissions: `2.4B`
- total compressed Parquet size: `1.1 TB`

## Local Validation Performed

Locally validated with DuckDB:

1. installed DuckDB Python package
2. loaded DuckDB `httpfs` extension
3. successfully queried comments sample rows from:
   - `hf://datasets/open-index/arctic/data/comments/2005/12/*.parquet`
4. successfully queried submissions sample rows from:
   - `hf://datasets/open-index/arctic/data/submissions/2005/12/*.parquet`
5. inspected locally returned schema fields for both record types

## Sample Local Findings

Sample comment fields verified locally:

- `id`
- `author`
- `subreddit`
- `body`
- `score`
- `created_utc`
- `created_at`
- `body_length`
- `link_id`
- `parent_id`

Sample submission fields verified locally:

- `id`
- `author`
- `subreddit`
- `title`
- `selftext`
- `score`
- `created_utc`
- `created_at`
- `title_length`
- `num_comments`
- `url`
- `over_18`

## Important Archive Caveat

During local path validation, a direct probe for comment files under `data/comments/2023/` failed with `404`.

Inference:

- do not assume all newer comment years are equally available until path-level checks confirm them
- define the first pilot in years that are already known-good across both record types

## Planning Implication

The first pilot should be defined by:

- known-good months
- a small set of target subreddits
- a smoke-first ingest sequence

This is why the current pilot is `travel`, `Q1 2021`, with `2021-01` as the smoke month.

## What Phase 1 Did Not Do

Phase 1 did not yet:

- generate a real shard-by-shard manifest
- ingest any raw data locally
- compute exact travel-subreddit row counts across the whole quarter

Those belong to Phase 2, where the pipeline can checkpoint and measure actual filtered output safely.
