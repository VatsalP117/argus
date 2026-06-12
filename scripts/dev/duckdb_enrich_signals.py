#!/usr/bin/env python3
import argparse
import json
import sys
from datetime import datetime, timezone
from pathlib import Path


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--input-glob", required=True)
    parser.add_argument("--signal-output-path", required=True)
    parser.add_argument("--entity-output-path", required=True)
    parser.add_argument("--metrics-output-path", required=True)
    parser.add_argument("--config-json", required=True)
    parser.add_argument("--record-type", required=True, choices=["comments", "submissions"])
    parser.add_argument("--signal-run-id", required=True)
    parser.add_argument("--month", required=True)
    parser.add_argument("--clean-dir", required=True)
    parser.add_argument("--marts-dir", required=True)
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

    config = json.loads(Path(args.config_json).read_text())
    input_glob = Path(args.input_glob)
    if not list(input_glob.parent.glob(input_glob.name)):
        emit_error(f"no clean parquet files matched {args.input_glob}")
        return 1

    con = duckdb.connect()
    con.execute(f"SET memory_limit = '{args.duckdb_memory_limit}'")
    con.execute(f"SET threads = {max(args.duckdb_threads, 1)}")
    temp_dir = Path(args.duckdb_temp_dir)
    temp_dir.mkdir(parents=True, exist_ok=True)
    temp_dir_sql = sql_string(str(temp_dir))
    con.execute(f"SET temp_directory = '{temp_dir_sql}'")

    source_type = "comment" if args.record_type == "comments" else "submission"
    analytical_text = "body_clean" if args.record_type == "comments" else "combined_text"
    input_sql = sql_string(args.input_glob)
    signal_run_id_sql = sql_string(args.signal_run_id)
    signaled_at_sql = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M:%S")

    signal_output_path = Path(args.signal_output_path)
    entity_output_path = Path(args.entity_output_path)
    metrics_output_path = Path(args.metrics_output_path)
    signal_output_path.parent.mkdir(parents=True, exist_ok=True)
    entity_output_path.parent.mkdir(parents=True, exist_ok=True)
    metrics_output_path.parent.mkdir(parents=True, exist_ok=True)

    signal_tmp = signal_output_path.with_name(signal_output_path.name + ".tmp")
    entity_tmp = entity_output_path.with_name(entity_output_path.name + ".tmp")
    metrics_tmp = metrics_output_path.with_name(metrics_output_path.name + ".tmp")
    for tmp in [signal_tmp, entity_tmp, metrics_tmp]:
        tmp.unlink(missing_ok=True)

    try:
        signal_query = build_signal_copy_query(
            input_sql=input_sql,
            output_sql=sql_string(str(signal_tmp)),
            source_type=source_type,
            analytical_text=analytical_text,
            signaled_at_sql=signaled_at_sql,
            signal_run_id_sql=signal_run_id_sql,
            signal_rules=config["signal_rules"],
            entity_rules=config["entity_rules"],
            signal_version=config["signal_version"],
        )
        con.execute(signal_query)
        signals_written, signal_output = finalize_temp_parquet(con, signal_tmp, signal_output_path)

        entity_query = build_entity_copy_query(
            input_sql=input_sql,
            output_sql=sql_string(str(entity_tmp)),
            source_type=source_type,
            analytical_text=analytical_text,
            signaled_at_sql=signaled_at_sql,
            signal_run_id_sql=signal_run_id_sql,
            entity_rules=config["entity_rules"],
            signal_version=config["signal_version"],
        )
        con.execute(entity_query)
        entity_rows_written, entity_output = finalize_temp_parquet(con, entity_tmp, entity_output_path)

        metrics_query = build_metrics_copy_query(
            clean_dir=args.clean_dir,
            marts_dir=args.marts_dir,
            month=args.month,
            output_sql=sql_string(str(metrics_tmp)),
        )
        con.execute(metrics_query)
        metrics_rows_written, metrics_output = finalize_temp_parquet(con, metrics_tmp, metrics_output_path)

        print(
            json.dumps(
                {
                    "status": "completed",
                    "signals_written": signals_written,
                    "entity_rows_written": entity_rows_written,
                    "metrics_rows_written": metrics_rows_written,
                    "signal_output_path": str(signal_output) if signal_output else "",
                    "entity_output_path": str(entity_output) if entity_output else "",
                    "metrics_output_path": str(metrics_output) if metrics_output else "",
                    "source_path": args.input_glob,
                }
            )
        )
        return 0
    except Exception as exc:  # pragma: no cover
        for tmp in [signal_tmp, entity_tmp, metrics_tmp]:
            tmp.unlink(missing_ok=True)
        emit_error(str(exc))
        return 1


def build_signal_copy_query(input_sql: str, output_sql: str, source_type: str, analytical_text: str, signaled_at_sql: str, signal_run_id_sql: str, signal_rules: list[dict], entity_rules: list[dict], signal_version: str) -> str:
    rules_values = ",\n                    ".join(
        f"('{sql_string(rule['signal_type'])}', '{sql_string(rule['label'])}', '{sql_string(rule['regex'])}', {str(bool(rule.get('require_topic_hint'))).upper()})"
        for rule in signal_rules
    )
    topic_hint_case = build_topic_hint_case(entity_rules)
    analytical_text_sql = analytical_text

    return f"""
        COPY (
            WITH source AS (
                SELECT
                    id AS source_id,
                    raw_id,
                    subreddit,
                    created_at,
                    created_utc,
                    source_file,
                    manifest_id,
                    clean_run_id,
                    month,
                    year,
                    score,
                    {analytical_text_sql} AS analytical_text,
                    {topic_hint_case} AS topic_hint
                FROM read_parquet('{input_sql}')
                WHERE NOT is_deleted
                  AND NOT is_removed
                  AND NOT is_bot_like
                  AND text_length > 0
                  AND {analytical_text_sql} IS NOT NULL
                  AND trim({analytical_text_sql}) <> ''
            ),
            rules(signal_type, matched_pattern, pattern_regex, require_topic_hint) AS (
                VALUES
                    {rules_values}
            )
            SELECT DISTINCT
                '{sql_string(source_type)}' AS source_type,
                source_id,
                raw_id,
                subreddit,
                created_at,
                created_utc,
                signal_type,
                1.0::DOUBLE AS signal_score,
                matched_pattern,
                analytical_text AS evidence_text,
                topic_hint,
                source_file,
                manifest_id,
                clean_run_id,
                month,
                year,
                '{sql_string(signal_version)}' AS signal_version,
                TIMESTAMP '{signaled_at_sql}' AS signaled_at,
                '{signal_run_id_sql}' AS signal_run_id
            FROM source
            JOIN rules
              ON regexp_matches(lower(analytical_text), pattern_regex)
            WHERE NOT require_topic_hint
               OR topic_hint IS NOT NULL
        )
        TO '{output_sql}'
        (FORMAT PARQUET, COMPRESSION ZSTD)
    """


def build_entity_copy_query(input_sql: str, output_sql: str, source_type: str, analytical_text: str, signaled_at_sql: str, signal_run_id_sql: str, entity_rules: list[dict], signal_version: str) -> str:
    rules_values = ",\n                    ".join(
        f"('{sql_string(rule['entity_type'])}', '{sql_string(rule['label'])}', '{sql_string(rule['regex'])}')"
        for rule in entity_rules
    )

    return f"""
        COPY (
            WITH source AS (
                SELECT
                    id AS source_id,
                    subreddit,
                    created_at,
                    created_utc,
                    source_file,
                    manifest_id,
                    clean_run_id,
                    month,
                    year,
                    {analytical_text} AS analytical_text
                FROM read_parquet('{input_sql}')
                WHERE NOT is_deleted
                  AND NOT is_removed
                  AND NOT is_bot_like
                  AND text_length > 0
                  AND {analytical_text} IS NOT NULL
                  AND trim({analytical_text}) <> ''
            ),
            rules(entity_type, normalized_entity, entity_regex) AS (
                VALUES
                    {rules_values}
            )
            SELECT DISTINCT
                '{sql_string(source_type)}' AS source_type,
                source_id,
                subreddit,
                created_at,
                created_utc,
                normalized_entity AS entity_text,
                entity_type,
                normalized_entity,
                analytical_text AS evidence_text,
                source_file,
                manifest_id,
                clean_run_id,
                month,
                year,
                '{sql_string(signal_version)}' AS signal_version,
                TIMESTAMP '{signaled_at_sql}' AS signaled_at,
                '{signal_run_id_sql}' AS signal_run_id
            FROM source
            JOIN rules
              ON regexp_matches(lower(analytical_text), entity_regex)
        )
        TO '{output_sql}'
        (FORMAT PARQUET, COMPRESSION ZSTD)
    """


def build_metrics_copy_query(clean_dir: str, marts_dir: str, month: str, output_sql: str) -> str:
    year, month_part = month.split("-", 1)
    clean_comments_glob = sql_string(f"{clean_dir}/comments/year={year}/month={month_part}/*.parquet")
    clean_submissions_glob = sql_string(f"{clean_dir}/submissions/year={year}/month={month_part}/*.parquet")
    signal_glob = sql_string(f"{marts_dir}/research_signals/year={year}/month={month_part}/*.parquet")

    return f"""
        COPY (
            WITH clean_union AS (
                SELECT
                    CAST(created_at AS DATE) AS day,
                    subreddit,
                    author,
                    score,
                    'comment' AS source_type
                FROM read_parquet('{clean_comments_glob}')
                UNION ALL
                SELECT
                    CAST(created_at AS DATE) AS day,
                    subreddit,
                    author,
                    score,
                    'submission' AS source_type
                FROM read_parquet('{clean_submissions_glob}')
            ),
            clean_agg AS (
                SELECT
                    day,
                    subreddit,
                    count(*) FILTER (WHERE source_type = 'submission') AS submission_count,
                    count(*) FILTER (WHERE source_type = 'comment') AS comment_count,
                    count(DISTINCT lower(author)) FILTER (WHERE author IS NOT NULL AND trim(author) <> '') AS unique_authors,
                    median(score) AS median_score
                FROM clean_union
                GROUP BY day, subreddit
            ),
            signal_agg AS (
                SELECT
                    CAST(created_at AS DATE) AS day,
                    subreddit,
                    count(*) FILTER (WHERE signal_type = 'pain_point') AS pain_point_count,
                    count(*) FILTER (WHERE signal_type = 'feature_request') AS feature_request_count,
                    count(*) FILTER (WHERE signal_type = 'recommendation_request') AS recommendation_request_count,
                    count(*) FILTER (WHERE signal_type = 'workaround') AS workaround_count,
                    count(*) FILTER (WHERE signal_type = 'comparison') AS comparison_count
                FROM read_parquet('{signal_glob}')
                GROUP BY day, subreddit
            )
            SELECT
                clean_agg.day,
                clean_agg.subreddit,
                clean_agg.submission_count,
                clean_agg.comment_count,
                clean_agg.unique_authors,
                clean_agg.median_score,
                coalesce(signal_agg.pain_point_count, 0) AS pain_point_count,
                coalesce(signal_agg.feature_request_count, 0) AS feature_request_count,
                coalesce(signal_agg.recommendation_request_count, 0) AS recommendation_request_count,
                coalesce(signal_agg.workaround_count, 0) AS workaround_count,
                coalesce(signal_agg.comparison_count, 0) AS comparison_count
            FROM clean_agg
            LEFT JOIN signal_agg
              ON clean_agg.day = signal_agg.day
             AND clean_agg.subreddit = signal_agg.subreddit
        )
        TO '{output_sql}'
        (FORMAT PARQUET, COMPRESSION ZSTD)
    """


def build_topic_hint_case(entity_rules: list[dict]) -> str:
    if not entity_rules:
        return "NULL::VARCHAR"

    lines = ["CASE"]
    for rule in entity_rules:
        lines.append(
            f"    WHEN regexp_matches(lower(analytical_text), '{sql_string(rule['regex'])}') THEN '{sql_string(rule['label'])}'"
        )
    lines.append("    ELSE NULL::VARCHAR")
    lines.append("END")
    return "\n".join(lines)


def finalize_temp_parquet(con, temp_path: Path, output_path: Path):
    if not temp_path.exists():
        return 0, None

    rows_written = int(con.execute(f"SELECT count(*) FROM read_parquet('{sql_string(str(temp_path))}')").fetchone()[0])
    if rows_written == 0:
        temp_path.unlink(missing_ok=True)
        return 0, None

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
                "signals_written": 0,
                "entity_rows_written": 0,
                "metrics_rows_written": 0,
                "signal_output_path": "",
                "entity_output_path": "",
                "metrics_output_path": "",
                "source_path": "",
                "error": message,
            }
        )
    )


if __name__ == "__main__":
    sys.exit(main())
