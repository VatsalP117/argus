INSTALL httpfs;
LOAD httpfs;

SELECT id, author, subreddit, body, score, created_at
FROM read_parquet('hf://datasets/open-index/arctic/data/comments/2005/12/*.parquet')
LIMIT 5;

SELECT id, author, subreddit, title, num_comments, created_at
FROM read_parquet('hf://datasets/open-index/arctic/data/submissions/2005/12/*.parquet')
LIMIT 5;
