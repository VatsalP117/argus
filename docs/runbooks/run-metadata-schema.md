# Run Metadata Schema

Every non-trivial Argus job should emit a machine-readable run record under `state/runs/`.

## Required Fields

- `run_id`
- `phase`
- `job_name`
- `started_at`
- `finished_at`
- `status`
- `git_sha`
- `config_hash`
- `records_seen`
- `records_written`
- `error_count`
- `warnings`
- `input_refs`
- `output_refs`
- `notes`

## Suggested JSON Shape

```json
{
  "run_id": "phase2-smoke-20210611-001",
  "phase": "phase2",
  "job_name": "raw_ingest_smoke",
  "started_at": "2026-06-11T18:00:00Z",
  "finished_at": "2026-06-11T18:04:13Z",
  "status": "completed",
  "git_sha": "abc123",
  "config_hash": "sha256:...",
  "records_seen": 120394,
  "records_written": 120394,
  "error_count": 0,
  "warnings": [],
  "input_refs": [
    "manifests/pilot/travel-q1-2021-plan.json"
  ],
  "output_refs": [
    "data/raw/comments/..."
  ],
  "notes": "Smoke ingest on 2021-01 comments shard subset"
}
```

## Rules

- `status` should be one of `completed`, `failed`, `partial`, `cancelled`
- all timestamps should be ISO 8601 UTC
- `records_seen` and `records_written` must be integers
- `warnings` should be an array even if empty
