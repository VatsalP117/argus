# Phase 3 Clean Transform Runbook

This runbook covers the first reproducible clean-layer transform.

## Run A Single Month

```bash
go run ./cmd/clean-transform \
  --pipeline configs/pipelines/pilot-travel-q1-2021.yaml \
  --record-type submissions \
  --month 2021-01
```

Repeat for comments:

```bash
go run ./cmd/clean-transform \
  --pipeline configs/pipelines/pilot-travel-q1-2021.yaml \
  --record-type comments \
  --month 2021-01
```

## Expected Outputs

- clean Parquet under `data/clean/<record_type>/year=YYYY/month=MM/`
- checkpoints under `state/checkpoints/phase3/`
- run records under `state/runs/phase3/`

## Validation

Run:

```bash
python3 scripts/dev/run_duckdb_sql_file.py \
  --sql-file sql/checks/phase-3-clean-validation.sql
```

Manual review should inspect:

- deleted and removed ratios
- bot-like samples
- empty text rows that are not deleted or removed
- duplicate ids if any appear
