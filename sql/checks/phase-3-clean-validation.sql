-- Phase 3 clean layer validation queries.

-- Row counts by clean record type.
SELECT 'comments' AS record_type, count(*) AS row_count
FROM read_parquet('data/clean/comments/year=*/month=*/*.parquet')
UNION ALL
SELECT 'submissions' AS record_type, count(*) AS row_count
FROM read_parquet('data/clean/submissions/year=*/month=*/*.parquet');

-- Traceability and null checks.
SELECT
  'comments' AS record_type,
  count(*) FILTER (WHERE raw_id IS NULL OR raw_id = '') AS missing_raw_id,
  count(*) FILTER (WHERE source_file IS NULL OR source_file = '') AS missing_source_file,
  count(*) FILTER (WHERE clean_run_id IS NULL OR clean_run_id = '') AS missing_clean_run_id,
  count(*) FILTER (WHERE cleaned_at IS NULL) AS missing_cleaned_at
FROM read_parquet('data/clean/comments/year=*/month=*/*.parquet')
UNION ALL
SELECT
  'submissions' AS record_type,
  count(*) FILTER (WHERE raw_id IS NULL OR raw_id = '') AS missing_raw_id,
  count(*) FILTER (WHERE source_file IS NULL OR source_file = '') AS missing_source_file,
  count(*) FILTER (WHERE clean_run_id IS NULL OR clean_run_id = '') AS missing_clean_run_id,
  count(*) FILTER (WHERE cleaned_at IS NULL) AS missing_cleaned_at
FROM read_parquet('data/clean/submissions/year=*/month=*/*.parquet');

-- Duplicate id checks.
SELECT 'comments' AS record_type, count(*) AS duplicate_id_groups
FROM (
  SELECT id
  FROM read_parquet('data/clean/comments/year=*/month=*/*.parquet')
  GROUP BY id
  HAVING count(*) > 1
)
UNION ALL
SELECT 'submissions' AS record_type, count(*) AS duplicate_id_groups
FROM (
  SELECT id
  FROM read_parquet('data/clean/submissions/year=*/month=*/*.parquet')
  GROUP BY id
  HAVING count(*) > 1
);

-- Rows that were deduplicated from overlapping raw artifacts.
SELECT 'comments' AS record_type, count(*) FILTER (WHERE raw_duplicate_count > 1) AS deduped_rows
FROM read_parquet('data/clean/comments/year=*/month=*/*.parquet')
UNION ALL
SELECT 'submissions' AS record_type, count(*) FILTER (WHERE raw_duplicate_count > 1) AS deduped_rows
FROM read_parquet('data/clean/submissions/year=*/month=*/*.parquet');

-- Empty text and timestamp plausibility checks.
SELECT
  'comments' AS record_type,
  count(*) FILTER (WHERE text_length = 0 AND NOT is_deleted AND NOT is_removed) AS unexpected_empty_text,
  count(*) FILTER (WHERE created_at IS NULL) AS invalid_created_at
FROM read_parquet('data/clean/comments/year=*/month=*/*.parquet')
UNION ALL
SELECT
  'submissions' AS record_type,
  count(*) FILTER (WHERE text_length = 0 AND NOT is_deleted AND NOT is_removed) AS unexpected_empty_text,
  count(*) FILTER (WHERE created_at IS NULL) AS invalid_created_at
FROM read_parquet('data/clean/submissions/year=*/month=*/*.parquet');

-- Deleted/removed/bot-like ratios for spot checking.
SELECT
  'comments' AS record_type,
  avg(CAST(is_deleted AS DOUBLE)) AS deleted_ratio,
  avg(CAST(is_removed AS DOUBLE)) AS removed_ratio,
  avg(CAST(is_bot_like AS DOUBLE)) AS bot_like_ratio
FROM read_parquet('data/clean/comments/year=*/month=*/*.parquet')
UNION ALL
SELECT
  'submissions' AS record_type,
  avg(CAST(is_deleted AS DOUBLE)) AS deleted_ratio,
  avg(CAST(is_removed AS DOUBLE)) AS removed_ratio,
  avg(CAST(is_bot_like AS DOUBLE)) AS bot_like_ratio
FROM read_parquet('data/clean/submissions/year=*/month=*/*.parquet');

-- Schema inspection.
DESCRIBE SELECT * FROM read_parquet('data/clean/comments/year=*/month=*/*.parquet');
DESCRIBE SELECT * FROM read_parquet('data/clean/submissions/year=*/month=*/*.parquet');
