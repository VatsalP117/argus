#!/usr/bin/env python3
import argparse
import hashlib
import json
import sys
from datetime import datetime, timezone
from pathlib import Path


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--database-path", required=True)
    parser.add_argument("--candidate-path", required=True)
    parser.add_argument("--score-path", required=True)
    parser.add_argument("--metadata-json", required=True)
    return parser.parse_args()


def sql_string(value: str):
    return "'" + value.replace("'", "''") + "'"


def stable_id(prefix: str, *parts):
    digest = hashlib.sha256("\0".join(str(part) for part in parts).encode()).hexdigest()
    return f"{prefix}-{digest[:24]}"


def signal_values(mappings: list[dict]):
    if not mappings:
        return None
    return ", ".join(
        f"({sql_string(item['rule_group'])}, {sql_string(item['signal_type'])}, {float(item['score'])})"
        for item in mappings
    )


def ensure_score_metadata(
    con,
    staging_batch_id,
    score_path,
    score_checksum,
    score_bytes,
    relevance_version,
    relevance_config_hash,
):
    current = con.execute(
        """
        SELECT
            score_path,
            score_checksum,
            score_bytes,
            relevance_version,
            relevance_config_hash
        FROM staged_candidate_batches
        WHERE staging_batch_id = ?
        """,
        [staging_batch_id],
    ).fetchone()
    if not current:
        return
    expected = (
        str(score_path),
        score_checksum,
        score_bytes,
        relevance_version,
        relevance_config_hash,
    )
    if all(value is None for value in current):
        con.execute(
            """
            UPDATE staged_candidate_batches
            SET score_path = ?,
                score_checksum = ?,
                score_bytes = ?,
                relevance_version = ?,
                relevance_config_hash = ?
            WHERE staging_batch_id = ?
            """,
            [*expected, staging_batch_id],
        )
    elif current != expected:
        raise ValueError("durable score staging metadata differs from commit input")


def existing_result(
    con,
    ingest_batch_id,
    scan_run_id,
    staging_batch_id,
    score_path,
    score_checksum,
    score_bytes,
    relevance_version,
    relevance_config_hash,
    include_review_tier,
):
    batch = con.execute(
        """
        SELECT
            source_rows,
            staged_rows,
            retained_rows,
            rejected_rows,
            quarantined_rows,
            durable_checksum,
            coalesce(cleanup_status, 'pending')
        FROM ingest_batches
        WHERE ingest_batch_id = ?
          AND status = 'validated'
        """,
        [ingest_batch_id],
    ).fetchone()
    if not batch:
        return None
    ensure_score_metadata(
        con,
        staging_batch_id,
        score_path,
        score_checksum,
        score_bytes,
        relevance_version,
        relevance_config_hash,
    )

    requested_predicate = (
        "decision IN ('retain', 'evaluate')"
        if include_review_tier
        else "decision = 'retain'"
    )
    requested_retained_rows = con.execute(
        f"""
        SELECT count(DISTINCT (source_type, source_id))
        FROM read_parquet({sql_string(str(score_path))})
        WHERE {requested_predicate}
        """
    ).fetchone()[0]
    if requested_retained_rows != batch[2]:
        raise ValueError(
            f"ingest batch {ingest_batch_id} was already committed with a "
            f"different retention scope (stored retained_rows={batch[2]}, "
            f"requested scope would retain={requested_retained_rows}); "
            f"--include-review-tier cannot extend an existing batch, drop the "
            f"batch or use a fresh manifest entry"
        )

    reconciliation = con.execute(
        """
        SELECT
            rows_rejected_early,
            source_equation_valid,
            staging_equation_valid
        FROM batch_reconciliation
        WHERE ingest_batch_id = ?
        """,
        [ingest_batch_id],
    ).fetchone()
    relevance_rows = con.execute(
        """
        SELECT count(*)
        FROM document_relevance relevance
        JOIN documents documents USING (document_id)
        WHERE documents.ingest_batch_id = ?
        """,
        [ingest_batch_id],
    ).fetchone()[0]
    signals_written = con.execute(
        """
        SELECT count(*)
        FROM signals signals
        JOIN documents documents USING (document_id)
        WHERE documents.ingest_batch_id = ?
        """,
        [ingest_batch_id],
    ).fetchone()[0]
    entities_written = con.execute(
        """
        SELECT count(*)
        FROM entities entities
        JOIN documents documents USING (document_id)
        WHERE documents.ingest_batch_id = ?
        """,
        [ingest_batch_id],
    ).fetchone()[0]
    rows_review_tier = con.execute(
        """
        SELECT count(*)
        FROM (
            SELECT documents.document_id
            FROM documents documents
            JOIN document_relevance relevance USING (document_id)
            WHERE documents.ingest_batch_id = ?
            GROUP BY documents.document_id
            HAVING bool_or(relevance.decision = 'evaluate')
               AND NOT bool_or(relevance.decision = 'retain')
        )
        """,
        [ingest_batch_id],
    ).fetchone()[0]

    return {
        "status": "skipped_existing",
        "scan_run_id": scan_run_id,
        "staging_batch_id": staging_batch_id,
        "ingest_batch_id": ingest_batch_id,
        "rows_seen": batch[0],
        "rows_rejected_early": reconciliation[0],
        "rows_staged": batch[1],
        "rows_retained": batch[2],
        "rows_rejected_late": batch[3],
        "rows_quarantined": batch[4],
        "relevance_rows": relevance_rows,
        "signals_written": signals_written,
        "entities_written": entities_written,
        "rows_review_tier": rows_review_tier,
        "source_equation_valid": reconciliation[1],
        "staging_equation_valid": reconciliation[2],
        "durable_checksum": batch[5],
        "cleanup_status": batch[6],
    }


def commit(args, metadata):
    try:
        import duckdb
    except ModuleNotFoundError as exc:
        raise RuntimeError(f"duckdb python package is not installed: {exc}") from exc

    candidate_path = Path(args.candidate_path)
    score_path = Path(args.score_path)
    if not candidate_path.is_file():
        raise ValueError(f"candidate staging does not exist: {candidate_path}")
    if not score_path.is_file():
        raise ValueError(f"score staging does not exist: {score_path}")

    manifest = metadata["manifest"]
    entry = metadata["entry"]
    checkpoint = metadata["checkpoint"]
    relevance = metadata["relevance_rules"]
    include_review_tier = bool(metadata.get("include_review_tier", False))
    scan_run_id = stable_id(
        "scan",
        manifest["manifest_id"],
        entry["entry_id"],
        entry["source_identity"],
        checkpoint["candidate_version"],
        checkpoint["candidate_config_hash"],
    )
    staging_batch_id = stable_id("staging", scan_run_id)
    ingest_batch_id = stable_id(
        "ingest",
        scan_run_id,
        relevance["version"],
        metadata["relevance_config_hash"],
    )
    now = datetime.now(timezone.utc).isoformat()

    con = duckdb.connect(args.database_path)
    try:
        schema_version = con.execute(
            "SELECT coalesce(max(version), 0) FROM schema_migrations"
        ).fetchone()[0]
        if schema_version < 3:
            raise ValueError(
                f"database schema version {schema_version} is too old; run db-migrate"
            )

        score_bytes = score_path.stat().st_size
        prior = existing_result(
            con,
            ingest_batch_id,
            scan_run_id,
            staging_batch_id,
            score_path,
            metadata["score_checksum"],
            score_bytes,
            relevance["version"],
            metadata["relevance_config_hash"],
            include_review_tier,
        )
        if prior:
            return prior

        con.execute(
            f"""
            CREATE TEMP VIEW candidate_source AS
            SELECT * FROM read_parquet({sql_string(str(candidate_path))})
            """
        )
        con.execute(
            f"""
            CREATE TEMP VIEW relevance_source AS
            SELECT * FROM read_parquet({sql_string(str(score_path))})
            """
        )

        rows_staged = con.execute(
            "SELECT count(*) FROM candidate_source"
        ).fetchone()[0]
        distinct_candidates = con.execute(
            """
            SELECT count(*)
            FROM (
                SELECT source_type, source_id
                FROM candidate_source
                GROUP BY source_type, source_id
            )
            """
        ).fetchone()[0]
        if rows_staged != checkpoint["result"]["rows_candidates"]:
            raise ValueError(
                "candidate row count does not match the scan checkpoint"
            )
        if distinct_candidates != rows_staged:
            raise ValueError("candidate staging contains duplicate source IDs")
        candidate_identity_mismatches = con.execute(
            """
            SELECT count(*)
            FROM candidate_source
            WHERE manifest_id <> ?
               OR manifest_entry_id <> ?
               OR source_identity <> ?
               OR candidate_version <> ?
            """,
            [
                manifest["manifest_id"],
                entry["entry_id"],
                entry["source_identity"],
                checkpoint["candidate_version"],
            ],
        ).fetchone()[0]
        if candidate_identity_mismatches:
            raise ValueError(
                "candidate staging contains rows with mismatched source identity"
            )

        domain_count = len(relevance["domains"])
        score_rows = con.execute(
            "SELECT count(*) FROM relevance_source"
        ).fetchone()[0]
        distinct_score_rows = con.execute(
            """
            SELECT count(*)
            FROM (
                SELECT source_type, source_id, domain
                FROM relevance_source
                GROUP BY source_type, source_id, domain
            )
            """
        ).fetchone()[0]
        if score_rows != rows_staged * domain_count or distinct_score_rows != score_rows:
            raise ValueError("relevance score rows do not reconcile by candidate and domain")
        relevance_versions = con.execute(
            "SELECT array_agg(DISTINCT relevance_version) FROM relevance_source"
        ).fetchone()[0]
        if relevance_versions != [relevance["version"]]:
            raise ValueError("relevance staging version does not match relevance config")
        score_orphans = con.execute(
            """
            SELECT count(*)
            FROM relevance_source scores
            LEFT JOIN candidate_source candidates
              USING (source_type, source_id)
            WHERE candidates.source_id IS NULL
            """
        ).fetchone()[0]
        if score_orphans:
            raise ValueError("relevance staging contains source IDs absent from candidates")

        retain_decision_predicate = (
            "scores.decision IN ('retain', 'evaluate')"
            if include_review_tier
            else "scores.decision = 'retain'"
        )

        con.execute(
            f"""
            CREATE TEMP TABLE retained_candidates AS
            SELECT candidates.*
            FROM candidate_source candidates
            WHERE EXISTS (
                SELECT 1
                FROM relevance_source scores
                WHERE scores.source_type = candidates.source_type
                  AND scores.source_id = candidates.source_id
                  AND {retain_decision_predicate}
            )
            """
        )
        rows_retained = con.execute(
            "SELECT count(*) FROM retained_candidates"
        ).fetchone()[0]
        rows_rejected_late = rows_staged - rows_retained
        rows_review_tier = con.execute(
            """
            SELECT count(*)
            FROM retained_candidates retained
            WHERE EXISTS (
                SELECT 1
                FROM relevance_source scores
                WHERE scores.source_type = retained.source_type
                  AND scores.source_id = retained.source_id
                  AND scores.decision = 'evaluate'
            )
            AND NOT EXISTS (
                SELECT 1
                FROM relevance_source scores
                WHERE scores.source_type = retained.source_type
                  AND scores.source_id = retained.source_id
                  AND scores.decision = 'retain'
            )
            """
        ).fetchone()[0]
        existing_documents = con.execute(
            """
            SELECT count(*)
            FROM retained_candidates retained
            JOIN documents documents
              ON documents.source_type = retained.source_type
             AND documents.source_id = retained.source_id
            """
        ).fetchone()[0]
        if existing_documents:
            raise ValueError(
                "retained source IDs already exist in a different durable batch"
            )

        rows_seen = checkpoint["result"]["rows_seen"]
        rows_rejected_early = checkpoint["result"]["rows_rejected_early"]
        source_equation_valid = rows_seen == rows_rejected_early + rows_staged
        staging_equation_valid = rows_staged == rows_retained + rows_rejected_late
        if not source_equation_valid or not staging_equation_valid:
            raise ValueError("batch reconciliation equations failed before commit")

        con.execute("BEGIN TRANSACTION")
        try:
            existing_manifest = con.execute(
                """
                SELECT manifest_checksum
                FROM source_manifests
                WHERE manifest_id = ?
                """,
                [manifest["manifest_id"]],
            ).fetchone()
            if existing_manifest and existing_manifest[0] != metadata["manifest_checksum"]:
                raise ValueError("manifest checksum differs from durable manifest record")

            con.execute(
                """
                INSERT OR IGNORE INTO source_manifests (
                    manifest_id,
                    dataset_repo,
                    archive_revision,
                    pipeline_name,
                    generated_at,
                    entry_count,
                    bytes_total,
                    manifest_path,
                    manifest_checksum
                )
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
                """,
                [
                    manifest["manifest_id"],
                    manifest["dataset_repo"],
                    manifest["archive_revision"],
                    manifest["pipeline_name"],
                    manifest["generated_at"],
                    manifest["summary"]["entry_count"],
                    manifest["summary"]["bytes_total"],
                    metadata["manifest_path"],
                    metadata["manifest_checksum"],
                ],
            )
            con.execute(
                """
                INSERT INTO candidate_scan_runs (
                    scan_run_id,
                    manifest_id,
                    manifest_entry_id,
                    source_identity,
                    record_type,
                    source_path,
                    archive_repo,
                    archive_revision,
                    candidate_version,
                    candidate_config_hash,
                    status,
                    rows_seen,
                    rows_candidates,
                    rows_rejected_early,
                    subreddit_prior_candidates,
                    bytes_written,
                    staging_path,
                    staging_checksum,
                    started_at,
                    finished_at
                )
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'completed', ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """,
                [
                    scan_run_id,
                    manifest["manifest_id"],
                    entry["entry_id"],
                    entry["source_identity"],
                    entry["record_type"],
                    entry["shard_path"],
                    manifest["dataset_repo"],
                    entry["archive_revision"],
                    checkpoint["candidate_version"],
                    checkpoint["candidate_config_hash"],
                    rows_seen,
                    rows_staged,
                    rows_rejected_early,
                    checkpoint["result"]["subreddit_prior_candidates"],
                    checkpoint["result"]["bytes_written"],
                    str(candidate_path),
                    metadata["candidate_checksum"],
                    checkpoint["started_at"],
                    checkpoint["finished_at"],
                ],
            )
            for group, matched_rows in checkpoint["result"]["matched_by_group"].items():
                con.execute(
                    """
                    INSERT INTO candidate_rule_yields (
                        scan_run_id, rule_group, matched_rows
                    )
                    VALUES (?, ?, ?)
                    """,
                    [scan_run_id, group, matched_rows],
                )
            con.execute(
                """
                INSERT INTO staged_candidate_batches (
                    staging_batch_id,
                    scan_run_id,
                    manifest_id,
                    manifest_entry_id,
                    source_identity,
                    candidate_version,
                    status,
                    staging_path,
                    staging_checksum,
                    staging_bytes,
                    candidate_rows,
                    validated_at,
                    commit_started_at,
                    score_path,
                    score_checksum,
                    score_bytes,
                    relevance_version,
                    relevance_config_hash
                )
                VALUES (?, ?, ?, ?, ?, ?, 'commit_started', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """,
                [
                    staging_batch_id,
                    scan_run_id,
                    manifest["manifest_id"],
                    entry["entry_id"],
                    entry["source_identity"],
                    checkpoint["candidate_version"],
                    str(candidate_path),
                    metadata["candidate_checksum"],
                    checkpoint["result"]["bytes_written"],
                    rows_staged,
                    now,
                    now,
                    str(score_path),
                    metadata["score_checksum"],
                    score_bytes,
                    relevance["version"],
                    metadata["relevance_config_hash"],
                ],
            )
            con.execute(
                """
                INSERT INTO ingest_batches (
                    ingest_batch_id,
                    manifest_id,
                    status,
                    started_at,
                    source_rows,
                    staged_rows,
                    retained_rows,
                    rejected_rows,
                    quarantined_rows,
                    staging_batch_id,
                    candidate_version,
                    staging_checksum,
                    cleanup_status
                )
                VALUES (?, ?, 'commit_started', ?, ?, ?, ?, ?, 0, ?, ?, ?, 'pending')
                """,
                [
                    ingest_batch_id,
                    manifest["manifest_id"],
                    now,
                    rows_seen,
                    rows_staged,
                    rows_retained,
                    rows_rejected_late,
                    staging_batch_id,
                    checkpoint["candidate_version"],
                    metadata["candidate_checksum"],
                ],
            )

            salt = sql_string(metadata["author_hash_salt"])
            ingest_id = sql_string(ingest_batch_id)
            relevance_version = sql_string(relevance["version"])
            source_shard_size = int(entry["size_bytes"])
            con.execute(
                f"""
                INSERT INTO documents (
                    document_id,
                    source_type,
                    source_id,
                    raw_id,
                    thread_id,
                    parent_id,
                    subreddit,
                    author_hash,
                    created_at,
                    score,
                    title,
                    original_text,
                    clean_text,
                    text_length,
                    source_url,
                    archive_repo,
                    archive_revision,
                    source_file,
                    source_shard_size,
                    manifest_id,
                    ingest_batch_id,
                    clean_version,
                    is_deleted,
                    is_removed,
                    is_bot_like
                )
                SELECT
                    source_type || ':' || source_id,
                    source_type,
                    source_id,
                    raw_id,
                    thread_id,
                    parent_id,
                    subreddit,
                    CASE
                        WHEN author IS NULL OR trim(author) = '' THEN NULL
                        ELSE sha256({salt} || ':' || lower(author))
                    END,
                    created_at,
                    score,
                    title,
                    original_text,
                    candidate_text,
                    length(candidate_text),
                    coalesce(source_url, source_file),
                    archive_repo,
                    archive_revision,
                    source_file,
                    {source_shard_size},
                    manifest_id,
                    {ingest_id},
                    candidate_version || '+' || {relevance_version},
                    is_deleted,
                    is_removed,
                    is_bot_like
                FROM retained_candidates
                """
            )
            con.execute(
                """
                INSERT INTO document_relevance (
                    document_id,
                    domain,
                    relevance_score,
                    relevance_tier,
                    matched_terms,
                    matched_rules,
                    subreddit_prior,
                    signal_prior,
                    semantic_score,
                    classifier_score,
                    decision,
                    decision_reasons,
                    relevance_version,
                    scored_at
                )
                SELECT
                    scores.source_type || ':' || scores.source_id,
                    scores.domain,
                    scores.relevance_score,
                    scores.relevance_tier,
                    scores.matched_terms,
                    scores.matched_rules,
                    scores.subreddit_prior,
                    scores.signal_prior,
                    NULL,
                    NULL,
                    scores.decision,
                    scores.decision_reasons,
                    scores.relevance_version,
                    scores.scored_at
                FROM relevance_source scores
                JOIN retained_candidates retained
                  USING (source_type, source_id)
                """
            )

            mappings = signal_values(relevance.get("signal_mappings", []))
            if mappings:
                con.execute(
                    f"""
                    INSERT INTO signals (
                        signal_id,
                        document_id,
                        signal_type,
                        signal_score,
                        matched_pattern,
                        evidence_text,
                        signal_version
                    )
                    WITH mappings(rule_group, signal_type, signal_score) AS (
                        VALUES {mappings}
                    )
                    SELECT DISTINCT
                        sha256(
                            retained.source_type || ':' || retained.source_id
                            || '|' || mappings.rule_group
                            || '|' || mappings.signal_type
                            || '|' || {relevance_version}
                        ),
                        retained.source_type || ':' || retained.source_id,
                        mappings.signal_type,
                        mappings.signal_score,
                        mappings.rule_group,
                        retained.candidate_text,
                        {relevance_version}
                    FROM retained_candidates retained
                    JOIN mappings
                      ON json_contains(
                          retained.matched_rule_groups,
                          to_json(mappings.rule_group)
                      )
                    """
                )
            con.execute(
                f"""
                INSERT INTO entities (
                    entity_mention_id,
                    document_id,
                    entity_type,
                    entity_text,
                    normalized_entity,
                    entity_version
                )
                SELECT DISTINCT
                    sha256(
                        retained.source_type || ':' || retained.source_id
                        || '|candidate_term|' || lower(term)
                        || '|' || {relevance_version}
                    ),
                    retained.source_type || ':' || retained.source_id,
                    'candidate_term',
                    term,
                    lower(term),
                    {relevance_version}
                FROM retained_candidates retained,
                UNNEST(from_json(retained.matched_terms, '["VARCHAR"]')) terms(term)
                WHERE term IS NOT NULL
                  AND trim(term) <> ''
                """
            )

            relevance_rows = con.execute(
                """
                SELECT count(*)
                FROM document_relevance relevance
                JOIN documents documents USING (document_id)
                WHERE documents.ingest_batch_id = ?
                """,
                [ingest_batch_id],
            ).fetchone()[0]
            expected_relevance_rows = rows_retained * domain_count
            if relevance_rows != expected_relevance_rows:
                raise ValueError(
                    "post-write relevance row count does not match retained documents"
                )
            durable_document_count = con.execute(
                "SELECT count(*) FROM documents WHERE ingest_batch_id = ?",
                [ingest_batch_id],
            ).fetchone()[0]
            if durable_document_count != rows_retained:
                raise ValueError(
                    "post-write durable document count does not match retained rows"
                )
            signals_written = con.execute(
                """
                SELECT count(*)
                FROM signals signals
                JOIN documents documents USING (document_id)
                WHERE documents.ingest_batch_id = ?
                """,
                [ingest_batch_id],
            ).fetchone()[0]
            entities_written = con.execute(
                """
                SELECT count(*)
                FROM entities entities
                JOIN documents documents USING (document_id)
                WHERE documents.ingest_batch_id = ?
                """,
                [ingest_batch_id],
            ).fetchone()[0]
            durable_checksum = con.execute(
                """
                SELECT sha256(string_agg(document_id, ',' ORDER BY document_id))
                FROM documents
                WHERE ingest_batch_id = ?
                """,
                [ingest_batch_id],
            ).fetchone()[0]

            con.execute(
                """
                INSERT INTO batch_reconciliation (
                    ingest_batch_id,
                    rows_seen,
                    rows_rejected_early,
                    rows_staged,
                    rows_retained,
                    rows_rejected_late,
                    rows_quarantined,
                    source_equation_valid,
                    staging_equation_valid,
                    scan_run_id,
                    rows_candidates
                )
                VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?)
                """,
                [
                    ingest_batch_id,
                    rows_seen,
                    rows_rejected_early,
                    rows_staged,
                    rows_retained,
                    rows_rejected_late,
                    source_equation_valid,
                    staging_equation_valid,
                    scan_run_id,
                    rows_staged,
                ],
            )
            con.execute(
                """
                UPDATE ingest_batches
                SET status = 'validated',
                    committed_at = ?,
                    validated_at = ?,
                    durable_checksum = ?
                WHERE ingest_batch_id = ?
                """,
                [now, now, durable_checksum, ingest_batch_id],
            )
            con.execute(
                """
                UPDATE staged_candidate_batches
                SET status = 'committed_validated',
                    committed_at = ?
                WHERE staging_batch_id = ?
                """,
                [now, staging_batch_id],
            )
            con.execute(
                """
                INSERT OR IGNORE INTO pipeline_versions (
                    component, version, checksum
                )
                VALUES ('candidate_rules', ?, ?),
                       ('relevance_rules', ?, ?)
                """,
                [
                    checkpoint["candidate_version"],
                    checkpoint["candidate_config_hash"],
                    relevance["version"],
                    metadata["relevance_config_hash"],
                ],
            )
            con.execute("COMMIT")
        except Exception:
            con.execute("ROLLBACK")
            raise

        return {
            "status": "completed",
            "scan_run_id": scan_run_id,
            "staging_batch_id": staging_batch_id,
            "ingest_batch_id": ingest_batch_id,
            "rows_seen": rows_seen,
            "rows_rejected_early": rows_rejected_early,
            "rows_staged": rows_staged,
            "rows_retained": rows_retained,
            "rows_rejected_late": rows_rejected_late,
            "rows_quarantined": 0,
            "relevance_rows": relevance_rows,
            "signals_written": signals_written,
            "entities_written": entities_written,
            "rows_review_tier": rows_review_tier,
            "source_equation_valid": source_equation_valid,
            "staging_equation_valid": staging_equation_valid,
            "durable_checksum": durable_checksum,
            "cleanup_status": "pending",
        }
    finally:
        con.close()


def main():
    args = parse_args()
    try:
        metadata = json.loads(args.metadata_json)
        print(json.dumps(commit(args, metadata)))
        return 0
    except Exception as exc:
        print(json.dumps({"status": "error", "error": str(exc)}))
        return 1


if __name__ == "__main__":
    sys.exit(main())
