# Argus Implementation Status Report

Date: 2026-06-12

This report is written for someone who is building Argus and wants an honest, beginner-friendly picture of what exists today.

## 1. High-level summary

What has been built so far:

- The repository has moved past planning-only work.
- Phase 1 discovery is documented and validated against the public `open-index/arctic` archive.
- Phase 2 raw-ingestion tooling exists and has been run locally.
- Phase 3 clean-layer tooling exists and has been run locally for January 2021 data.

What part of the Argus architecture this belongs to:

- Mostly discovery, raw ingestion, and early cleaning.
- Not yet research enrichment, evidence exports, or a user-facing research workflow.

What phase of work we are in:

- Discovery: implemented.
- Raw ingestion: implemented and partially hardened.
- Cleaning/normalization: partially implemented.
- Research enrichment: not started in code.
- Query/access layer: only validation SQL exists, not research workflows.
- Infrastructure/warehouse: intentionally deferred.

Current working state:

- The repo can generate shard manifests from Hugging Face metadata.
- It can preflight remote row counts for selected shards.
- It can ingest filtered raw Parquet locally for chosen subreddit slices.
- It can produce a first clean layer from raw Parquet.
- It can validate raw and clean outputs with checked-in SQL.

What is runnable end-to-end today:

- A bounded data pipeline is runnable from remote archive discovery to local raw Parquet to local clean Parquet.
- A full research workflow is not runnable yet because there is no signal-enrichment job, mart builder, or evidence export job.

Concrete current data state on disk:

- Raw comments: January 2021 only, 14,795 rows.
- Raw submissions: January 2021 complete at 6,136 rows, plus partial February 2021 at 1,817 rows.
- Clean comments: January 2021 only, 9,509 rows.
- Clean submissions: January 2021 only, 6,019 rows.

Important caveat:

- The clean comments layer is behind the current raw comments layer. Clean comments were last rebuilt before all January comment raw batches finished.

## 2. Current repository state

Important folders and files:

```text
argus/
  README.md
  SPEC.md
  IMPLEMENTATION_GUIDE.md
  configs/
    domains/travel.yaml
    pipelines/pilot-travel-q1-2021.yaml
    signals/deterministic-v1.yaml
  manifests/
    archive/arctic-discovery-summary.json
    pilot/travel-q1-2021-plan.json
    pilot/travel-q1-2021-smoke-manifest.json
    pilot/travel-q1-2021-full-manifest.json
  cmd/
    manifest-builder/main.go
    ingest-worker/main.go
    phase2-preflight/main.go
    clean-transform/main.go
  internal/
    archive/huggingface.go
    manifest/manifest.go
    checkpoint/checkpoint.go
    config/pipeline.go
    runmeta/runmeta.go
  scripts/dev/
    bootstrap_duckdb.sh
    duckdb_filter_copy.py
    duckdb_count.py
    duckdb_clean_transform.py
    run_duckdb_sql_file.py
  sql/
    discovery/*.sql
    checks/*.sql
  docs/
    decisions/ADR-001-local-first-duckdb.md
    research/*.md
    runbooks/*.md
  data/
    raw/
    clean/
    marts/
    exports/
  state/
    checkpoints/
    runs/
    logs/
```

What each major area is responsible for:

- `README.md`: short status and operating model.
- `SPEC.md`: the product and architecture target.
- `IMPLEMENTATION_GUIDE.md`: phase-by-phase build order and definitions of done.
- `configs/`: the current pilot definition in machine-readable form.
- `manifests/`: archive discovery outputs and shard-by-shard ingest plans.
- `cmd/`: the actual Go CLIs.
- `internal/`: reusable code for archive listing, manifest building, config loading, checkpoints, and run metadata.
- `scripts/dev/`: DuckDB-powered helpers invoked by the Go CLIs.
- `sql/discovery/`: manual archive validation and schema inspection queries.
- `sql/checks/`: raw-layer and clean-layer validation queries.
- `docs/runbooks/`: operational instructions for rerunning each phase.
- `data/`: local storage layers.
- `state/`: checkpoints, run records, and logs.

Files to read first:

1. `README.md`
2. `docs/research/pilot-definition.md`
3. `docs/research/phase-1-discovery-report.md`
4. `docs/runbooks/phase-2-manifest-and-smoke-ingest.md`
5. `configs/pipelines/pilot-travel-q1-2021.yaml`
6. `cmd/manifest-builder/main.go`
7. `cmd/ingest-worker/main.go`
8. `scripts/dev/duckdb_filter_copy.py`
9. `docs/runbooks/phase-3-cleaning-rules.md`
10. `cmd/clean-transform/main.go`
11. `scripts/dev/duckdb_clean_transform.py`

Generated files and artifacts:

- `manifests/pilot/travel-q1-2021-smoke-manifest.json`
- `manifests/pilot/travel-q1-2021-full-manifest.json`
- `state/runs/phase2/*.json`
- `state/runs/phase3/*.json`
- `state/checkpoints/phase2/**`
- `state/checkpoints/phase3/**`
- `data/raw/**`
- `data/clean/**`

Experimental or temporary items:

- `state/logs/phase2-backfill-*.log`
- `state/logs/phase2-backfill.pid`
- `data/raw/submissions/year=2021/month=02/submissions-2021-02-part-003-filtered.parquet.tmp`
- repeated Phase 3 run records show iterative local re-runs rather than a polished final workflow

## 3. What has been implemented

### Feature: Phase 1 archive discovery

- Purpose:
  Validate that the target archive is real, remotely readable, and shaped in a way Argus can work with.
- Current behavior:
  The repo documents and checks that `open-index/arctic` can be queried via DuckDB and that both comments and submissions are available in monthly Parquet shards.
- Key files:
  `docs/research/phase-1-discovery-report.md`, `manifests/archive/arctic-discovery-summary.json`, `sql/discovery/*.sql`, `docs/runbooks/phase-1-archive-validation.md`
- How it works:
  DuckDB uses `read_parquet('hf://datasets/open-index/arctic/...')` to inspect schemas and sample rows without downloading full months first.
- How to run:
  Run the SQL in `sql/discovery/validate_hf_duckdb_access.sql` through `scripts/dev/run_duckdb_sql_file.py`.
- Current limitations:
  This is validation, not ingest. It does not compute final filtered pilot row counts by itself.

### Feature: Pilot configuration

- Purpose:
  Define a narrow first slice so the project stays local-first and measurable.
- Current behavior:
  The pilot is travel-focused, bounded to Q1 2021, with January 2021 as the smoke month and six travel-related subreddits.
- Key files:
  `configs/pipelines/pilot-travel-q1-2021.yaml`, `configs/domains/travel.yaml`, `docs/research/pilot-definition.md`
- How it works:
  The pipeline config drives the CLIs, and the domain config captures research terms and subreddits.
- How to run:
  The config is consumed automatically by the Go commands through `--pipeline`.
- Current limitations:
  Only one domain and one pilot are encoded today.

### Feature: Manifest generation

- Purpose:
  Turn a broad archive assumption like "Q1 2021 submissions and comments" into an exact list of remote Parquet files to process.
- Current behavior:
  The manifest builder hits the Hugging Face dataset tree API and writes deterministic JSON manifests containing one entry per shard.
- Key files:
  `cmd/manifest-builder/main.go`, `internal/archive/huggingface.go`, `internal/manifest/manifest.go`, `internal/manifest/manifest_test.go`
- How it works:
  The Go code lists remote files for each month and record type, sorts them, attaches size metadata and URLs, and hashes the result into a deterministic `manifest_id`.
- How to run:
  `go run ./cmd/manifest-builder --pipeline configs/pipelines/pilot-travel-q1-2021.yaml --month 2021-01 --output manifests/pilot/travel-q1-2021-smoke-manifest.json`
- Current limitations:
  The manifest contains shard metadata, not filtered row counts. Filtering happens later.

### Feature: Preflight row counting

- Purpose:
  Estimate how many rows will survive subreddit filtering before committing to a larger ingest.
- Current behavior:
  A Go CLI invokes a DuckDB Python script that counts rows matching the pilot subreddits across selected manifest entries.
- Key files:
  `cmd/phase2-preflight/main.go`, `scripts/dev/duckdb_count.py`, `docs/runbooks/phase-2-validation.md`
- How it works:
  It passes exact resolve URLs into DuckDB, loads `httpfs`, scans remote Parquet, and runs `count(*)` with a `WHERE lower(subreddit) IN (...)` filter.
- How to run:
  `go run ./cmd/phase2-preflight --pipeline configs/pipelines/pilot-travel-q1-2021.yaml --manifest manifests/pilot/travel-q1-2021-full-manifest.json --month 2021-01 --output state/runs/phase2/preflight-2021-01.json`
- Current limitations:
  It is a measurement tool only. It does not write data.

### Feature: Raw ingest worker

- Purpose:
  Copy only the selected subreddit rows from remote archive shards into local raw Parquet.
- Current behavior:
  The worker can process one shard at a time or group shards into bounded batches for a month and record type.
- Key files:
  `cmd/ingest-worker/main.go`, `scripts/dev/duckdb_filter_copy.py`, `internal/checkpoint/checkpoint.go`, `internal/runmeta/runmeta.go`
- How it works:
  The Go code selects manifest entries, optionally groups them into bounded batches, and calls DuckDB through Python. DuckDB reads remote Parquet, filters on subreddit, adds provenance columns, and writes compressed Parquet locally.
- How to run:
  `go run ./cmd/ingest-worker --pipeline configs/pipelines/pilot-travel-q1-2021.yaml --manifest manifests/pilot/travel-q1-2021-full-manifest.json --record-type comments --month 2021-01 --group-by-month --batch-size 8 --max-batch-source-bytes 536870912`
- Current limitations:
  There is no worker pool yet. Execution is single-process and sequential. Automatic retry logic does not exist.

### Feature: Checkpoints and run records

- Purpose:
  Make long-running work resumable and auditable.
- Current behavior:
  Every manifest build, preflight, ingest, and clean run writes JSON run metadata. Ingest and clean steps also write checkpoint files.
- Key files:
  `internal/checkpoint/checkpoint.go`, `internal/runmeta/runmeta.go`, `docs/runbooks/run-metadata-schema.md`, `state/checkpoints/**`, `state/runs/**`
- How it works:
  The CLIs write machine-readable JSON with status, timestamps, inputs, outputs, and row counts.
- How to run:
  These files are written automatically during pipeline runs.
- Current limitations:
  Grouped-entry checkpoints record status and output paths, but not per-entry row counts. Run IDs are only second-granularity, which has already caused collisions.

### Feature: Clean transform

- Purpose:
  Convert source-faithful raw rows into normalized, analysis-friendly Parquet while preserving traceability.
- Current behavior:
  A Go CLI invokes a DuckDB Python script that deduplicates, normalizes text, flags deleted/removed/bot-like content, and adds clean-layer lineage fields.
- Key files:
  `cmd/clean-transform/main.go`, `scripts/dev/duckdb_clean_transform.py`, `docs/runbooks/phase-3-cleaning-rules.md`, `sql/checks/phase-3-clean-validation.sql`
- How it works:
  DuckDB reads local raw Parquet, deduplicates by `id`, keeps the newest `ingested_at`, adds cleaned text columns and boolean flags, and writes local clean Parquet.
- How to run:
  `go run ./cmd/clean-transform --pipeline configs/pipelines/pilot-travel-q1-2021.yaml --record-type comments --month 2021-01`
- Current limitations:
  Only comments and submissions transforms exist. No language detection, no entity extraction, and no research signal generation yet.

### Feature: Validation SQL

- Purpose:
  Give a repeatable way to inspect whether raw and clean outputs look structurally correct.
- Current behavior:
  Checked-in SQL files report row counts, null checks, schema shape, duplicate groups, and deleted/removed/bot-like ratios.
- Key files:
  `sql/checks/phase-2-raw-validation.sql`, `sql/checks/phase-3-clean-validation.sql`, `scripts/dev/run_duckdb_sql_file.py`
- How it works:
  The Python helper opens DuckDB locally and executes each SQL statement in order.
- How to run:
  `python3 scripts/dev/run_duckdb_sql_file.py --sql-file sql/checks/phase-2-raw-validation.sql`
- Current limitations:
  These are validation checks, not business-facing research queries.

## 4. Architecture explanation

Simple current data flow:

```text
open-index/arctic remote Parquet
-> discovery SQL and archive validation
-> pilot config
-> exact shard manifest JSON
-> preflight row counts
-> raw ingest worker
-> local raw Parquet
-> clean transform
-> local clean Parquet
-> (future) deterministic research signals
-> (future) marts and evidence exports
-> (future) notebook/API/query layer
```

### 1. Discovery layer

- Status:
  Implemented.
- What code exists:
  `sql/discovery/*.sql`, `docs/research/phase-1-discovery-report.md`, `manifests/archive/arctic-discovery-summary.json`
- What data flows through it:
  Remote Hugging Face Parquet metadata and sample rows.
- What it outputs:
  Schema validation, archive assumptions, and pilot planning inputs.
- What is missing:
  Automated discovery of best months or subreddits. Right now discovery is manual and documented.

### 2. Raw ingestion layer

- Status:
  Implemented and partially hardened.
- What code exists:
  `cmd/manifest-builder`, `cmd/phase2-preflight`, `cmd/ingest-worker`, `internal/archive`, `internal/manifest`, `scripts/dev/duckdb_filter_copy.py`, checkpoints, run records
- What data flows through it:
  Exact remote shard URLs from the manifest.
- What it outputs:
  Local Parquet files under `data/raw/<record_type>/year=YYYY/month=MM/`
- What is missing:
  Automatic retries, concurrency control, disk safety enforcement in code, and a polished backfill driver.

### 3. Cleaning/normalization layer

- Status:
  Partially implemented.
- What code exists:
  `cmd/clean-transform`, `scripts/dev/duckdb_clean_transform.py`, cleaning rules docs, validation SQL
- What data flows through it:
  Local raw Parquet from Phase 2.
- What it outputs:
  Local clean Parquet with normalized text, dedupe handling, and flags like `is_deleted`, `is_removed`, and `is_bot_like`.
- What is missing:
  A complete rebuild across all ingested raw months, richer normalization, language handling, and stable scheduling.

### 4. Research enrichment layer

- Status:
  Not started in code.
- What code exists:
  Only the config stub `configs/signals/deterministic-v1.yaml`
- What data flows through it:
  Nothing yet.
- What it outputs:
  Nothing yet.
- What is missing:
  Actual enrich-signals CLI, signal schema, rules execution, and QA.

### 5. Query/access layer

- Status:
  Partially implemented, but only for QA and validation.
- What code exists:
  `sql/checks/*.sql`, `sql/discovery/*.sql`
- What data flows through it:
  Local raw and clean Parquet, plus remote archive data during discovery.
- What it outputs:
  Validation tuples and schema inspection.
- What is missing:
  Research queries for pain points, app ideas, market maps, and evidence export.

### 6. Optional ClickHouse/warehouse layer

- Status:
  Not started and intentionally deferred.
- What code exists:
  ADRs and specs only.
- What data flows through it:
  None.
- What it outputs:
  None.
- What is missing:
  Everything. This is a later scaling decision, not current implementation.

## 5. Concepts I need to understand

### Parquet

- What it means:
  A columnar file format commonly used for analytics.
- Why Argus needs it:
  Reddit archive data is large, and Parquet lets us scan only the parts we need.
- Where it appears:
  Everywhere: remote archive files, local raw files, and local clean files.
- Argus example:
  `data/raw/comments/year=2021/month=01/comments-2021-01-part-004-filtered.parquet`

### Columnar storage

- What it means:
  Data is stored by column instead of by whole row.
- Why Argus needs it:
  Analytics often read a few columns from many rows, which is cheaper in columnar form.
- Where it appears:
  In the archive and in all local Parquet layers.
- Argus example:
  Discovery SQL can read only `id`, `author`, `subreddit`, `body`, `score`, and `created_at` instead of every column.

### Row group

- What it means:
  A row group is an internal chunk inside a Parquet file.
- Why Argus needs it:
  It affects how efficiently DuckDB can scan and skip data.
- Where it appears:
  Implicitly inside remote and local Parquet; current code does not tune row groups directly.
- Argus example:
  We batch by file today, not by row group. Row groups matter under the hood, but are not a first-class planning unit yet.

### Column pruning

- What it means:
  Reading only the columns a query needs.
- Why Argus needs it:
  It reduces remote reads and memory usage.
- Where it appears:
  In discovery SQL and validation SQL that select specific columns.
- Argus example:
  `sql/discovery/validate_hf_duckdb_access.sql` selects a narrow set of fields from remote Parquet.

### Predicate pushdown

- What it means:
  Letting the storage/query engine apply filters early so less data is read.
- Why Argus needs it:
  Argus filters by subreddit and later will filter by time, domain, and quality flags.
- Where it appears:
  The ingest and preflight scripts filter on `lower(subreddit) IN (...)`.
- Argus example:
  `scripts/dev/duckdb_filter_copy.py` filters remote archive rows before writing local Parquet.

### Partitioning

- What it means:
  Organizing data into predictable directory or logical partitions.
- Why Argus needs it:
  It keeps the local lake simple to navigate and query by month and record type.
- Where it appears:
  `data/raw/<record_type>/year=YYYY/month=MM/` and `data/clean/<record_type>/year=YYYY/month=MM/`
- Argus example:
  January comments live under `data/raw/comments/year=2021/month=01/`

### DuckDB

- What it means:
  An embedded analytical database that can query files directly.
- Why Argus needs it:
  It gives local-first analytics without standing up a warehouse.
- Where it appears:
  Discovery, preflight, raw copy, clean transform, and validation helpers.
- Argus example:
  The Python scripts import DuckDB and run SQL against remote and local Parquet.

### `read_parquet`

- What it means:
  A DuckDB table function that treats Parquet files like queryable tables.
- Why Argus needs it:
  It lets Argus query remote or local Parquet without loading it into a database first.
- Where it appears:
  All SQL and Python DuckDB helpers.
- Argus example:
  `SELECT count(*) FROM read_parquet('data/raw/comments/year=*/month=*/*.parquet')`

### `httpfs`

- What it means:
  A DuckDB extension that allows HTTP-backed file access.
- Why Argus needs it:
  Hugging Face archive reads depend on it.
- Where it appears:
  `scripts/dev/duckdb_count.py`, `scripts/dev/duckdb_filter_copy.py`, discovery SQL
- Argus example:
  `INSTALL httpfs; LOAD httpfs;`

### Remote Parquet scans

- What it means:
  Querying Parquet files over the network instead of downloading them all first.
- Why Argus needs it:
  It keeps discovery and selective ingest cheap.
- Where it appears:
  Phase 1 validation, preflight, and raw ingest.
- Argus example:
  The ingest worker passes exact Hugging Face resolve URLs into DuckDB.

### Manifest

- What it means:
  A JSON plan listing exactly which source files belong to a run.
- Why Argus needs it:
  Reproducibility, resumability, and safe backfills all depend on knowing the exact work units.
- Where it appears:
  `manifests/pilot/travel-q1-2021-*.json`
- Argus example:
  The full manifest has 1,122 exact shard entries totaling about 48.6 GB of remote Parquet.

### Shard

- What it means:
  One Parquet file that represents a slice of a month and record type.
- Why Argus needs it:
  Shards are the practical unit of planning and retry in this project.
- Where it appears:
  Manifest entries like `data/comments/2021/01/023.parquet`
- Argus example:
  `entry_id: comments-2021-01-023`

### Checkpoint

- What it means:
  A state file recording progress for a work unit.
- Why Argus needs it:
  Long runs should resume safely instead of starting from scratch.
- Where it appears:
  `state/checkpoints/phase2/**` and `state/checkpoints/phase3/**`
- Argus example:
  A grouped ingest checkpoint records part status, output path, and rows written.

### Idempotency

- What it means:
  Rerunning a job should not corrupt results or duplicate data.
- Why Argus needs it:
  Local batch jobs will be interrupted and re-run often.
- Where it appears:
  Existing non-empty output files are skipped unless `--force` is used.
- Argus example:
  `existingUsableOutput` removes zero-byte files and accepts non-empty files as reusable.

### Resumability

- What it means:
  The ability to continue a partially completed job.
- Why Argus needs it:
  Full pilot backfills are too large to treat as one fragile all-or-nothing run.
- Where it appears:
  Grouped ingest batches, per-batch checkpoints, skip-existing behavior.
- Argus example:
  January submissions were built across earlier and later batch runs, and later runs skipped already-written parts.

### Lineage

- What it means:
  Knowing where each row came from and which run produced it.
- Why Argus needs it:
  Research claims need evidence back to source rows.
- Where it appears:
  `source_file`, `manifest_id`, `raw_id`, `clean_run_id`, run records, checkpoints
- Argus example:
  Clean rows retain `raw_id` and `source_file`, so you can trace a cleaned record back to its remote source shard.

### Raw layer

- What it means:
  Local data that stays close to the source but is filtered into the pilot scope.
- Why Argus needs it:
  It is the rebuildable landing zone before cleaning rules change.
- Where it appears:
  `data/raw/**`
- Argus example:
  Raw comment files contain archive columns plus `source_file`, `ingested_at`, and `manifest_id`.

### Clean layer

- What it means:
  Local data that is normalized and quality-annotated for analysis.
- Why Argus needs it:
  Research queries become much easier when deleted content, duplicates, bots, and text cleanup are handled consistently.
- Where it appears:
  `data/clean/**`
- Argus example:
  Clean submissions add `title_clean`, `selftext_clean`, `combined_text`, `is_deleted`, `is_removed`, `is_bot_like`, and `clean_run_id`.

### Marts / derived tables

- What it means:
  Research-focused tables built from cleaned data.
- Why Argus needs it:
  Repeated workflows like pain-point counts should not recalculate everything each time.
- Where it appears:
  Only as planned directories and spec language today.
- Argus example:
  `data/marts/` exists, but no marts are implemented yet.

### Worker pool

- What it means:
  A coordinated set of workers processing tasks concurrently.
- Why Argus needs it:
  It will matter when Phase 2 moves from single-process sequential work to controlled parallel backfills.
- Where it appears:
  Only in specs and runbooks today, not in code.
- Argus example:
  `cmd/ingest-worker` is still sequential. There is no pool yet.

### Batching

- What it means:
  Grouping several shards into one bounded unit of work.
- Why Argus needs it:
  It reduces per-shard startup overhead while avoiding month-sized reruns.
- Where it appears:
  `groupEntriesByMonth` in `cmd/ingest-worker/main.go`
- Argus example:
  January comments were split into 17 grouped batch files.

### Backpressure

- What it means:
  A system preventing producers from creating work faster than consumers can safely handle it.
- Why Argus needs it:
  When concurrency is added, Argus will need to avoid overwhelming RAM, SSD, or remote scans.
- Where it appears:
  Planned only. Current code uses bounded batch size and source-byte caps, which is a simple early form of pressure control.
- Argus example:
  `--max-batch-source-bytes` keeps one grouped scan from becoming too large.

### Retries

- What it means:
  Automatically rerunning failed units of work.
- Why Argus needs it:
  Remote reads can fail, and long jobs should recover gracefully.
- Where it appears:
  Not implemented as automatic logic yet.
- Argus example:
  Today the project relies on manual reruns plus skip-existing semantics.

### Spill-to-disk

- What it means:
  Letting DuckDB write temporary intermediate work to disk when RAM is not enough.
- Why Argus needs it:
  Local laptops have limited memory, especially for remote scans over big months.
- Where it appears:
  DuckDB temp directory settings in Python scripts.
- Argus example:
  Scripts set `temp_directory` to `.duckdb/tmp`.

### Memory limit

- What it means:
  A cap on how much memory DuckDB is allowed to use.
- Why Argus needs it:
  It reduces the chance that one scan consumes the whole machine.
- Where it appears:
  `--duckdb-memory-limit` flags in preflight, ingest, and clean jobs.
- Argus example:
  Default is `4GB`.

### ClickHouse

- What it means:
  A warehouse-style analytical database.
- Why Argus might need it later:
  If local Parquet plus DuckDB becomes too slow or too awkward at larger scale.
- Where it appears:
  `SPEC.md` and `docs/decisions/ADR-001-local-first-duckdb.md`
- Argus example:
  It is intentionally not used in current code.

## 6. Decisions made and why

### Decision: Start local-first with DuckDB and Parquet

- Why:
  Lower cost, faster iteration, and less operational overhead.
- Alternatives:
  Start with ClickHouse or another warehouse.
- Tradeoffs:
  Simpler now, weaker concurrency and shared-serving story later.
- Final or changeable:
  Changeable later. Documented in `ADR-001`.

### Decision: Prove a pilot slice before broad ingestion

- Why:
  Research usefulness matters more than full-history ingest.
- Alternatives:
  Ingest huge date ranges first.
- Tradeoffs:
  Less coverage now, much less wasted effort.
- Final or changeable:
  This is a strategic principle, likely stable.

### Decision: Use Q1 2021 travel as the first pilot

- Why:
  Known-good archive access, bounded scale, and relevant research signal potential.
- Alternatives:
  Newer years, larger domains, or all of Reddit.
- Tradeoffs:
  Less recent data, but lower archive uncertainty.
- Final or changeable:
  Changeable, but currently the only encoded pilot.

### Decision: Build exact manifests before ingest

- Why:
  Reproducibility and safe retry behavior.
- Alternatives:
  Use broad month wildcards without an explicit plan file.
- Tradeoffs:
  Slightly more upfront metadata work, much better control.
- Final or changeable:
  Likely stable.

### Decision: Filter on ingest into a raw pilot slice

- Why:
  Full-Reddit months are huge; the pilot only needs selected subreddits.
- Alternatives:
  Download whole months first, filter later.
- Tradeoffs:
  Saves disk, but still requires expensive remote scans to find matching rows.
- Final or changeable:
  Probably stable for pilot-scale work.

### Decision: Keep raw and clean layers separate

- Why:
  Rebuildability and trust.
- Alternatives:
  Clean in place or skip a source-faithful landing layer.
- Tradeoffs:
  More files, better traceability.
- Final or changeable:
  Very likely stable.

### Decision: Use grouped bounded ingest batches instead of one wildcard month scan

- Why:
  Safer memory profile and better resume behavior.
- Alternatives:
  One giant monthly scan or one process per shard.
- Tradeoffs:
  More checkpoint files, better operational safety.
- Final or changeable:
  Changeable, but clearly better than the earlier extremes.

### Decision: Use simple deterministic cleaning rules first

- Why:
  The project needs transparent, explainable transformations before ML or LLM-based enrichment.
- Alternatives:
  Jump straight to classifiers or summarizers.
- Tradeoffs:
  Less sophistication now, higher trust and easier debugging.
- Final or changeable:
  Changeable later.

### Decision: Do not hash authors yet

- Why:
  Research requirements are not settled, and preserving author fields keeps options open.
- Alternatives:
  Hash or drop authors immediately.
- Tradeoffs:
  Better flexibility now, but privacy policy decisions still need to be made.
- Final or changeable:
  Explicitly revisitable.

## 7. Blockers and problems faced

Chronological list:

### 1. Needed to confirm the archive was usable before writing ingestion code

- What happened:
  The project started as a plan and needed real validation of the `open-index/arctic` archive.
- Symptom:
  No guarantee the paths, schemas, or remote reads would work as assumed.
- Root cause:
  Dataset-card descriptions are not enough for implementation.
- Fix:
  Added discovery SQL, a discovery report, and an archive summary manifest.
- Resolved:
  Yes, for the validated pilot scope.
- Lesson:
  Always validate external data physically, not just conceptually.

### 2. A direct comment probe under `data/comments/2023/` returned `404`

- What happened:
  A newer-year comment path did not behave as expected.
- Symptom:
  Remote access for that path failed.
- Root cause:
  The archive layout or coverage could not be assumed uniformly for newer comment years.
- Fix:
  Chose a known-good pilot window in Q1 2021.
- Resolved:
  Worked around, not globally resolved.
- Lesson:
  Treat archive freshness and completeness as something to validate, not assume.

### 3. Initial manifest IDs were not stable enough

- What happened:
  Manifest ID generation needed normalization.
- Symptom:
  Historical artifacts show older manifest ID formats, for example `pilot_travel_q1_2021-20260611T201506Z` inside an earlier preflight artifact.
- Root cause:
  The first manifest ID approach was not deterministic enough for clean reproducibility.
- Fix:
  `internal/manifest/manifest.go` now derives deterministic IDs from pipeline inputs and entries.
- Resolved:
  Yes, in current code.
- Lesson:
  Run identity and manifest identity are different problems and should be modeled separately.

### 4. One-process-per-shard and month-wide wildcard scans were both awkward

- What happened:
  The project needed a better unit of work for ingest.
- Symptom:
  Shard mode had startup overhead; month-wide wildcard scans were too risky and large.
- Root cause:
  The archive is large enough that both extremes are operationally clumsy.
- Fix:
  Added grouped month batches using exact manifest URLs plus source-byte caps.
- Resolved:
  Mostly resolved for pilot work.
- Lesson:
  Good data engineering is often about choosing the right work-unit size.

### 5. Zero-byte outputs could poison retries

- What happened:
  Failed or interrupted jobs could leave empty files behind.
- Symptom:
  A later rerun might treat an empty output as already complete.
- Root cause:
  File existence alone is not enough to prove success.
- Fix:
  Added atomic temp-file writing and explicit zero-byte cleanup. Added tests for this behavior.
- Resolved:
  Largely yes.
- Lesson:
  Resume logic must validate output quality, not just output presence.

### 6. Run ID collisions occurred when jobs started in the same second

- What happened:
  The ingest worker uses second-granularity timestamps for `run_id`.
- Symptom:
  `state/checkpoints/phase2/phase2-ingest-20260612T095001Z/` contains both comments and submissions work, which indicates two runs shared the same ID.
- Root cause:
  `run_id` precision is too low.
- Fix:
  No code fix yet.
- Resolved:
  No. This is still unresolved.
- Lesson:
  Run IDs must be unique even during fast local orchestration.

### 7. Grouped-entry checkpoints do not carry per-entry row counts

- What happened:
  Grouped batch checkpoints exist, but the per-entry child checkpoints only record status and output path.
- Symptom:
  February submission entry checkpoints show `rows_written: 0` even when the grouped batch wrote rows.
- Root cause:
  The helper that writes grouped-entry checkpoints does not distribute or record per-entry metrics.
- Fix:
  None yet.
- Resolved:
  No. This is an observability gap.
- Lesson:
  A checkpoint format should capture the level of detail you will want during debugging later.

### 8. Clean-layer reruns happened while raw ingestion was still changing

- What happened:
  January clean outputs were generated and regenerated before raw January comments fully stabilized.
- Symptom:
  Current raw comments have 14,795 rows, but clean comments only have 9,509.
- Root cause:
  Phase 3 was run against an in-progress Phase 2 state.
- Fix:
  Partial local reruns improved the clean layer, but it still lags the latest raw comments.
- Resolved:
  Not fully. Clean comments should be rebuilt after raw backfill settles.
- Lesson:
  Downstream layers should usually not be treated as final while upstream ingestion is still moving.

### 9. A February submissions backfill was interrupted mid-run

- What happened:
  February submissions part 1 and part 2 completed, but part 3 stopped mid-flight.
- Symptom:
  There is a zero-byte `.tmp` file and no final Phase 2 run record for `phase2-ingest-20260612T101700Z`.
- Root cause:
  The process stopped before final cleanup and summary write.
- Fix:
  Partial outputs and checkpoints survive, but the run itself did not close cleanly.
- Resolved:
  Partially. The temp file should be cleaned and the run resumed or rerun.
- Lesson:
  Resumability is working at the file level, but run finalization still needs hardening.

## 8. Current data assumptions

### Confirmed

- The project is targeting the public Hugging Face dataset `open-index/arctic`.
- The archive is readable through DuckDB using `read_parquet(...)`.
- Comments and submissions are both available in Parquet.
- The validated path shape is monthly and shard-based:
  - `data/comments/<year>/<month>/<shard>.parquet`
  - `data/submissions/<year>/<month>/<shard>.parquet`
- The current pilot manifests confirm:
  - January 2021: 196 shard entries total
  - Q1 2021 full pilot: 1,122 shard entries total
- The pilot filter currently targets six subreddits:
  `travel`, `solotravel`, `shoestring`, `onebag`, `digitalnomad`, `travelhacks`

### Assumed but not yet validated

- Q1 2021 is representative enough for the first travel research workflows.
- Filtering by subreddit is a good enough first cut for defining the research slice.
- Keeping author names unmodified is acceptable for local research at this stage.
- Remote access performance will remain tolerable for bounded month-by-month ingest.

### Unknown

- Exact archive completeness for all newer years, especially comments.
- Exact licensing or acceptable-use edge cases beyond the current working assumption that public archive use is allowed.
- Whether later pilot domains should reuse the same schema and cleaning rules unchanged.
- Whether future enrichment needs author retention, hashing, or removal.
- Whether full-quarter ingest for both record types will stay operationally comfortable on the current machine without further tuning.

## 9. Current schema/data model

### Raw comments dataset

- Purpose:
  Source-faithful local landing zone for selected travel comments.
- Important columns implemented:
  `id`, `author`, `subreddit`, `body`, `score`, `created_utc`, `created_at`, `body_length`, `link_id`, `parent_id`, `distinguished`, `author_flair_text`, `source_file`, `ingested_at`, `manifest_id`, `month`, `year`
- Why they matter:
  They preserve Reddit content plus provenance and time context.
- How it will be queried later:
  As the clean-layer source and for evidence tracebacks.

### Raw submissions dataset

- Purpose:
  Source-faithful local landing zone for selected travel submissions.
- Important columns implemented:
  `id`, `author`, `subreddit`, `title`, `selftext`, `score`, `created_utc`, `created_at`, `title_length`, `num_comments`, `url`, `over_18`, `link_flair_text`, `author_flair_text`, `source_file`, `ingested_at`, `manifest_id`, `month`, `year`
- Why they matter:
  They preserve the main research text plus provenance.
- How it will be queried later:
  As clean-layer input and evidence context.

### Clean comments dataset

- Purpose:
  Normalized analytical form of raw comments.
- Important columns implemented:
  Raw columns plus `raw_duplicate_count`, `body_clean`, `is_deleted`, `is_removed`, `is_bot_like`, `text_length`, `language`, `raw_id`, `cleaned_at`, `clean_run_id`
- Why they matter:
  They make comments easier to filter, compare, and trace.
- How it will be queried later:
  Pain-point extraction, quality filtering, and evidence exports.

### Clean submissions dataset

- Purpose:
  Normalized analytical form of raw submissions.
- Important columns implemented:
  Raw columns plus `raw_duplicate_count`, `title_clean`, `selftext_clean`, `combined_text`, `is_deleted`, `is_removed`, `is_bot_like`, `text_length`, `language`, `raw_id`, `cleaned_at`, `clean_run_id`
- Why they matter:
  They expose one combined text field for future rule-based signal extraction while preserving original fields.
- How it will be queried later:
  Feature-request detection, pain-point phrasing, and evidence review.

### Research signal fields

- Current state:
  Not implemented.
- What exists:
  Only the rule config `configs/signals/deterministic-v1.yaml`
- Planned concepts implied by config:
  signal types like `pain_point`, `feature_request`, `recommendation_request`, `workaround`, and `comparison`
- What is still missing:
  A formal signal schema, actual output tables, and the code that writes them.

### Metrics / mart fields

- Current state:
  Not implemented.
- What exists:
  `data/marts/` and `sql/marts/` placeholders only.
- Likely future contents:
  counts by month, subreddit, signal type, entity, and evidence-backed research slices
- Important honesty note:
  No mart schema is encoded yet, so this remains planned work.

## 10. How to run the current project

From a fresh local setup, from the repo root:

Required tools:

- Go 1.25+
- Python 3.9+
- git
- network access for Hugging Face reads

Environment variables:

- None are required for the normal path.
- Optional workaround if DuckDB extension caching has permission trouble:
  temporarily point `HOME` at a writable directory before remote DuckDB work.

Setup:

```bash
./scripts/dev/bootstrap_duckdb.sh
go version
python3 -c "import duckdb; print(duckdb.__version__)"
```

Test commands:

```bash
go test ./...
```

Current observed result:

- `go test ./...` passes.

Build commands:

- There is no `Makefile`.
- The repo currently uses `go run` directly rather than a separate build step.

Manifest generation:

```bash
go run ./cmd/manifest-builder \
  --pipeline configs/pipelines/pilot-travel-q1-2021.yaml \
  --month 2021-01 \
  --output manifests/pilot/travel-q1-2021-smoke-manifest.json
```

Expected stdout shape:

```text
manifest written: manifests/pilot/travel-q1-2021-smoke-manifest.json
entries: 196
bytes_total: 8760988971
```

Preflight counting:

```bash
go run ./cmd/phase2-preflight \
  --pipeline configs/pipelines/pilot-travel-q1-2021.yaml \
  --manifest manifests/pilot/travel-q1-2021-smoke-manifest.json \
  --record-type submissions \
  --month 2021-01 \
  --limit-shards 4 \
  --output state/runs/phase2/preflight-2021-01-submissions-4.json
```

Expected output shape:

- JSON report to stdout
- JSON file under `state/runs/phase2/`

Observed example:

- 4 January submission shards
- 203,311,681 source bytes
- 491 matching rows

Raw ingest:

```bash
go run ./cmd/ingest-worker \
  --pipeline configs/pipelines/pilot-travel-q1-2021.yaml \
  --manifest manifests/pilot/travel-q1-2021-full-manifest.json \
  --record-type comments \
  --month 2021-01 \
  --group-by-month \
  --batch-size 8 \
  --max-batch-source-bytes 536870912
```

Expected stdout shape:

```text
run complete: phase2-ingest-...
work_units_processed: <n>
rows_written: <n>
errors: 0
```

Raw validation:

```bash
python3 scripts/dev/run_duckdb_sql_file.py \
  --sql-file sql/checks/phase-2-raw-validation.sql
```

Clean transform:

```bash
go run ./cmd/clean-transform \
  --pipeline configs/pipelines/pilot-travel-q1-2021.yaml \
  --record-type comments \
  --month 2021-01
```

Expected stdout shape:

```text
run complete: phase3-clean-...
work_units_processed: 1
rows_written: <n>
errors: 0
```

Clean validation:

```bash
python3 scripts/dev/run_duckdb_sql_file.py \
  --sql-file sql/checks/phase-3-clean-validation.sql
```

Where outputs are written:

- Manifests: `manifests/pilot/`
- Raw Parquet: `data/raw/`
- Clean Parquet: `data/clean/`
- Checkpoints: `state/checkpoints/`
- Run records: `state/runs/`
- Logs: `state/logs/`

## 11. How to verify it works

Smoke tests:

1. Generate a January smoke manifest.
2. Preflight a tiny set of shards.
3. Ingest a bounded month and record type.
4. Run raw validation SQL.
5. Run a clean transform.
6. Run clean validation SQL.

Useful manual checks:

```bash
python3 scripts/dev/run_duckdb_sql_file.py --sql-file sql/checks/phase-2-raw-validation.sql
python3 scripts/dev/run_duckdb_sql_file.py --sql-file sql/checks/phase-3-clean-validation.sql
```

Current expected output shape from raw validation:

- row-count tuples like `('comments', 14795)`
- provenance-null tuples like `('comments', 0, 0, 0)`
- distinct subreddit tuples like `('comments', 6)`
- schema listings

Current expected output shape from clean validation:

- row-count tuples like `('comments', 9509)`
- traceability-null tuples like `('comments', 0, 0, 0, 0)`
- duplicate-group counts
- deleted/removed/bot-like ratios
- schema listings

How to inspect generated Parquet:

```bash
python3 - <<'PY'
import duckdb
con = duckdb.connect()
print(con.execute("SELECT id, subreddit, source_file FROM read_parquet('data/raw/comments/year=*/month=*/*.parquet') LIMIT 3").fetchall())
print(con.execute("SELECT id, subreddit, text_length, is_deleted FROM read_parquet('data/clean/comments/year=*/month=*/*.parquet') LIMIT 3").fetchall())
PY
```

How to inspect manifests:

- Open `manifests/pilot/travel-q1-2021-smoke-manifest.json`
- Check `summary.entry_count`, `summary.bytes_total`, and a few `entries[]`

How to inspect checkpoints:

- Open `state/checkpoints/phase2/<run_id>/`
- Verify `status`, `output_path`, and `rows_written`

How to know ingestion succeeded:

- Non-empty Parquet files exist under `data/raw/...`
- raw validation shows non-zero rows
- provenance fields are present

How to know cleaning succeeded:

- Non-empty Parquet files exist under `data/clean/...`
- clean validation shows zero missing `raw_id`, `source_file`, `clean_run_id`, and `cleaned_at`

How to know research signals succeeded:

- You cannot yet, because no signal job exists.

## 12. What is not done yet

Not started:

- signal-enrichment CLI
- research marts
- evidence export workflow
- notebook/API access layer
- ClickHouse path
- worker pool / concurrency controls

Partially implemented:

- raw ingest hardening
- clean layer
- resumability and operational logging
- broader month backfill beyond January

Implemented but untested or lightly tested:

- manifest builder has only basic unit coverage
- preflight has no unit tests
- clean transform has no tests
- DuckDB scripts are validated by use, not by automated test suite

Implemented but needs refactor:

- second-granularity run IDs
- grouped checkpoint metrics
- cleaner orchestration between Phase 2 and Phase 3

Blocked by unknown dataset details:

- safe assumptions about newer-year comments availability
- longer-range archive completeness

Deferred intentionally:

- ClickHouse
- UI/product layer
- real-time ingestion
- advanced ML/LLM enrichment

## 13. Next recommended steps

### 1. Rebuild January clean outputs after raw January is frozen

- Why it matters:
  The current clean comments dataset lags the current raw comments dataset.
- Complexity:
  Easy
- Learn first:
  raw vs clean layers, idempotency
- Files likely involved:
  `cmd/clean-transform/main.go`, `scripts/dev/duckdb_clean_transform.py`, `sql/checks/phase-3-clean-validation.sql`
- Definition of done:
  January clean comments and submissions are regenerated from the final January raw layer and validation still passes.

### 2. Fix run ID collisions

- Why it matters:
  Shared run IDs make run records and checkpoint directories ambiguous.
- Complexity:
  Easy
- Learn first:
  run metadata, checkpoints
- Files likely involved:
  `cmd/ingest-worker/main.go`, `cmd/phase2-preflight/main.go`, `cmd/clean-transform/main.go`
- Definition of done:
  Two jobs launched in the same second get different run IDs.

### 3. Finish and validate February submissions ingest, then decide whether to continue Q1

- Why it matters:
  Right now February is only partially ingested.
- Complexity:
  Medium
- Learn first:
  manifests, batching, resumability
- Files likely involved:
  `cmd/ingest-worker/main.go`, `state/checkpoints/phase2/**`, `state/logs/**`
- Definition of done:
  February submissions either complete cleanly or are explicitly rolled back and re-run with no stray `.tmp` file.

### 4. Add a stable Phase 2 backfill driver script or wrapper

- Why it matters:
  The repo has the pieces, but not yet a polished orchestration path for full pilot execution.
- Complexity:
  Medium
- Learn first:
  batching, run metadata, shell safety
- Files likely involved:
  likely a new script under `scripts/dev/` plus runbook updates
- Definition of done:
  One documented command can run manifest build, preflight, ingest, and validation in a predictable order.

### 5. Add tests for clean-transform SQL behavior

- Why it matters:
  The clean layer now changes row counts and adds logic, but it has no automated correctness tests.
- Complexity:
  Medium
- Learn first:
  deduping, test fixtures, raw-to-clean lineage
- Files likely involved:
  `scripts/dev/duckdb_clean_transform.py`, `testdata/fixtures/`, maybe a new Go or Python test harness
- Definition of done:
  Deterministic fixtures prove deleted/removed flags, bot heuristics, and dedupe behavior.

### 6. Implement Phase 4 deterministic signal generation

- Why it matters:
  This is the first step from data plumbing to actual research value.
- Complexity:
  Hard
- Learn first:
  clean layer schema, rule-based labeling, evidence traceability
- Files likely involved:
  new `cmd/enrich-signals/main.go` or Python helper, `configs/signals/deterministic-v1.yaml`, `data/marts/`, `sql/`
- Definition of done:
  Clean data can be transformed into at least one research-facing signal table with source-row lineage.

### 7. Add first research queries and evidence export

- Why it matters:
  The project is only valuable if it answers real research questions.
- Complexity:
  Medium
- Learn first:
  marts, SQL aggregation, evidence traceability
- Files likely involved:
  `sql/marts/`, `data/exports/`, new runbooks
- Definition of done:
  One checked-in workflow can output repeated pain points plus example evidence rows.

### 8. Reassess whether Q1 should be widened after workflow value is proven

- Why it matters:
  Scale should follow research usefulness, not lead it.
- Complexity:
  Medium
- Learn first:
  cost measurement, partitioning, pipeline operations
- Files likely involved:
  pipeline config, manifests, docs/research notes
- Definition of done:
  There is an explicit go/no-go decision for expanding scope.

## 14. Questions you need me to answer

- Do you want the first real research workflow to be pain-point discovery, app-idea discovery, or evidence export?
- Should travel remain the v1 domain, or was it only a safe pilot?
- Do you want to keep author names in local research outputs, or should we hash them soon?
- Should v1 be CLI-first only, or should we start shaping notebook-friendly outputs now?
- What is the acceptable local disk budget for the next phase: stay under 20 GB strictly, or loosen it if the pilot proves useful?
- Do you want Q1 2021 to remain the pilot window, or should we switch to a different quarter after Phase 4 exists?
- Should the first signal layer optimize for precision or recall?

## 15. Explain the last 2 days like a timeline

### Day 1: 2026-06-11

- Started with:
  project specification, execution guide, and repo scaffolding
- Implemented:
  repo layout, ADR, pilot definition, archive validation docs, discovery SQL, archive summary, initial manifest builder, initial ingest worker, initial manifests
- Faced:
  needed to validate archive assumptions and choose a safe pilot window
- Fixed:
  validated remote access with DuckDB and chose Q1 2021 travel
- End state:
  Argus had real Phase 1 outputs and a first Phase 2 ingest path

### Day 2: 2026-06-12

- Started with:
  working Phase 2 basics that still needed hardening
- Implemented:
  deterministic manifest IDs, grouped bounded ingest, preflight counting, zero-byte output handling, Phase 2 validation SQL, optimization docs, Phase 3 clean transform, Phase 3 validation SQL
- Faced:
  retry safety issues, noisy local reruns, run ID collisions, in-progress backfill state, and upstream/downstream timing mismatch between raw and clean layers
- Fixed:
  improved resumability, added temp-file writes, added skip-existing tests, produced real January raw and clean outputs
- End state:
  January raw and early clean layers exist locally, February submissions are partially ingested, and research enrichment is still the next major frontier

## 16. Teach me the implementation path

If you want to understand this codebase, learn it in this order.

### Step 1

- What to read:
  `README.md`, `SPEC.md`, `IMPLEMENTATION_GUIDE.md`
- What concept it teaches:
  What Argus is trying to become, and why the build is staged.
- Small experiment:
  Skim the phase list and say which phase we are actually in today.
- Question you should be able to answer:
  Why are we not starting with ClickHouse or full-history ingest?

### Step 2

- What to read:
  `docs/research/pilot-definition.md`, `configs/pipelines/pilot-travel-q1-2021.yaml`, `configs/domains/travel.yaml`
- What concept it teaches:
  How a product idea becomes a bounded pilot.
- Small experiment:
  Change nothing, but trace where the six subreddits are defined.
- Question you should be able to answer:
  What exactly is the current pilot slice?

### Step 3

- What to read:
  `docs/research/phase-1-discovery-report.md`, `sql/discovery/*.sql`
- What concept it teaches:
  Remote Parquet exploration with DuckDB.
- Small experiment:
  Run the archive validation SQL and inspect sample rows.
- Question you should be able to answer:
  How do we know the archive is usable without downloading all of it?

### Step 4

- What to read:
  `cmd/manifest-builder/main.go`, `internal/archive/huggingface.go`, `internal/manifest/manifest.go`
- What concept it teaches:
  How large archive assumptions become exact shard manifests.
- Small experiment:
  Generate a smoke manifest and inspect the first few entries.
- Question you should be able to answer:
  What is a manifest, and why is it central to resumability?

### Step 5

- What to read:
  `cmd/phase2-preflight/main.go`, `scripts/dev/duckdb_count.py`
- What concept it teaches:
  Counting before copying.
- Small experiment:
  Preflight four January submission shards.
- Question you should be able to answer:
  How can we estimate filtered output before writing local files?

### Step 6

- What to read:
  `cmd/ingest-worker/main.go`, `scripts/dev/duckdb_filter_copy.py`, `docs/runbooks/phase-2-validation.md`
- What concept it teaches:
  Filter-on-ingest, batching, checkpoints, and provenance.
- Small experiment:
  Read one Phase 2 checkpoint JSON next to its output Parquet file.
- Question you should be able to answer:
  How does Argus turn remote Reddit shards into local raw Parquet safely?

### Step 7

- What to read:
  `docs/runbooks/phase-3-cleaning-rules.md`, `cmd/clean-transform/main.go`, `scripts/dev/duckdb_clean_transform.py`
- What concept it teaches:
  The difference between raw data and clean analytical data.
- Small experiment:
  Compare the raw and clean schemas for one record type.
- Question you should be able to answer:
  What transformations happen before research signals are computed?

### Step 8

- What to read:
  `sql/checks/phase-2-raw-validation.sql`, `sql/checks/phase-3-clean-validation.sql`
- What concept it teaches:
  How to verify pipeline progress using data, not hope.
- Small experiment:
  Run both validation files and compare row counts.
- Question you should be able to answer:
  How do you know whether raw and clean outputs are trustworthy enough to build on?

### Step 9

- What to read:
  `configs/signals/deterministic-v1.yaml`
- What concept it teaches:
  Where the project is trying to go next.
- Small experiment:
  Read the signal types and imagine what a signal table would need to store.
- Question you should be able to answer:
  What is missing between today’s clean data and actual research workflows?

Final honest summary:

- Argus is no longer an empty scaffold.
- It already has a real discovery path, manifest layer, ingest path, raw layer, and early clean layer.
- It is not yet a finished research platform.
- The next major milestone is not “more ingestion.” It is proving that cleaned data can become useful research signals and evidence-backed outputs.
