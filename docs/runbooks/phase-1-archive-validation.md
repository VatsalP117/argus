# Phase 1 Archive Validation Runbook

This runbook captures how to re-run the local archive validation that was used to prepare the current pilot definition.

## Archive Under Validation

- dataset: `open-index/arctic`
- host: Hugging Face datasets
- access pattern: remote Parquet reads via `hf://datasets/open-index/arctic/...`

## Validated On

- date: 2026-06-11
- machine: local MacBook Air
- method: DuckDB Python package with `httpfs`

## Validation Goals

1. Confirm remote archive access works locally.
2. Confirm both comments and submissions are readable.
3. Confirm schema fields needed for Argus exist.
4. Confirm the archive is monthly-sharded.
5. Confirm a practical pilot window can be chosen without full-history ingest.

## Minimal Validation Script

```python
import duckdb

con = duckdb.connect()
con.execute("INSTALL httpfs;")
con.execute("LOAD httpfs;")

comments = con.execute(
    """
    SELECT id, author, subreddit, body, score, created_at
    FROM read_parquet('hf://datasets/open-index/arctic/data/comments/2005/12/*.parquet')
    LIMIT 3
    """
).fetchall()

submissions = con.execute(
    """
    SELECT id, author, subreddit, title, num_comments, created_at
    FROM read_parquet('hf://datasets/open-index/arctic/data/submissions/2005/12/*.parquet')
    LIMIT 3
    """
).fetchall()

print(comments)
print(submissions)
```

## Key Findings Already Verified

- remote DuckDB access works locally
- comments and submissions are both readable
- comments schema includes:
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
- submissions schema includes:
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

## Important Caveat

The dataset card describes the archive broadly through 2026-02, but a direct path probe for comment data under `data/comments/2023/` returned `404` during local validation. Because of that, the first Argus pilot should target the years that are definitely available across both comments and submissions rather than assuming newer comments are ready.

## Practical Outcome

The Phase 1 pilot is defined around `Q1 2021` with a mandatory `2021-01` smoke month.
