# Argus Data Engineering Resources

## Knowledge

- [DuckDB official documentation](https://duckdb.org/docs/)
  Use for: DuckDB SQL, in-process analytics, Parquet reads, memory/thread settings, and local analytical workflows.
- [DuckDB Parquet documentation](https://duckdb.org/docs/current/data/parquet/overview.html)
  Use for: columnar file reads, Parquet projection/filter behavior, and why Argus can query files directly.
- [Apache Parquet overview](https://parquet.apache.org/docs/)
  Use for: the storage-format mental model behind remote archive shards and temporary staging files.
- [dbt guide: modular data modeling techniques](https://www.getdbt.com/blog/modular-data-modeling-techniques)
  Use for: staged data modeling vocabulary, transformation layering, and why readable, modular SQL matters.
- [Great Expectations: GX Core](https://greatexpectations.io/)
  Use for: data quality language, validation expectations, and the habit of documenting data checks.
- [Great Expectations Data Docs reference](https://docs.greatexpectations.io/docs/0.18/reference/learn/terms/data_docs/)
  Use for: the idea that validation results can become shared documentation.
- [Designing Data-Intensive Applications by Martin Kleppmann](https://www.oreilly.com/library/view/designing-data-intensive-applications/9781491903063/)
  Use for: durable systems thinking, batch processing, data models, and reliability vocabulary.
- [Local-first software essay](https://www.inkandswitch.com/local-first/)
  Use for: the broader philosophy behind keeping useful data and computation local where practical.

## Wisdom (Communities)

- [DuckDB Discord](https://duckdb.org/community/)
  Use for: practical questions about DuckDB behavior, performance, and file querying.
- [r/dataengineering](https://www.reddit.com/r/dataengineering/)
  Use for: comparing Argus design tradeoffs against common industry practice.
- [dbt Community](https://www.getdbt.com/community)
  Use for: analytics engineering patterns, modular SQL habits, and data modeling discussions.

## Gaps
- Add project-specific notes from maintainers once the current intended roadmap becomes clearer.
- Add known Argus operational incidents after real runs fail in interesting ways.
