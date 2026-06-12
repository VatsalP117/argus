-- Phase 4 signal-layer validation queries.

SELECT signal_type, count(*) AS row_count
FROM read_parquet('data/marts/research_signals/year=*/month=*/*.parquet')
GROUP BY signal_type
ORDER BY row_count DESC, signal_type;

SELECT
  count(*) FILTER (WHERE source_id IS NULL OR source_id = '') AS missing_source_id,
  count(*) FILTER (WHERE subreddit IS NULL OR subreddit = '') AS missing_subreddit,
  count(*) FILTER (WHERE evidence_text IS NULL OR evidence_text = '') AS missing_evidence_text,
  count(*) FILTER (WHERE signal_run_id IS NULL OR signal_run_id = '') AS missing_signal_run_id
FROM read_parquet('data/marts/research_signals/year=*/month=*/*.parquet');

SELECT entity_type, count(*) AS row_count
FROM read_parquet('data/marts/entity_mentions/year=*/month=*/*.parquet')
GROUP BY entity_type
ORDER BY row_count DESC, entity_type;

SELECT
  count(*) FILTER (WHERE normalized_entity IS NULL OR normalized_entity = '') AS missing_normalized_entity,
  count(*) FILTER (WHERE source_id IS NULL OR source_id = '') AS missing_source_id
FROM read_parquet('data/marts/entity_mentions/year=*/month=*/*.parquet');

SELECT
  count(*) AS metric_rows,
  count(*) FILTER (WHERE pain_point_count > 0) AS rows_with_pain_points,
  count(*) FILTER (WHERE feature_request_count > 0) AS rows_with_feature_requests
FROM read_parquet('data/marts/subreddit_metrics_daily/year=*/month=*/*.parquet');

DESCRIBE SELECT * FROM read_parquet('data/marts/research_signals/year=*/month=*/*.parquet');
DESCRIBE SELECT * FROM read_parquet('data/marts/entity_mentions/year=*/month=*/*.parquet');
DESCRIBE SELECT * FROM read_parquet('data/marts/subreddit_metrics_daily/year=*/month=*/*.parquet');
