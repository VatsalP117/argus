WITH request_signals AS (
  SELECT
    subreddit,
    signal_type,
    topic_hint,
    matched_pattern,
    count(*) AS signal_count
  FROM read_parquet('data/marts/research_signals/year=*/month=*/*.parquet')
  WHERE signal_type IN ('feature_request', 'recommendation_request', 'workaround')
  GROUP BY subreddit, signal_type, topic_hint, matched_pattern
),
ranked_evidence AS (
  SELECT
    subreddit,
    signal_type,
    topic_hint,
    matched_pattern,
    source_type,
    source_id,
    created_at,
    left(evidence_text, 300) AS evidence_snippet,
    row_number() OVER (
      PARTITION BY subreddit, signal_type, topic_hint, matched_pattern
      ORDER BY created_at DESC, source_id
    ) AS evidence_rank
  FROM read_parquet('data/marts/research_signals/year=*/month=*/*.parquet')
  WHERE signal_type IN ('feature_request', 'recommendation_request', 'workaround')
)
SELECT
  request_signals.subreddit,
  request_signals.signal_type,
  request_signals.topic_hint,
  request_signals.matched_pattern,
  request_signals.signal_count,
  ranked_evidence.source_type,
  ranked_evidence.source_id,
  ranked_evidence.created_at,
  ranked_evidence.evidence_snippet
FROM request_signals
LEFT JOIN ranked_evidence
  ON request_signals.subreddit = ranked_evidence.subreddit
 AND request_signals.signal_type = ranked_evidence.signal_type
 AND coalesce(request_signals.topic_hint, '') = coalesce(ranked_evidence.topic_hint, '')
 AND request_signals.matched_pattern = ranked_evidence.matched_pattern
 AND ranked_evidence.evidence_rank = 1
ORDER BY request_signals.signal_count DESC, request_signals.subreddit, request_signals.signal_type
LIMIT 50;
