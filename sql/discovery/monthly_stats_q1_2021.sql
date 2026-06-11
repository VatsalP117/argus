INSTALL httpfs;
LOAD httpfs;

SELECT *
FROM read_csv_auto('https://huggingface.co/datasets/open-index/arctic/resolve/main/stats.csv')
WHERE year = 2021
  AND month IN (1, 2, 3)
ORDER BY month, type;
