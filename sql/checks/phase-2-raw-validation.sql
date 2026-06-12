-- Phase 2 raw layer validation queries.
-- Run these after a smoke or pilot ingest from the repository root.

-- Row counts by record type.
SELECT 'comments' AS record_type, count(*) AS row_count
FROM read_parquet('data/raw/comments/year=*/month=*/*.parquet')
UNION ALL
SELECT 'submissions' AS record_type, count(*) AS row_count
FROM read_parquet('data/raw/submissions/year=*/month=*/*.parquet');

-- Provenance fields required for Phase 3.
SELECT
  'comments' AS record_type,
  count(*) FILTER (WHERE source_file IS NULL OR source_file = '') AS missing_source_file,
  count(*) FILTER (WHERE ingested_at IS NULL) AS missing_ingested_at,
  count(*) FILTER (WHERE manifest_id IS NULL OR manifest_id = '') AS missing_manifest_id
FROM read_parquet('data/raw/comments/year=*/month=*/*.parquet')
UNION ALL
SELECT
  'submissions' AS record_type,
  count(*) FILTER (WHERE source_file IS NULL OR source_file = '') AS missing_source_file,
  count(*) FILTER (WHERE ingested_at IS NULL) AS missing_ingested_at,
  count(*) FILTER (WHERE manifest_id IS NULL OR manifest_id = '') AS missing_manifest_id
FROM read_parquet('data/raw/submissions/year=*/month=*/*.parquet');

-- Distinct subreddits in the raw pilot output.
SELECT 'comments' AS record_type, count(DISTINCT lower(subreddit)) AS distinct_subreddits
FROM read_parquet('data/raw/comments/year=*/month=*/*.parquet')
UNION ALL
SELECT 'submissions' AS record_type, count(DISTINCT lower(subreddit)) AS distinct_subreddits
FROM read_parquet('data/raw/submissions/year=*/month=*/*.parquet');

-- Inspect schema for both record types.
DESCRIBE SELECT * FROM read_parquet('data/raw/comments/year=*/month=*/*.parquet');
DESCRIBE SELECT * FROM read_parquet('data/raw/submissions/year=*/month=*/*.parquet');
