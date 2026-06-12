#!/usr/bin/env python3
import argparse
import csv
import glob
import json
import re
import sys
from datetime import date, datetime
from decimal import Decimal
from pathlib import Path


DISALLOWED_SQL_PATTERNS = [
    r"\bINSERT\b",
    r"\bUPDATE\b",
    r"\bDELETE\b",
    r"\bCOPY\b",
    r"\bEXPORT\b",
    r"\bIMPORT\b",
    r"\bATTACH\b",
    r"\bDETACH\b",
    r"\bINSTALL\b",
    r"\bLOAD\b",
    r"\bPRAGMA\b",
    r"\bALTER\b",
    r"\bDROP\b",
    r"\bCREATE\b",
    r"\bREPLACE\b",
    r"\bTRUNCATE\b",
    r"\bCALL\b",
    r"\bread_parquet\s*\(",
    r"\bread_csv\s*\(",
    r"\bread_json\s*\(",
    r"\bread_text\s*\(",
]


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--query-name", required=True, choices=["signal_summary", "signal_evidence", "entity_summary", "subreddit_metrics", "source_search", "custom_sql"])
    parser.add_argument("--clean-dir", required=True)
    parser.add_argument("--marts-dir", required=True)
    parser.add_argument("--months", default="*")
    parser.add_argument("--signal-type", default="*")
    parser.add_argument("--topic-hint", default="*")
    parser.add_argument("--subreddit", default="*")
    parser.add_argument("--source-type", default="*")
    parser.add_argument("--entity-type", default="*")
    parser.add_argument("--entity-text", default="*")
    parser.add_argument("--matched-pattern", default="*")
    parser.add_argument("--contains-text", default="")
    parser.add_argument("--sql-file", default="")
    parser.add_argument("--limit", type=int, default=50)
    parser.add_argument("--output-format", choices=["json", "csv"], default="json")
    parser.add_argument("--output-path", default="")
    parser.add_argument("--include-sql", action="store_true")
    parser.add_argument("--duckdb-memory-limit", default="4GB")
    parser.add_argument("--duckdb-threads", type=int, default=4)
    parser.add_argument("--duckdb-temp-dir", default=".duckdb/tmp")
    return parser.parse_args()


def main():
    args = parse_args()

    if args.output_format == "csv" and not args.output_path:
        emit_error("output-path is required when output-format=csv")
        return 1

    try:
        import duckdb
    except ModuleNotFoundError as exc:
        emit_error(f"duckdb python package is not installed: {exc}")
        return 1

    try:
        months = normalize_months(args.months)
    except ValueError as exc:
        emit_error(str(exc))
        return 1

    con = duckdb.connect()
    con.execute(f"SET memory_limit = '{sql_string(args.duckdb_memory_limit)}'")
    con.execute(f"SET threads = {max(args.duckdb_threads, 1)}")

    temp_dir = Path(args.duckdb_temp_dir)
    temp_dir.mkdir(parents=True, exist_ok=True)
    con.execute(f"SET temp_directory = '{sql_string(str(temp_dir))}'")

    try:
        register_views(con, args.clean_dir, args.marts_dir, months)
        sql = build_query_sql(args)
        if args.output_format == "csv":
            output_path = Path(args.output_path)
            output_path.parent.mkdir(parents=True, exist_ok=True)
            row_count = export_csv(con, sql, output_path)
            payload = {
                "status": "completed",
                "query_name": args.query_name,
                "output_format": args.output_format,
                "row_count": row_count,
                "columns": [],
                "rows": [],
                "output_path": str(output_path),
            }
            if args.include_sql:
                payload["sql"] = sql
            print(json.dumps(payload))
            return 0

        rows, columns = execute_json_query(con, sql)
        payload = {
            "status": "completed",
            "query_name": args.query_name,
            "output_format": args.output_format,
            "row_count": len(rows),
            "columns": columns,
            "rows": rows,
            "output_path": "",
        }
        if args.output_path:
            output_path = Path(args.output_path)
            output_path.parent.mkdir(parents=True, exist_ok=True)
            output_path.write_text(json.dumps(payload, indent=2))
            payload["output_path"] = str(output_path)
        if args.include_sql:
            payload["sql"] = sql
        print(json.dumps(payload))
        return 0
    except Exception as exc:  # pragma: no cover
        emit_error(str(exc))
        return 1


def register_views(con, clean_dir: str, marts_dir: str, months: list[str]):
    comment_source = build_source_expression(clean_dir, "comments", months)
    submission_source = build_source_expression(clean_dir, "submissions", months)
    signal_source = build_source_expression(marts_dir, "research_signals", months)
    entity_source = build_source_expression(marts_dir, "entity_mentions", months)
    metrics_source = build_source_expression(marts_dir, "subreddit_metrics_daily", months)

    con.execute(
        f"""
        CREATE OR REPLACE TEMP VIEW source_documents AS
        SELECT
            'comment' AS source_type,
            id AS source_id,
            raw_id,
            subreddit,
            author,
            created_at,
            created_utc,
            score,
            body_clean AS analytical_text,
            text_length,
            clean_run_id,
            source_file,
            manifest_id,
            month,
            year
        FROM read_parquet({comment_source})
        UNION ALL
        SELECT
            'submission' AS source_type,
            id AS source_id,
            raw_id,
            subreddit,
            author,
            created_at,
            created_utc,
            score,
            combined_text AS analytical_text,
            text_length,
            clean_run_id,
            source_file,
            manifest_id,
            month,
            year
        FROM read_parquet({submission_source})
        """
    )

    con.execute(f"CREATE OR REPLACE TEMP VIEW research_signals AS SELECT * FROM read_parquet({signal_source})")
    con.execute(f"CREATE OR REPLACE TEMP VIEW entity_mentions AS SELECT * FROM read_parquet({entity_source})")
    con.execute(f"CREATE OR REPLACE TEMP VIEW subreddit_metrics_daily AS SELECT * FROM read_parquet({metrics_source})")


def build_query_sql(args) -> str:
    limit = max(args.limit, 1)

    if args.query_name == "signal_summary":
        conditions = build_signal_conditions(args)
        return f"""
            WITH filtered AS (
                SELECT *
                FROM research_signals
                WHERE {conditions}
            )
            SELECT
                signal_type,
                coalesce(topic_hint, 'unclassified') AS topic_hint,
                matched_pattern,
                count(*) AS signal_count,
                count(DISTINCT subreddit) AS subreddit_count,
                min(created_at) AS first_seen_at,
                max(created_at) AS last_seen_at
            FROM filtered
            GROUP BY 1, 2, 3
            ORDER BY signal_count DESC, subreddit_count DESC, signal_type, topic_hint, matched_pattern
            LIMIT {limit}
        """

    if args.query_name == "signal_evidence":
        conditions = build_signal_conditions(args)
        return f"""
            WITH filtered AS (
                SELECT *
                FROM research_signals
                WHERE {conditions}
            )
            SELECT
                filtered.signal_type,
                coalesce(filtered.topic_hint, 'unclassified') AS topic_hint,
                filtered.matched_pattern,
                filtered.subreddit,
                filtered.source_type,
                filtered.source_id,
                filtered.raw_id,
                filtered.created_at,
                source_documents.score,
                left(filtered.evidence_text, 500) AS evidence_text,
                filtered.source_file,
                filtered.manifest_id,
                filtered.clean_run_id,
                filtered.signal_run_id
            FROM filtered
            LEFT JOIN source_documents
              ON filtered.source_type = source_documents.source_type
             AND filtered.source_id = source_documents.source_id
            ORDER BY filtered.created_at DESC, filtered.source_id
            LIMIT {limit}
        """

    if args.query_name == "entity_summary":
        conditions = build_entity_conditions(args)
        return f"""
            WITH filtered AS (
                SELECT *
                FROM entity_mentions
                WHERE {conditions}
            )
            SELECT
                entity_type,
                normalized_entity,
                count(*) AS mention_count,
                count(DISTINCT subreddit) AS subreddit_count,
                min(created_at) AS first_seen_at,
                max(created_at) AS last_seen_at
            FROM filtered
            GROUP BY 1, 2
            ORDER BY mention_count DESC, subreddit_count DESC, entity_type, normalized_entity
            LIMIT {limit}
        """

    if args.query_name == "subreddit_metrics":
        conditions = build_metrics_conditions(args)
        return f"""
            SELECT
                day,
                subreddit,
                submission_count,
                comment_count,
                unique_authors,
                median_score,
                pain_point_count,
                feature_request_count,
                recommendation_request_count,
                workaround_count,
                comparison_count,
                round(
                    (
                        pain_point_count
                        + feature_request_count
                        + recommendation_request_count
                        + workaround_count
                        + comparison_count
                    )::DOUBLE / nullif(comment_count + submission_count, 0),
                    4
                ) AS signal_density
            FROM subreddit_metrics_daily
            WHERE {conditions}
            ORDER BY day DESC, subreddit
            LIMIT {limit}
        """

    if args.query_name == "source_search":
        conditions = build_source_conditions(args)
        return f"""
            SELECT
                source_type,
                source_id,
                raw_id,
                subreddit,
                created_at,
                score,
                left(analytical_text, 500) AS analytical_text,
                clean_run_id,
                source_file,
                manifest_id
            FROM source_documents
            WHERE {conditions}
            ORDER BY created_at DESC, source_id
            LIMIT {limit}
        """

    return build_custom_sql(args.sql_file, limit)


def build_signal_conditions(args) -> str:
    clauses = ["TRUE"]
    if normalize_filter(args.signal_type) != "*":
        clauses.append(f"lower(signal_type) = '{sql_string(normalize_filter(args.signal_type))}'")
    if normalize_filter(args.topic_hint) != "*":
        clauses.append(f"lower(coalesce(topic_hint, '')) = '{sql_string(normalize_filter(args.topic_hint))}'")
    if normalize_filter(args.subreddit) != "*":
        clauses.append(f"lower(subreddit) = '{sql_string(normalize_filter(args.subreddit))}'")
    if normalize_filter(args.source_type) != "*":
        clauses.append(f"lower(source_type) = '{sql_string(normalize_filter(args.source_type))}'")
    if normalize_filter(args.matched_pattern) != "*":
        clauses.append(f"lower(matched_pattern) = '{sql_string(normalize_filter(args.matched_pattern))}'")
    return " AND ".join(clauses)


def build_entity_conditions(args) -> str:
    clauses = ["TRUE"]
    if normalize_filter(args.entity_type) != "*":
        clauses.append(f"lower(entity_type) = '{sql_string(normalize_filter(args.entity_type))}'")
    if normalize_filter(args.entity_text) != "*":
        clauses.append(f"lower(normalized_entity) = '{sql_string(normalize_filter(args.entity_text))}'")
    if normalize_filter(args.subreddit) != "*":
        clauses.append(f"lower(subreddit) = '{sql_string(normalize_filter(args.subreddit))}'")
    if normalize_filter(args.source_type) != "*":
        clauses.append(f"lower(source_type) = '{sql_string(normalize_filter(args.source_type))}'")
    return " AND ".join(clauses)


def build_metrics_conditions(args) -> str:
    clauses = ["TRUE"]
    if normalize_filter(args.subreddit) != "*":
        clauses.append(f"lower(subreddit) = '{sql_string(normalize_filter(args.subreddit))}'")
    return " AND ".join(clauses)


def build_source_conditions(args) -> str:
    clauses = [
        "analytical_text IS NOT NULL",
        "trim(analytical_text) <> ''",
    ]
    if normalize_filter(args.subreddit) != "*":
        clauses.append(f"lower(subreddit) = '{sql_string(normalize_filter(args.subreddit))}'")
    if normalize_filter(args.source_type) != "*":
        clauses.append(f"lower(source_type) = '{sql_string(normalize_filter(args.source_type))}'")
    contains_text = args.contains_text.strip().lower()
    if contains_text:
        clauses.append(f"lower(analytical_text) LIKE '%{sql_string(contains_text)}%'")
    return " AND ".join(clauses)


def build_custom_sql(sql_file: str, limit: int) -> str:
    if not sql_file:
        raise ValueError("sql-file is required when query-name=custom_sql")

    sql_path = Path(sql_file)
    if not sql_path.exists():
        raise ValueError(f"sql file does not exist: {sql_file}")

    raw_sql = sql_path.read_text().strip()
    if raw_sql.endswith(";"):
        raw_sql = raw_sql[:-1].strip()
    if ";" in raw_sql:
        raise ValueError("custom sql must be a single statement")

    if not re.match(r"^(SELECT|WITH)\b", raw_sql, flags=re.IGNORECASE):
        raise ValueError("custom sql must begin with SELECT or WITH")

    for pattern in DISALLOWED_SQL_PATTERNS:
        if re.search(pattern, raw_sql, flags=re.IGNORECASE):
            raise ValueError(f"custom sql contains a disallowed pattern: {pattern}")

    return f"SELECT * FROM ({raw_sql}) AS query_result LIMIT {limit}"


def export_csv(con, sql: str, output_path: Path) -> int:
    output_sql = sql_string(str(output_path))
    con.execute(f"COPY ({sql}) TO '{output_sql}' (FORMAT CSV, HEADER)")
    with output_path.open() as handle:
        return max(sum(1 for _ in csv.reader(handle)) - 1, 0)


def execute_json_query(con, sql: str):
    rows = con.execute(sql).fetchall()
    columns = [item[0] for item in con.description]
    result_rows = []
    for row in rows:
        item = {}
        for idx, column in enumerate(columns):
            item[column] = serialize_value(row[idx])
        result_rows.append(item)
    return result_rows, columns


def serialize_value(value):
    if isinstance(value, (datetime, date)):
        return value.isoformat()
    if isinstance(value, Decimal):
        return float(value)
    if isinstance(value, list):
        return [serialize_value(item) for item in value]
    return value


def build_source_expression(base_dir: str, dataset_name: str, months: list[str]) -> str:
    patterns = []
    if months == ["*"]:
        patterns = [str(Path(base_dir) / dataset_name / "year=*" / "month=*" / "*.parquet")]
    else:
        for month in months:
            year, month_part = month.split("-", 1)
            patterns.append(str(Path(base_dir) / dataset_name / f"year={year}" / f"month={month_part}" / "*.parquet"))

    matched = []
    for pattern in patterns:
        matched.extend(glob.glob(pattern))

    if not matched:
        raise ValueError(f"no parquet files matched {dataset_name} for months {', '.join(months)}")

    sql_patterns = ",".join(f"'{sql_string(pattern)}'" for pattern in patterns)
    if len(patterns) == 1:
        return f"'{sql_string(patterns[0])}'"
    return f"[{sql_patterns}]"


def normalize_months(raw_months: str) -> list[str]:
    cleaned = raw_months.strip()
    if not cleaned or cleaned == "*":
        return ["*"]

    months = []
    for item in cleaned.split(","):
        month = item.strip()
        if not re.match(r"^\d{4}-\d{2}$", month):
            raise ValueError(f"invalid month value: {month}")
        months.append(month)
    return months


def normalize_filter(value: str) -> str:
    cleaned = value.strip().lower()
    if not cleaned:
        return "*"
    return cleaned


def sql_string(value: str) -> str:
    return value.replace("'", "''")


def emit_error(message: str):
    print(
        json.dumps(
            {
                "status": "error",
                "query_name": "",
                "output_format": "",
                "row_count": 0,
                "columns": [],
                "rows": [],
                "output_path": "",
                "error": message,
            }
        )
    )


if __name__ == "__main__":
    sys.exit(main())
