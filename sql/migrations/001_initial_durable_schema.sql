CREATE TABLE IF NOT EXISTS documents (
    document_id VARCHAR PRIMARY KEY,
    source_type VARCHAR NOT NULL,
    source_id VARCHAR NOT NULL,
    raw_id VARCHAR,
    thread_id VARCHAR,
    parent_id VARCHAR,
    subreddit VARCHAR,
    author_hash VARCHAR,
    created_at TIMESTAMPTZ,
    score BIGINT,
    title VARCHAR,
    original_text VARCHAR NOT NULL,
    clean_text VARCHAR NOT NULL,
    text_length BIGINT NOT NULL,
    source_url VARCHAR NOT NULL,
    archive_repo VARCHAR NOT NULL,
    archive_revision VARCHAR NOT NULL,
    source_file VARCHAR NOT NULL,
    source_shard_size BIGINT NOT NULL,
    manifest_id VARCHAR NOT NULL,
    ingest_batch_id VARCHAR NOT NULL,
    clean_version VARCHAR NOT NULL,
    is_deleted BOOLEAN NOT NULL DEFAULT false,
    is_removed BOOLEAN NOT NULL DEFAULT false,
    is_bot_like BOOLEAN NOT NULL DEFAULT false,
    retained_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    UNIQUE (source_type, source_id)
);

CREATE TABLE IF NOT EXISTS document_relevance (
    document_id VARCHAR NOT NULL,
    domain VARCHAR NOT NULL,
    relevance_score DOUBLE NOT NULL,
    relevance_tier VARCHAR NOT NULL,
    matched_terms JSON,
    matched_rules JSON,
    subreddit_prior DOUBLE,
    signal_prior DOUBLE,
    semantic_score DOUBLE,
    classifier_score DOUBLE,
    decision VARCHAR NOT NULL,
    decision_reasons JSON,
    relevance_version VARCHAR NOT NULL,
    scored_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    PRIMARY KEY (document_id, domain, relevance_version)
);

CREATE TABLE IF NOT EXISTS signals (
    signal_id VARCHAR PRIMARY KEY,
    document_id VARCHAR NOT NULL,
    signal_type VARCHAR NOT NULL,
    signal_score DOUBLE NOT NULL,
    matched_pattern VARCHAR,
    evidence_text VARCHAR NOT NULL,
    signal_version VARCHAR NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

CREATE TABLE IF NOT EXISTS entities (
    entity_mention_id VARCHAR PRIMARY KEY,
    document_id VARCHAR NOT NULL,
    entity_type VARCHAR NOT NULL,
    entity_text VARCHAR NOT NULL,
    normalized_entity VARCHAR NOT NULL,
    entity_version VARCHAR NOT NULL
);

CREATE TABLE IF NOT EXISTS document_embeddings (
    document_id VARCHAR NOT NULL,
    embedding_model VARCHAR NOT NULL,
    embedding_dimension INTEGER NOT NULL,
    embedding FLOAT[] NOT NULL,
    embedded_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    PRIMARY KEY (document_id, embedding_model)
);

CREATE TABLE IF NOT EXISTS themes (
    theme_id VARCHAR PRIMARY KEY,
    domain VARCHAR NOT NULL,
    theme_name VARCHAR NOT NULL,
    theme_description VARCHAR NOT NULL,
    theme_version VARCHAR NOT NULL,
    document_count BIGINT NOT NULL,
    community_count BIGINT NOT NULL,
    first_seen_at TIMESTAMPTZ,
    last_seen_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

CREATE TABLE IF NOT EXISTS theme_documents (
    theme_id VARCHAR NOT NULL,
    document_id VARCHAR NOT NULL,
    membership_score DOUBLE NOT NULL,
    evidence_rank INTEGER,
    membership_reason VARCHAR,
    PRIMARY KEY (theme_id, document_id)
);

CREATE TABLE IF NOT EXISTS opportunities (
    opportunity_id VARCHAR PRIMARY KEY,
    domain VARCHAR NOT NULL,
    title VARCHAR NOT NULL,
    problem_statement VARCHAR NOT NULL,
    target_user VARCHAR,
    current_workarounds JSON,
    existing_products JSON,
    opportunity_score DOUBLE NOT NULL,
    evidence_strength DOUBLE NOT NULL,
    status VARCHAR NOT NULL,
    generated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    opportunity_version VARCHAR NOT NULL
);

CREATE TABLE IF NOT EXISTS opportunity_scores (
    opportunity_id VARCHAR PRIMARY KEY,
    recurrence DOUBLE,
    severity DOUBLE,
    community_breadth DOUBLE,
    workaround_activity DOUBLE,
    request_intent DOUBLE,
    willingness_to_pay DOUBLE,
    competitor_dissatisfaction DOUBLE,
    recent_momentum DOUBLE,
    evidence_diversity DOUBLE,
    contradictory_evidence_penalty DOUBLE
);

CREATE TABLE IF NOT EXISTS opportunity_evidence (
    opportunity_id VARCHAR NOT NULL,
    document_id VARCHAR NOT NULL,
    evidence_role VARCHAR NOT NULL,
    evidence_rank INTEGER,
    reason VARCHAR,
    PRIMARY KEY (opportunity_id, document_id, evidence_role)
);

CREATE TABLE IF NOT EXISTS source_manifests (
    manifest_id VARCHAR PRIMARY KEY,
    dataset_repo VARCHAR NOT NULL,
    archive_revision VARCHAR NOT NULL,
    pipeline_name VARCHAR NOT NULL,
    generated_at TIMESTAMPTZ NOT NULL,
    entry_count BIGINT NOT NULL,
    bytes_total BIGINT NOT NULL,
    manifest_path VARCHAR NOT NULL,
    manifest_checksum VARCHAR NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

CREATE TABLE IF NOT EXISTS ingest_batches (
    ingest_batch_id VARCHAR PRIMARY KEY,
    manifest_id VARCHAR NOT NULL,
    status VARCHAR NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    committed_at TIMESTAMPTZ,
    validated_at TIMESTAMPTZ,
    source_rows BIGINT,
    staged_rows BIGINT,
    retained_rows BIGINT,
    rejected_rows BIGINT,
    quarantined_rows BIGINT,
    durable_checksum VARCHAR,
    error VARCHAR
);

CREATE TABLE IF NOT EXISTS batch_reconciliation (
    ingest_batch_id VARCHAR PRIMARY KEY,
    rows_seen BIGINT NOT NULL,
    rows_rejected_early BIGINT NOT NULL,
    rows_staged BIGINT NOT NULL,
    rows_retained BIGINT NOT NULL,
    rows_rejected_late BIGINT NOT NULL,
    rows_quarantined BIGINT NOT NULL,
    source_equation_valid BOOLEAN NOT NULL,
    staging_equation_valid BOOLEAN NOT NULL,
    validated_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

CREATE TABLE IF NOT EXISTS pipeline_versions (
    component VARCHAR NOT NULL,
    version VARCHAR NOT NULL,
    checksum VARCHAR,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp,
    PRIMARY KEY (component, version)
);

CREATE TABLE IF NOT EXISTS saved_queries (
    saved_query_id VARCHAR PRIMARY KEY,
    name VARCHAR NOT NULL,
    query_text VARCHAR NOT NULL,
    parameters JSON,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

CREATE TABLE IF NOT EXISTS saved_research (
    saved_research_id VARCHAR PRIMARY KEY,
    title VARCHAR NOT NULL,
    question VARCHAR NOT NULL,
    answer_markdown VARCHAR NOT NULL,
    evidence_document_ids JSON NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
);

CREATE TABLE IF NOT EXISTS ask_runs (
    ask_run_id VARCHAR PRIMARY KEY,
    question VARCHAR NOT NULL,
    plan_json JSON,
    answer_markdown VARCHAR,
    evidence_document_ids JSON,
    llm_provider VARCHAR,
    llm_model VARCHAR,
    status VARCHAR NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    error VARCHAR
);
