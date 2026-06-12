#!/usr/bin/env python3
import argparse
import glob
import json
import sys
from pathlib import Path


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--signal-glob", required=True)
    parser.add_argument("--summary-output-path", required=True)
    parser.add_argument("--evidence-output-path", required=True)
    parser.add_argument("--signal-type", required=True)
    parser.add_argument("--topic-hint", default="*")
    parser.add_argument("--max-groups", type=int, default=25)
    parser.add_argument("--examples-per-group", type=int, default=5)
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

    if not glob.glob(args.signal_glob):
        emit_error(f"no signal parquet files matched {args.signal_glob}")
        return 1

    con = duckdb.connect()
    con.execute(f"SET memory_limit = '{args.duckdb_memory_limit}'")
    con.execute(f"SET threads = {max(args.duckdb_threads, 1)}")
    temp_dir = Path(args.duckdb_temp_dir)
    temp_dir.mkdir(parents=True, exist_ok=True)
    con.execute(f"SET temp_directory = '{sql_string(str(temp_dir))}'")

    summary_output_path = Path(args.summary_output_path)
    evidence_output_path = Path(args.evidence_output_path)
    summary_output_path.parent.mkdir(parents=True, exist_ok=True)
    evidence_output_path.parent.mkdir(parents=True, exist_ok=True)

    summary_tmp = summary_output_path.with_name(summary_output_path.name + ".tmp")
    evidence_tmp = evidence_output_path.with_name(evidence_output_path.name + ".tmp")
    for tmp in [summary_tmp, evidence_tmp]:
        tmp.unlink(missing_ok=True)

    filter_sql = build_filter_clause(args.signal_type, args.topic_hint)

    try:
        summary_query = build_summary_export_query(
            signal_glob=sql_string(args.signal_glob),
            output_path=sql_string(str(summary_tmp)),
            filter_sql=filter_sql,
            max_groups=max(args.max_groups, 1),
        )
        con.execute(summary_query)

        evidence_query = build_evidence_export_query(
            signal_glob=sql_string(args.signal_glob),
            output_path=sql_string(str(evidence_tmp)),
            filter_sql=filter_sql,
            max_groups=max(args.max_groups, 1),
            examples_per_group=max(args.examples_per_group, 1),
        )
        con.execute(evidence_query)

        summary_rows_written, summary_output = finalize_temp_csv(summary_tmp, summary_output_path)
        evidence_rows_written, evidence_output = finalize_temp_csv(evidence_tmp, evidence_output_path)

        print(
            json.dumps(
                {
                    "status": "completed",
                    "summary_rows_written": summary_rows_written,
                    "evidence_rows_written": evidence_rows_written,
                    "summary_output_path": str(summary_output) if summary_output else "",
                    "evidence_output_path": str(evidence_output) if evidence_output else "",
                }
            )
        )
        return 0
    except Exception as exc:  # pragma: no cover
        for tmp in [summary_tmp, evidence_tmp]:
            tmp.unlink(missing_ok=True)
        emit_error(str(exc))
        return 1


def build_filter_clause(signal_type: str, topic_hint: str) -> str:
    filters = [f"signal_type = '{sql_string(signal_type)}'"]
    normalized_topic_hint = topic_hint.strip().lower()
    if normalized_topic_hint and normalized_topic_hint != "*":
        filters.append(f"lower(coalesce(topic_hint, '')) = '{sql_string(normalized_topic_hint)}'")
    return " AND ".join(filters)


def build_summary_export_query(signal_glob: str, output_path: str, filter_sql: str, max_groups: int) -> str:
    return f"""
        COPY (
            WITH filtered AS (
                SELECT
                    coalesce(topic_hint, 'unclassified') AS topic_hint,
                    matched_pattern,
                    subreddit,
                    created_at
                FROM read_parquet('{signal_glob}')
                WHERE {filter_sql}
            )
            SELECT
                topic_hint,
                matched_pattern,
                count(*) AS signal_count,
                count(DISTINCT subreddit) AS subreddit_count,
                min(created_at) AS first_seen_at,
                max(created_at) AS last_seen_at
            FROM filtered
            GROUP BY 1, 2
            ORDER BY signal_count DESC, subreddit_count DESC, topic_hint, matched_pattern
            LIMIT {max_groups}
        )
        TO '{output_path}'
        (FORMAT CSV, HEADER)
    """


def build_evidence_export_query(signal_glob: str, output_path: str, filter_sql: str, max_groups: int, examples_per_group: int) -> str:
    return f"""
        COPY (
            WITH filtered AS (
                SELECT
                    coalesce(topic_hint, 'unclassified') AS topic_hint,
                    matched_pattern,
                    subreddit,
                    created_at,
                    source_type,
                    raw_id,
                    source_id,
                    evidence_text,
                    source_file,
                    manifest_id,
                    clean_run_id,
                    signal_run_id
                FROM read_parquet('{signal_glob}')
                WHERE {filter_sql}
            ),
            top_groups AS (
                SELECT
                    topic_hint,
                    matched_pattern,
                    count(*) AS signal_count
                FROM filtered
                GROUP BY 1, 2
                ORDER BY signal_count DESC, topic_hint, matched_pattern
                LIMIT {max_groups}
            ),
            ranked AS (
                SELECT
                    filtered.*,
                    row_number() OVER (
                        PARTITION BY filtered.topic_hint, filtered.matched_pattern
                        ORDER BY filtered.created_at DESC, filtered.raw_id
                    ) AS example_rank
                FROM filtered
                JOIN top_groups
                  ON filtered.topic_hint = top_groups.topic_hint
                 AND filtered.matched_pattern = top_groups.matched_pattern
            )
            SELECT
                topic_hint,
                matched_pattern,
                example_rank,
                subreddit,
                created_at,
                source_type,
                raw_id,
                source_id,
                evidence_text,
                source_file,
                manifest_id,
                clean_run_id,
                signal_run_id
            FROM ranked
            WHERE example_rank <= {examples_per_group}
            ORDER BY topic_hint, matched_pattern, example_rank
        )
        TO '{output_path}'
        (FORMAT CSV, HEADER)
    """


def finalize_temp_csv(temp_path: Path, output_path: Path):
    if not temp_path.exists():
        return 0, None

    with temp_path.open() as handle:
        rows_written = max(sum(1 for _ in handle) - 1, 0)

    output_path.parent.mkdir(parents=True, exist_ok=True)
    temp_path.replace(output_path)
    return rows_written, output_path


def sql_string(value: str) -> str:
    return value.replace("'", "''")


def emit_error(message: str):
    print(
        json.dumps(
            {
                "status": "error",
                "summary_rows_written": 0,
                "evidence_rows_written": 0,
                "summary_output_path": "",
                "evidence_output_path": "",
                "error": message,
            }
        )
    )


if __name__ == "__main__":
    sys.exit(main())
