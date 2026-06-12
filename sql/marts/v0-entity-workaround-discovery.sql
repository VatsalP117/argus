WITH entity_counts AS (
  SELECT
    entity_type AS result_category,
    normalized_entity AS result_label,
    count(*) AS result_count
  FROM read_parquet('data/marts/entity_mentions/year=*/month=*/*.parquet')
  GROUP BY entity_type, normalized_entity
),
ranked_entity_evidence AS (
  SELECT
    entity_type AS result_category,
    normalized_entity AS result_label,
    source_type,
    source_id,
    subreddit,
    created_at,
    left(evidence_text, 300) AS evidence_snippet,
    row_number() OVER (
      PARTITION BY entity_type, normalized_entity
      ORDER BY created_at DESC, source_id
    ) AS evidence_rank
  FROM read_parquet('data/marts/entity_mentions/year=*/month=*/*.parquet')
),
workaround_counts AS (
  SELECT
    coalesce(topic_hint, 'unclassified') AS result_category,
    matched_pattern AS result_label,
    count(*) AS result_count
  FROM read_parquet('data/marts/research_signals/year=*/month=*/*.parquet')
  WHERE signal_type = 'workaround'
  GROUP BY coalesce(topic_hint, 'unclassified'), matched_pattern
),
ranked_workaround_evidence AS (
  SELECT
    coalesce(topic_hint, 'unclassified') AS result_category,
    matched_pattern AS result_label,
    source_type,
    source_id,
    subreddit,
    created_at,
    left(evidence_text, 300) AS evidence_snippet,
    row_number() OVER (
      PARTITION BY coalesce(topic_hint, 'unclassified'), matched_pattern
      ORDER BY created_at DESC, source_id
    ) AS evidence_rank
  FROM read_parquet('data/marts/research_signals/year=*/month=*/*.parquet')
  WHERE signal_type = 'workaround'
),
entity_results AS (
  SELECT
    'entity' AS result_kind,
    entity_counts.result_category,
    entity_counts.result_label,
    entity_counts.result_count,
    ranked_entity_evidence.source_type,
    ranked_entity_evidence.source_id,
    ranked_entity_evidence.subreddit,
    ranked_entity_evidence.created_at,
    ranked_entity_evidence.evidence_snippet,
    row_number() OVER (
      PARTITION BY 'entity'
      ORDER BY entity_counts.result_count DESC, entity_counts.result_category, entity_counts.result_label
    ) AS result_rank
  FROM entity_counts
  LEFT JOIN ranked_entity_evidence
    ON entity_counts.result_category = ranked_entity_evidence.result_category
   AND entity_counts.result_label = ranked_entity_evidence.result_label
   AND ranked_entity_evidence.evidence_rank = 1
),
workaround_results AS (
  SELECT
    'workaround' AS result_kind,
    workaround_counts.result_category,
    workaround_counts.result_label,
    workaround_counts.result_count,
    ranked_workaround_evidence.source_type,
    ranked_workaround_evidence.source_id,
    ranked_workaround_evidence.subreddit,
    ranked_workaround_evidence.created_at,
    ranked_workaround_evidence.evidence_snippet,
    row_number() OVER (
      PARTITION BY 'workaround'
      ORDER BY workaround_counts.result_count DESC, workaround_counts.result_category, workaround_counts.result_label
    ) AS result_rank
  FROM workaround_counts
  LEFT JOIN ranked_workaround_evidence
    ON workaround_counts.result_category = ranked_workaround_evidence.result_category
   AND workaround_counts.result_label = ranked_workaround_evidence.result_label
   AND ranked_workaround_evidence.evidence_rank = 1
)
SELECT
  result_kind,
  result_category,
  result_label,
  result_count,
  source_type,
  source_id,
  subreddit,
  created_at,
  evidence_snippet
FROM entity_results
WHERE result_rank <= 20

UNION ALL

SELECT
  result_kind,
  result_category,
  result_label,
  result_count,
  source_type,
  source_id,
  subreddit,
  created_at,
  evidence_snippet
FROM workaround_results
WHERE result_rank <= 10

ORDER BY result_kind, result_count DESC, result_category, result_label;
