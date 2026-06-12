WITH signal_counts AS (
  SELECT
    subreddit,
    topic_hint,
    matched_pattern,
    count(*) AS signal_count
  FROM read_parquet('data/marts/research_signals/year=*/month=*/*.parquet')
  WHERE signal_type = 'pain_point'
  GROUP BY subreddit, topic_hint, matched_pattern
),
ranked_evidence AS (
  SELECT
    subreddit,
    topic_hint,
    matched_pattern,
    source_type,
    source_id,
    created_at,
    left(evidence_text, 300) AS evidence_snippet,
    row_number() OVER (
      PARTITION BY subreddit, topic_hint, matched_pattern
      ORDER BY created_at DESC, source_id
    ) AS evidence_rank
  FROM read_parquet('data/marts/research_signals/year=*/month=*/*.parquet')
  WHERE signal_type = 'pain_point'
)
SELECT
  signal_counts.subreddit,
  signal_counts.topic_hint,
  signal_counts.matched_pattern,
  signal_counts.signal_count,
  ranked_evidence.source_type,
  ranked_evidence.source_id,
  ranked_evidence.created_at,
  ranked_evidence.evidence_snippet
FROM signal_counts
LEFT JOIN ranked_evidence
  ON signal_counts.subreddit = ranked_evidence.subreddit
 AND coalesce(signal_counts.topic_hint, '') = coalesce(ranked_evidence.topic_hint, '')
 AND signal_counts.matched_pattern = ranked_evidence.matched_pattern
 AND ranked_evidence.evidence_rank = 1
ORDER BY signal_counts.signal_count DESC, signal_counts.subreddit, signal_counts.matched_pattern
LIMIT 50;
