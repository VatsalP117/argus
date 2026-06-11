INSTALL httpfs;
LOAD httpfs;

SELECT count(*) AS comments_rows
FROM read_parquet('hf://datasets/open-index/arctic/data/comments/2021/01/*.parquet');

SELECT count(*) AS submissions_rows
FROM read_parquet('hf://datasets/open-index/arctic/data/submissions/2021/01/*.parquet');

SELECT min(created_at) AS min_created_at, max(created_at) AS max_created_at
FROM read_parquet('hf://datasets/open-index/arctic/data/comments/2021/01/*.parquet');

SELECT min(created_at) AS min_created_at, max(created_at) AS max_created_at
FROM read_parquet('hf://datasets/open-index/arctic/data/submissions/2021/01/*.parquet');
