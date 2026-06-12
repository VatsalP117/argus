# Phase 5 Evidence Export Runbook

This runbook covers the first V0 evidence-export workflow on the frozen Jan-Feb 2021 travel slice.

## What Exists Now

Phase 5 currently includes:

- a Go `export-evidence` CLI
- a DuckDB-backed CSV export helper
- grouped summary exports for one signal type at a time
- evidence-row exports with lineage fields for manual review

## Recommended V0 Scope

Use the frozen local slice:

- domain: `travel`
- months:
  - `2021-01`
  - `2021-02`
- record types:
  - comments
  - submissions

## Run An Export

Export the top pain-point groups plus example evidence rows:

```bash
go run ./cmd/export-evidence \
  --pipeline configs/pipelines/v0-travel-jan-feb-2021.yaml \
  --signal-type pain_point
```

Example with a topic filter:

```bash
go run ./cmd/export-evidence \
  --pipeline configs/pipelines/v0-travel-jan-feb-2021.yaml \
  --signal-type comparison \
  --topic-hint airbnb \
  --max-groups 10 \
  --examples-per-group 3
```

## Expected Outputs

Each run writes a timestamped bundle under:

- `data/exports/phase5-export-*/summary.csv`
- `data/exports/phase5-export-*/evidence.csv`
- `state/runs/phase5/*.json`

## Output Shape

`summary.csv` includes:

- `topic_hint`
- `matched_pattern`
- `signal_count`
- `subreddit_count`
- `first_seen_at`
- `last_seen_at`

`evidence.csv` includes:

- `topic_hint`
- `matched_pattern`
- `example_rank`
- `subreddit`
- `created_at`
- `source_type`
- `raw_id`
- `source_id`
- `evidence_text`
- `source_file`
- `manifest_id`
- `clean_run_id`
- `signal_run_id`

## Current Limitations

- exports are CSV-first for easy manual review
- only one signal type is exported per run
- topic filtering is exact-match on `topic_hint`
- this is a V0 analyst workflow, not a polished product surface
