# ADR-002: Durable DuckDB And Ephemeral Raw Staging

## Status

Accepted

## Date

2026-06-13

## Context

The Argus proof of concept used:

- local Parquet for raw, clean, and mart layers
- in-memory DuckDB connections for transformation and querying
- subreddit-filtered ingestion

The next product phase must:

- scan broader archive slices without trusting a curated subreddit list
- retain only research-relevant documents
- support fast local search, themes, opportunities, and LLM-backed analysis
- stay below a `30 GB` durable data ceiling
- permit up to `70 GB` of temporary working storage

Keeping complete raw, clean, mart, and database copies would spend the storage budget on duplicated data rather than research coverage.

## Decision

Argus will use:

- remote versioned Parquet as the recoverable upstream source
- bounded local Parquet files as temporary batch staging
- `data/argus.duckdb` as the durable local research corpus
- manifests, checkpoints, reconciliation reports, and snapshots for reproducibility

Subreddit will be treated as a relevance feature, not as a required ingest predicate.

Raw staging may be deleted only after:

- source identity is pinned
- durable rows preserve original text and provenance
- row-count reconciliation passes
- transactional database writes commit
- post-write validation passes
- the completed checkpoint is written

## Consequences

Positive:

- more durable storage is available for relevant research evidence
- broader communities can be scanned without retaining all Reddit rows
- local queries and product workflows become simpler and faster
- staging disk remains bounded and reusable
- batch failures remain recoverable from manifests

Negative:

- the local database is no longer a byte-for-byte copy of every scanned row
- source reproducibility depends on correctly pinned remote revisions
- lifecycle and reconciliation logic become critical infrastructure
- database snapshots and exports are required for local disaster recovery
- changing retention rules may require rescanning remote archive shards

## Rejected Alternatives

### Keep every full raw month locally

Rejected because one 2021 Reddit month is about `19-21 GB` of compressed Parquet before clean copies, derived data, and spill space.

### Store only cleaned Parquet

Rejected because the product needs transactional ingestion state, durable saved research, theme relationships, and convenient local querying.

### Store all remotely scanned rows in DuckDB

Rejected because the `30 GB` durable budget would be consumed rapidly by irrelevant data.

### Use ClickHouse or a cloud warehouse

Rejected until measured concurrency or query-latency bottlenecks justify additional infrastructure.

## Revisit Conditions

Revisit this decision if:

- the upstream archive can no longer be reproduced reliably
- the active durable corpus exceeds `30 GB` despite retention controls
- DuckDB write coordination blocks the local application
- representative queries remain too slow after compaction and indexing improvements
- multi-user or always-on serving becomes a real requirement
