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
    parser.add_argument("--ingest-batch-id", required=True)
    return parser.parse_args()


def file_sha256(path: Path):
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return "sha256:" + digest.hexdigest()


def event_id(staging_batch_id: str, kind: str):
    digest = hashlib.sha256(f"{staging_batch_id}\0{kind}".encode()).hexdigest()
    return f"cleanup-{digest[:24]}"


def load_batch(con, ingest_batch_id):
    return con.execute(
        """
        SELECT
            ingest.ingest_batch_id,
            ingest.status,
            coalesce(ingest.cleanup_status, 'pending'),
            ingest.source_rows,
            ingest.staged_rows,
            ingest.retained_rows,
            ingest.rejected_rows,
            ingest.quarantined_rows,
            ingest.durable_checksum,
            reconciliation.source_equation_valid,
            reconciliation.staging_equation_valid,
            staging.staging_batch_id,
            staging.status,
            staging.staging_path,
            staging.staging_checksum,
            staging.staging_bytes,
            staging.score_path,
            staging.score_checksum,
            staging.score_bytes
        FROM ingest_batches ingest
        JOIN batch_reconciliation reconciliation
          USING (ingest_batch_id)
        JOIN staged_candidate_batches staging
          ON ingest.staging_batch_id = staging.staging_batch_id
        WHERE ingest.ingest_batch_id = ?
        """,
        [ingest_batch_id],
    ).fetchone()


def existing_event_status(con, cleanup_event_id):
    row = con.execute(
        """
        SELECT status
        FROM staging_cleanup_events
        WHERE cleanup_event_id = ?
        """,
        [cleanup_event_id],
    ).fetchone()
    return row[0] if row else None


def validate_durable(con, batch):
    ingest_batch_id = batch[0]
    if batch[1] != "validated":
        raise ValueError(f"ingest batch is not validated: {batch[1]}")
    if not batch[9] or not batch[10]:
        raise ValueError("batch reconciliation equations are not valid")

    document_count = con.execute(
        "SELECT count(*) FROM documents WHERE ingest_batch_id = ?",
        [ingest_batch_id],
    ).fetchone()[0]
    if document_count != batch[5]:
        raise ValueError("durable document count differs from retained row count")
    invalid_documents = con.execute(
        """
        SELECT count(*)
        FROM documents
        WHERE ingest_batch_id = ?
          AND (
              is_bot_like
              OR source_url IS NULL
              OR trim(source_url) = ''
          )
        """,
        [ingest_batch_id],
    ).fetchone()[0]
    if invalid_documents:
        raise ValueError("durable batch contains invalid retained documents")

    durable_checksum = con.execute(
        """
        SELECT sha256(string_agg(document_id, ',' ORDER BY document_id))
        FROM documents
        WHERE ingest_batch_id = ?
        """,
        [ingest_batch_id],
    ).fetchone()[0]
    if durable_checksum != batch[8]:
        raise ValueError("durable document checksum differs from validated batch")
    return durable_checksum


def validate_staging(con, batch, files):
    candidate_path = files[0]["path"]
    score_path = files[1]["path"]
    candidate_rows = con.execute(
        f"SELECT count(*) FROM read_parquet('{str(candidate_path).replace(chr(39), chr(39) * 2)}')"
    ).fetchone()[0]
    if candidate_rows != batch[4]:
        raise ValueError("candidate staging row count differs from durable batch")

    durable_relevance_rows = con.execute(
        """
        SELECT count(*)
        FROM document_relevance relevance
        JOIN documents documents USING (document_id)
        WHERE documents.ingest_batch_id = ?
        """,
        [batch[0]],
    ).fetchone()[0]
    staged_retained_scores = con.execute(
        f"""
        SELECT count(*)
        FROM read_parquet('{str(score_path).replace(chr(39), chr(39) * 2)}') scores
        JOIN documents documents
          ON documents.source_type = scores.source_type
         AND documents.source_id = scores.source_id
        WHERE documents.ingest_batch_id = ?
        """,
        [batch[0]],
    ).fetchone()[0]
    if staged_retained_scores != durable_relevance_rows:
        raise ValueError("relevance staging rows differ from durable relevance rows")


def cleanup(args):
    try:
        import duckdb
    except ModuleNotFoundError as exc:
        raise RuntimeError(f"duckdb python package is not installed: {exc}") from exc

    con = duckdb.connect(args.database_path)
    try:
        batch = load_batch(con, args.ingest_batch_id)
        if not batch:
            raise ValueError(f"ingest batch was not found: {args.ingest_batch_id}")

        staging_batch_id = batch[11]
        files = [
            {
                "kind": "candidate",
                "path": Path(batch[13]),
                "checksum": batch[14],
                "bytes": batch[15],
            },
            {
                "kind": "score",
                "path": Path(batch[16]),
                "checksum": batch[17],
                "bytes": batch[18],
            },
        ]
        if any(not item["path"] or not item["checksum"] for item in files):
            raise ValueError("staging metadata is incomplete")

        if batch[2] == "completed":
            if any(item["path"].exists() for item in files):
                raise ValueError("cleanup is marked complete but staging files still exist")
            return {
                "status": "skipped_existing",
                "ingest_batch_id": args.ingest_batch_id,
                "staging_batch_id": staging_batch_id,
                "files_removed": 0,
                "bytes_removed": 0,
                "durable_checksum": batch[8],
                "cleanup_status": "completed",
            }

        durable_checksum = validate_durable(con, batch)
        resumable_missing = False
        for item in files:
            item["event_id"] = event_id(staging_batch_id, item["kind"])
            item["event_status"] = existing_event_status(con, item["event_id"])
            if item["path"].exists():
                if item["path"].stat().st_size != item["bytes"]:
                    raise ValueError(f"{item['kind']} staging size differs from durable metadata")
                if file_sha256(item["path"]) != item["checksum"]:
                    raise ValueError(f"{item['kind']} staging checksum differs from durable metadata")
            elif item["event_status"] in {"started", "completed"}:
                resumable_missing = True
            else:
                raise ValueError(f"{item['kind']} staging is missing without a cleanup event")

        if not resumable_missing:
            validate_staging(con, batch, files)

        now = datetime.now(timezone.utc).isoformat()
        for item in files:
            con.execute(
                """
                INSERT OR IGNORE INTO staging_cleanup_events (
                    cleanup_event_id,
                    staging_batch_id,
                    staging_path,
                    staging_checksum,
                    bytes_before,
                    status,
                    attempted_at
                )
                VALUES (?, ?, ?, ?, ?, 'started', ?)
                """,
                [
                    item["event_id"],
                    staging_batch_id,
                    str(item["path"]),
                    item["checksum"],
                    item["bytes"],
                    now,
                ],
            )

        files_removed = 0
        bytes_removed = 0
        for item in files:
            try:
                if item["path"].exists():
                    item["path"].unlink()
                    files_removed += 1
                    bytes_removed += item["bytes"]
                con.execute(
                    """
                    UPDATE staging_cleanup_events
                    SET status = 'completed',
                        finished_at = ?,
                        error = NULL
                    WHERE cleanup_event_id = ?
                    """,
                    [datetime.now(timezone.utc).isoformat(), item["event_id"]],
                )
            except Exception as exc:
                con.execute(
                    """
                    UPDATE staging_cleanup_events
                    SET status = 'failed',
                        finished_at = ?,
                        error = ?
                    WHERE cleanup_event_id = ?
                    """,
                    [
                        datetime.now(timezone.utc).isoformat(),
                        str(exc),
                        item["event_id"],
                    ],
                )
                raise

        finished_at = datetime.now(timezone.utc).isoformat()
        con.execute("BEGIN TRANSACTION")
        try:
            con.execute(
                """
                UPDATE staged_candidate_batches
                SET status = 'cleaned',
                    cleaned_at = ?
                WHERE staging_batch_id = ?
                """,
                [finished_at, staging_batch_id],
            )
            con.execute(
                """
                UPDATE ingest_batches
                SET cleanup_status = 'completed'
                WHERE ingest_batch_id = ?
                """,
                [args.ingest_batch_id],
            )
            con.execute("COMMIT")
        except Exception:
            con.execute("ROLLBACK")
            raise

        return {
            "status": "completed",
            "ingest_batch_id": args.ingest_batch_id,
            "staging_batch_id": staging_batch_id,
            "files_removed": files_removed,
            "bytes_removed": bytes_removed,
            "durable_checksum": durable_checksum,
            "cleanup_status": "completed",
        }
    finally:
        con.close()


def main():
    args = parse_args()
    try:
        print(json.dumps(cleanup(args)))
        return 0
    except Exception as exc:
        print(json.dumps({"status": "error", "error": str(exc)}))
        return 1


if __name__ == "__main__":
    sys.exit(main())
