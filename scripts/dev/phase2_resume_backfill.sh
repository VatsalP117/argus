#!/usr/bin/env zsh
set -euo pipefail

pipeline_path="${1:-configs/pipelines/pilot-travel-q1-2021.yaml}"
manifest_path="${2:-manifests/pilot/travel-q1-2021-full-manifest.json}"
bin_dir=".bin"
manifest_builder_bin="$bin_dir/manifest-builder"
ingest_worker_bin="$bin_dir/ingest-worker"

log_dir="state/logs"
mkdir -p "$log_dir"
mkdir -p "$bin_dir"

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
log_path="$log_dir/phase2-backfill-$timestamp.log"
pid_path="$log_dir/phase2-backfill.pid"

if [[ -f "$pid_path" ]]; then
  existing_pid="$(cat "$pid_path")"
  if [[ -n "$existing_pid" ]] && kill -0 "$existing_pid" 2>/dev/null; then
    echo "phase2 backfill already running with pid $existing_pid" >&2
    exit 1
  fi
  rm -f "$pid_path"
fi

cleanup() {
  rm -f "$pid_path"
}
trap cleanup EXIT INT TERM

echo "$$" > "$pid_path"

run_step() {
  local record_type="$1"
  local month="$2"

  printf '[%s] start %s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$record_type" "$month" | tee -a "$log_path"

  "$ingest_worker_bin" \
    --pipeline "$pipeline_path" \
    --manifest "$manifest_path" \
    --record-type "$record_type" \
    --month "$month" \
    --group-by-month \
    --batch-size 8 \
    --max-batch-source-bytes 536870912 \
    2>&1 | tee -a "$log_path"

  printf '[%s] done %s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$record_type" "$month" | tee -a "$log_path"
}

go build -o "$manifest_builder_bin" ./cmd/manifest-builder
go build -o "$ingest_worker_bin" ./cmd/ingest-worker

"$manifest_builder_bin" \
  --pipeline "$pipeline_path" \
  --output "$manifest_path" \
  2>&1 | tee -a "$log_path"

run_step submissions 2021-01
run_step comments 2021-01
run_step submissions 2021-02
run_step comments 2021-02
run_step submissions 2021-03
run_step comments 2021-03
