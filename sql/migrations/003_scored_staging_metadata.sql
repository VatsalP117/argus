ALTER TABLE staged_candidate_batches
    ADD COLUMN IF NOT EXISTS score_path VARCHAR;

ALTER TABLE staged_candidate_batches
    ADD COLUMN IF NOT EXISTS score_checksum VARCHAR;

ALTER TABLE staged_candidate_batches
    ADD COLUMN IF NOT EXISTS score_bytes BIGINT;

ALTER TABLE staged_candidate_batches
    ADD COLUMN IF NOT EXISTS relevance_version VARCHAR;

ALTER TABLE staged_candidate_batches
    ADD COLUMN IF NOT EXISTS relevance_config_hash VARCHAR;
