# ADR-001: Local-First DuckDB Baseline

## Status

Accepted

## Date

2026-06-11

## Context

Argus needs to prove that Reddit-based market research workflows are useful before taking on the cost and operational burden of a dedicated warehouse or always-on infrastructure.

Constraints:

- low budget preferred
- local development machine available
- around 100 GB free SSD now available
- no requirement for multi-user serving in v1
- no need for real-time ingestion in v1

## Decision

Argus v1 will use:

- DuckDB for archive discovery and local analytics
- Parquet for raw, clean, and mart storage
- local execution as the default operating model

Argus will not use ClickHouse in phase 0 or phase 1.

## Why

DuckDB plus Parquet is the right tradeoff because it:

- avoids fixed monthly infra cost
- supports remote Hugging Face Parquet reads
- keeps artifacts portable
- shortens iteration loops for early research workflows
- reduces the risk of overbuilding before the outputs are useful

## Consequences

Positive:

- simpler local development
- lower operating cost
- easier reproducibility
- easier deletion and rebuild of intermediate data

Negative:

- weaker concurrency story
- fewer warehouse-style serving features
- broad historical backfills may become slower or less ergonomic later

## Revisit Conditions

Revisit this decision only if measured evidence shows one of the following:

- local disk becomes the main bottleneck
- local query runtimes become unacceptable for core workflows
- a shared always-on dataset is needed
- materialized serving tables or higher concurrency become necessary
