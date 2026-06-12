#!/usr/bin/env bash
set -euo pipefail

PIPELINE_PATH="${1:-configs/pipelines/v0-travel-jan-feb-2021.yaml}"
RUN_TS="$(date -u +"%Y%m%dT%H%M%SZ")"
OUT_DIR="data/exports/poc-run-${RUN_TS}"
QUERY_DIR="${OUT_DIR}/queries"
LOG_DIR="${OUT_DIR}/logs"

mkdir -p "${QUERY_DIR}" "${LOG_DIR}"

echo "==> rebuilding phase 4 marts"
go run ./cmd/enrich-signals \
  --pipeline "${PIPELINE_PATH}" \
  --force | tee "${LOG_DIR}/phase4-enrich.log"

echo "==> validating phase 4 marts"
python3 scripts/dev/run_duckdb_sql_file.py \
  --sql-file sql/checks/phase-4-signal-validation.sql | tee "${QUERY_DIR}/phase-4-validation.txt"

echo "==> running pain point query"
python3 scripts/dev/run_duckdb_sql_file.py \
  --sql-file sql/marts/v0-pain-point-discovery.sql | tee "${QUERY_DIR}/pain-point-discovery.txt"

echo "==> running app idea query"
python3 scripts/dev/run_duckdb_sql_file.py \
  --sql-file sql/marts/v0-app-idea-discovery.sql | tee "${QUERY_DIR}/app-idea-discovery.txt"

echo "==> running entity/workaround query"
python3 scripts/dev/run_duckdb_sql_file.py \
  --sql-file sql/marts/v0-entity-workaround-discovery.sql | tee "${QUERY_DIR}/entity-workaround-discovery.txt"

echo "==> exporting pain point evidence"
go run ./cmd/export-evidence \
  --pipeline "${PIPELINE_PATH}" \
  --signal-type pain_point | tee "${LOG_DIR}/pain-point-export.log"

echo "==> exporting app idea evidence"
go run ./cmd/export-evidence \
  --pipeline "${PIPELINE_PATH}" \
  --signal-type recommendation_request | tee "${LOG_DIR}/recommendation-export.log"

echo "==> poc bundle ready at ${OUT_DIR}"
