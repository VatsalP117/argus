# Phase 2 Optimization Plan

This document narrows the general optimization ideas into the Argus pipeline that exists today.

## What We Are Optimizing

The current pain is not final filtered storage size. It is the shape of the work:

- month-wide grouped reads can scan several gigabytes of remote Parquet in one DuckDB job
- shard mode starts a fresh Python and DuckDB process for every shard
- interrupted grouped runs can leave zero-byte outputs that look complete on the next retry

For the current travel pilot, that means a January run can scan more than `8 GiB` remotely while producing only a few megabytes of filtered output. The optimization goal is to make those scans bounded, resumable, and cheap to retry.

## Expected Results

### 1. Lower Peak Memory

Expected result:

- fewer out-of-memory failures
- more stable laptop runs
- predictable spill behavior when jobs exceed RAM

How:

- process exact manifest entries in bounded batches instead of one wildcard month scan
- set DuckDB `memory_limit`, `threads`, and `temp_directory`
- let DuckDB spill intermediate work to disk instead of competing for all available RAM

### 2. Better Wall-Clock Time

Expected result:

- lower startup overhead than one Python process per shard
- fewer reruns of giant failed jobs
- faster restart after interruption

How:

- batch several manifest entries into one DuckDB invocation
- keep batches small enough that failure means rerunning a few hundred megabytes, not a whole month
- preserve exact manifest URLs so we only scan what we intentionally scheduled

### 3. Safer Resume Behavior

Expected result:

- no more poisoned retries caused by empty placeholder outputs
- reprocessing only happens where it is actually needed

How:

- write to a temporary file first
- rename into place only after row count and file size checks pass
- treat zero-byte outputs as invalid and delete them before rerun

### 4. Better Operational Visibility

Expected result:

- we can finally compare months and record types using actual work-unit sizes
- future concurrency tuning can be evidence-based instead of guessed

How:

- keep manifest-level source byte counts
- checkpoint every bounded batch
- record output paths and partial failures per run

## Optimization Order

### Now

1. Replace month wildcard grouped scans with bounded manifest batches.
2. Make output writes atomic.
3. Add DuckDB runtime controls for memory, threads, and temp spill.
4. Fix resume semantics around zero-byte files.

### Next

1. Add raw validation SQL for row counts, provenance fields, and checkpoint reconciliation.
2. Add a cheap preflight mode that runs `count(*)` queries per month and record type before ingest.
3. Add controlled parallelism across batches after single-worker behavior is stable.

### Later

1. Add local month compaction as a separate post-ingest step.
2. Decide whether ClickHouse is justified only after clean-layer and research queries are measured.

## What We Expect Numerically

These are directional expectations, not guarantees:

- peak memory should be capped by the configured DuckDB budget plus spill, instead of the size of a whole wildcard month scan
- grouped retry scope should drop from "an entire month and record type" to "one bounded batch"
- January filtered output should remain tiny relative to source scans, likely in the low-megabyte range for the current travel pilot
- effective runtime should improve because we are reducing Python startup churn without paying the risk of month-sized reruns

## Why We Are Not Dropping Metadata

Argus is not a text-only warehouse. Later phases need:

- timestamps
- subreddit
- source ids
- provenance
- quality flags

That metadata is necessary for cleaning, traceability, and research workflows. The right optimization is bounded execution, not throwing away fields that future phases require.

## Learning Roadmap

If you want to go deep on this project, study these in order.

### 1. Columnar Storage Basics

Learn:

- why Parquet stores data by column instead of by row
- column pruning
- predicate pushdown
- compression basics like dictionary encoding and ZSTD

Why it matters:

- this explains why scanning six needed columns is much cheaper than scanning the full row shape

### 2. DuckDB Execution Model

Learn:

- `read_parquet(...)`
- remote scanning with `httpfs`
- spill to disk
- memory limits
- why one large scan can still be expensive even on columnar data

Why it matters:

- DuckDB is the first real execution engine in Argus

### 3. Parquet + Manifest Thinking

Learn:

- shards, partitions, and append-only datasets
- why manifests are useful for reproducibility
- resumability by work unit

Why it matters:

- Argus operates on archive shards, not on abstract rows

### 4. Batching, Worker Pools, and Backpressure

Learn:

- bounded concurrency
- queue depth
- backpressure
- why "more goroutines" is not the same as "faster"

Why it matters:

- this is the core of making the ingest path both fast and safe

### 5. Data Quality and Rebuildability

Learn:

- raw vs clean vs marts
- lineage
- checkpoints
- idempotency

Why it matters:

- the project becomes trustworthy only when every layer can be rebuilt without mystery steps

### 6. ClickHouse Later, Not First

Learn:

- MergeTree basics
- batch inserts
- materialized views
- partitioning and sort keys

Why it matters:

- ClickHouse may become useful later, but only after DuckDB plus Parquet is measured honestly

## Suggested Learning Sequence

Use this order if you want a practical path:

1. Read the Parquet and DuckDB docs at a beginner level.
2. Run small `read_parquet` queries against one Arctic shard and inspect selected columns.
3. Learn how manifests and checkpoints map a huge archive into retryable work units.
4. Learn batch processing and backpressure patterns in Go.
5. Move to ClickHouse concepts only after Phase 3 and Phase 4 are tangible.
