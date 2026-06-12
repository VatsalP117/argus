# Argus

Argus is a local-first Reddit research platform for market research, startup idea discovery, and pain-point analysis.

The core idea is simple:

- ingest bounded slices of historical Reddit data
- normalize and enrich them
- expose repeatable research workflows with evidence-backed outputs

Argus is deliberately not starting as:

- a full-history ingestion job
- a real-time streaming system
- a polished end-user product
- a warehouse-heavy data platform

## Current Status

Current implementation status:

- Phase 0: scaffolded
- Phase 1: archive discovery and pilot definition prepared
- Phase 2: core manifest and raw ingest path implemented, optimization and hardening in progress

Companion docs:

- [SPEC.md](/Users/vatsalpatel/Desktop/Projects/argus/SPEC.md)
- [IMPLEMENTATION_GUIDE.md](/Users/vatsalpatel/Desktop/Projects/argus/IMPLEMENTATION_GUIDE.md)

## Default Operating Model

Argus v1 is local-first.

Default stack:

- DuckDB for remote archive access and local analytics
- Parquet for raw, clean, and mart layers
- Go for ingestion and pipeline tooling
- checked-in SQL plus configs for repeatable research workflows

## Initial Pilot

The first pilot is intentionally narrow:

- domain: travel
- target window: Q1 2021
- smoke month: 2021-01
- record types: submissions and comments
- research focus:
  - repeated travel pain points
  - request-like conversations suggesting app ideas
  - evidence exports for manual review

See the detailed pilot definition in [docs/research/pilot-definition.md](/Users/vatsalpatel/Desktop/Projects/argus/docs/research/pilot-definition.md).

## Local Requirements

Minimum practical local baseline:

- Apple Silicon Mac or similar modern laptop/desktop
- 16 GB RAM
- about 100 GB free SSD space before serious pilot ingest
- Python 3.9+
- Go 1.25+

DuckDB is currently expected via Python package:

```bash
python3 -m pip install --user duckdb
```

## Repository Layout

Important directories:

- `configs/`: domain, pipeline, and signal configuration
- `docs/`: ADRs, runbooks, and research notes
- `manifests/`: archive discovery outputs and pilot plans
- `sql/`: discovery, validation, and future mart queries
- `data/`: local raw, clean, mart, tmp, and export outputs
- `state/`: run logs and checkpoints

## Rules For Contributors And Agents

- Keep the pilot bounded until research outputs prove value.
- Preserve evidence traceability from every derived finding back to source rows.
- Keep raw, clean, and mart layers separate.
- Prefer checked-in SQL and config over notebook-only logic.
- Do not introduce ClickHouse or a VM until measured bottlenecks justify it.
