CREATE TABLE IF NOT EXISTS candidate_scan_runs (
    scan_run_id VARCHAR PRIMARY KEY,
    manifest_id VARCHAR NOT NULL,
    manifest_entry_id VARCHAR NOT NULL,
    source_identity VARCHAR NOT NULL,
    record_type VARCHAR NOT NULL,
    source_path VARCHAR NOT NULL,
    archive_repo VARCHAR NOT NULL,
    archive_revision VARCHAR NOT NULL,
    candidate_version VARCHAR NOT NULL,
    candidate_config_hash VARCHAR NOT NULL,
    status VARCHAR NOT NULL,
    rows_seen BIGINT NOT NULL,
    rows_candidates BIGINT NOT NULL,
    rows_rejected_early BIGINT NOT NULL,
    subreddit_prior_candidates BIGINT NOT NULL,
    bytes_written BIGINT NOT NULL,
    staging_path VARCHAR,
    staging_checksum VARCHAR,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    error VARCHAR,
    UNIQUE (
        manifest_id,
        manifest_entry_id,
        source_identity,
        candidate_version,
        candidate_config_hash
    )
);

CREATE TABLE IF NOT EXISTS candidate_rule_yields (
    scan_run_id VARCHAR NOT NULL,
    rule_group VARCHAR NOT NULL,
    matched_rows BIGINT NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    PRIMARY KEY (scan_run_id, rule_group)
);

CREATE TABLE IF NOT EXISTS staged_candidate_batches (
    staging_batch_id VARCHAR PRIMARY KEY,
    scan_run_id VARCHAR NOT NULL UNIQUE,
    manifest_id VARCHAR NOT NULL,
    manifest_entry_id VARCHAR NOT NULL,
    source_identity VARCHAR NOT NULL,
    candidate_version VARCHAR NOT NULL,
    status VARCHAR NOT NULL,
    staging_path VARCHAR,
    staging_checksum VARCHAR,
    staging_bytes BIGINT NOT NULL,
    candidate_rows BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    validated_at TIMESTAMPTZ,
    commit_started_at TIMESTAMPTZ,
    committed_at TIMESTAMPTZ,
    cleaned_at TIMESTAMPTZ,
    error VARCHAR
);

CREATE TABLE IF NOT EXISTS staging_cleanup_events (
    cleanup_event_id VARCHAR PRIMARY KEY,
    staging_batch_id VARCHAR NOT NULL,
    staging_path VARCHAR NOT NULL,
    staging_checksum VARCHAR,
    bytes_before BIGINT NOT NULL,
    status VARCHAR NOT NULL,
    attempted_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    finished_at TIMESTAMPTZ,
    error VARCHAR
);

ALTER TABLE ingest_batches
    ADD COLUMN IF NOT EXISTS staging_batch_id VARCHAR;

ALTER TABLE ingest_batches
    ADD COLUMN IF NOT EXISTS candidate_version VARCHAR;

ALTER TABLE ingest_batches
    ADD COLUMN IF NOT EXISTS staging_checksum VARCHAR;

ALTER TABLE ingest_batches
    ADD COLUMN IF NOT EXISTS cleanup_status VARCHAR;

ALTER TABLE batch_reconciliation
    ADD COLUMN IF NOT EXISTS scan_run_id VARCHAR;

ALTER TABLE batch_reconciliation
    ADD COLUMN IF NOT EXISTS rows_candidates BIGINT;
