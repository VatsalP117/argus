WITH pain_points AS (
  SELECT
    subreddit,
    coalesce(topic_hint, 'unclassified') AS topic_hint
  FROM research_signals
  WHERE signal_type = 'pain_point'
)
SELECT
  subreddit,
  count(*) AS pain_point_count,
  count(DISTINCT topic_hint) AS distinct_topics
FROM pain_points
GROUP BY subreddit
ORDER BY pain_point_count DESC, subreddit
