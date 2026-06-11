INSTALL httpfs;
LOAD httpfs;

DESCRIBE
SELECT *
FROM read_parquet('hf://datasets/open-index/arctic/data/comments/2005/12/*.parquet');

DESCRIBE
SELECT *
FROM read_parquet('hf://datasets/open-index/arctic/data/submissions/2005/12/*.parquet');
