#!/usr/bin/env python3
import argparse
import json
import sys
from datetime import datetime, timezone
from pathlib import Path


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--input-glob", required=True)
    parser.add_argument("--output-path", required=True)
    parser.add_argument("--record-type", required=True, choices=["comments", "submissions"])
    parser.add_argument("--clean-run-id", required=True)
    parser.add_argument("--duckdb-memory-limit", default="4GB")
    parser.add_argument("--duckdb-threads", type=int, default=4)
    parser.add_argument("--duckdb-temp-dir", default=".duckdb/tmp")
    return parser.parse_args()


def main():
    args = parse_args()

    try:
        import duckdb
    except ModuleNotFoundError as exc:
        emit_error(f"duckdb python package is not installed: {exc}")
        return 1

    input_glob = Path(args.input_glob)
    if not list(input_glob.parent.glob(input_glob.name)):
        emit_error(f"no raw parquet files matched {args.input_glob}")
        return 1

    output_path = Path(args.output_path)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    temp_output_path = output_path.with_name(output_path.name + ".tmp")
    temp_output_path.unlink(missing_ok=True)

    con = duckdb.connect()
    con.execute(f"SET memory_limit = '{args.duckdb_memory_limit}'")
    con.execute(f"SET threads = {max(args.duckdb_threads, 1)}")
    temp_dir = Path(args.duckdb_temp_dir)
    temp_dir.mkdir(parents=True, exist_ok=True)
    temp_dir_sql = str(temp_dir).replace("'", "''")
    con.execute(f"SET temp_directory = '{temp_dir_sql}'")

    input_sql = args.input_glob.replace("'", "''")
    output_sql = str(temp_output_path).replace("'", "''")
    clean_run_id_sql = args.clean_run_id.replace("'", "''")
    cleaned_at_sql = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M:%S")

    try:
        con.execute(build_copy_query(args.record_type, input_sql, output_sql, clean_run_id_sql, cleaned_at_sql))

        bytes_written = temp_output_path.stat().st_size if temp_output_path.exists() else 0
        if bytes_written == 0:
            temp_output_path.unlink(missing_ok=True)
            print(json.dumps({"status": "completed_zero_rows", "rows_written": 0, "bytes_written": 0, "output_path": "", "source_path": args.input_glob}))
            return 0

        rows_written = int(con.execute(f"SELECT count(*) FROM read_parquet('{output_sql}')").fetchone()[0])
        if rows_written == 0 and temp_output_path.exists():
            temp_output_path.unlink()
            bytes_written = 0
        elif rows_written > 0:
            temp_output_path.replace(output_path)

        print(
            json.dumps(
                {
                    "status": "completed" if rows_written > 0 else "completed_zero_rows",
                    "rows_written": rows_written,
                    "bytes_written": bytes_written,
                    "output_path": str(output_path) if rows_written > 0 else "",
                    "source_path": args.input_glob,
                }
            )
        )
        return 0
    except Exception as exc:  # pragma: no cover
        temp_output_path.unlink(missing_ok=True)
        emit_error(str(exc))
        return 1


def build_copy_query(record_type: str, input_sql: str, output_sql: str, clean_run_id_sql: str, cleaned_at_sql: str) -> str:
    if record_type == "comments":
        return f"""
            COPY (
                WITH source AS (
                    SELECT * FROM read_parquet('{input_sql}')
                ),
                deduped AS (
                    SELECT
                        *,
                        count(*) OVER (PARTITION BY id) AS raw_duplicate_count,
                        row_number() OVER (
                            PARTITION BY id
                            ORDER BY ingested_at DESC, source_file ASC
                        ) AS duplicate_rank
                    FROM source
                ),
                normalized AS (
                    SELECT
                        * EXCLUDE (duplicate_rank),
                        trim(coalesce(body, '')) AS body_raw_trimmed,
                        regexp_replace(trim(coalesce(body, '')), '\\s+', ' ', 'g') AS body_clean
                    FROM deduped
                    WHERE duplicate_rank = 1
                )
                SELECT
                    id,
                    author,
                    subreddit,
                    body,
                    score,
                    created_utc,
                    created_at,
                    body_length,
                    link_id,
                    parent_id,
                    distinguished,
                    author_flair_text,
                    source_file,
                    ingested_at,
                    manifest_id,
                    month,
                    year,
                    raw_duplicate_count,
                    body_clean,
                    lower(body_raw_trimmed) = '[deleted]' AS is_deleted,
                    lower(body_raw_trimmed) = '[removed]' AS is_removed,
                    (
                        lower(coalesce(author, '')) = 'automoderator'
                        OR regexp_matches(lower(coalesce(author, '')), '(^|[^a-z])bot([^a-z]|$)')
                        OR regexp_matches(lower(coalesce(body, '')), '^(i am a bot|this action was performed automatically|beep boop)')
                    ) AS is_bot_like,
                    length(body_clean) AS text_length,
                    NULL::VARCHAR AS language,
                    id AS raw_id,
                    TIMESTAMP '{cleaned_at_sql}' AS cleaned_at,
                    '{clean_run_id_sql}' AS clean_run_id
                FROM normalized
            )
            TO '{output_sql}'
            (FORMAT PARQUET, COMPRESSION ZSTD)
        """

    return f"""
        COPY (
            WITH source AS (
                SELECT * FROM read_parquet('{input_sql}')
            ),
            deduped AS (
                SELECT
                    *,
                    count(*) OVER (PARTITION BY id) AS raw_duplicate_count,
                    row_number() OVER (
                        PARTITION BY id
                        ORDER BY ingested_at DESC, source_file ASC
                    ) AS duplicate_rank
                FROM source
            ),
            normalized AS (
                SELECT
                    * EXCLUDE (duplicate_rank),
                    trim(coalesce(title, '')) AS title_raw_trimmed,
                    trim(coalesce(selftext, '')) AS selftext_raw_trimmed,
                    regexp_replace(trim(coalesce(title, '')), '\\s+', ' ', 'g') AS title_clean,
                    regexp_replace(trim(coalesce(selftext, '')), '\\s+', ' ', 'g') AS selftext_clean
                FROM deduped
                WHERE duplicate_rank = 1
            ),
            combined AS (
                SELECT
                    *,
                    CASE
                        WHEN title_clean = '' THEN selftext_clean
                        WHEN selftext_clean = '' THEN title_clean
                        ELSE title_clean || '\n\n' || selftext_clean
                    END AS combined_text
                FROM normalized
            )
            SELECT
                * EXCLUDE (title_raw_trimmed, selftext_raw_trimmed, title_clean, selftext_clean, combined_text),
                title_clean,
                selftext_clean,
                combined_text,
                (
                    lower(title_raw_trimmed) = '[deleted]'
                    OR lower(selftext_raw_trimmed) = '[deleted]'
                ) AS is_deleted,
                (
                    lower(title_raw_trimmed) = '[removed]'
                    OR lower(selftext_raw_trimmed) = '[removed]'
                ) AS is_removed,
                (
                    lower(coalesce(author, '')) = 'automoderator'
                    OR regexp_matches(lower(coalesce(author, '')), '(^|[^a-z])bot([^a-z]|$)')
                    OR regexp_matches(lower(coalesce(title, '') || ' ' || coalesce(selftext, '')), '^(i am a bot|this action was performed automatically|beep boop)')
                ) AS is_bot_like,
                length(combined_text) AS text_length,
                NULL::VARCHAR AS language,
                id AS raw_id,
                TIMESTAMP '{cleaned_at_sql}' AS cleaned_at,
                '{clean_run_id_sql}' AS clean_run_id
            FROM combined
        )
        TO '{output_sql}'
        (FORMAT PARQUET, COMPRESSION ZSTD)
    """


def emit_error(message: str):
    print(json.dumps({"status": "error", "rows_written": 0, "bytes_written": 0, "output_path": "", "source_path": "", "error": message}))


if __name__ == "__main__":
    sys.exit(main())
