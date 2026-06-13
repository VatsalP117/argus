#!/usr/bin/env python3
import argparse
import json
import re
import sys
from pathlib import Path


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--input-url", required=True)
    parser.add_argument("--output-path", required=True)
    parser.add_argument("--record-type", required=True, choices=["comments", "submissions"])
    parser.add_argument("--entry-id", required=True)
    parser.add_argument("--manifest-id", required=True)
    parser.add_argument("--dataset-repo", required=True)
    parser.add_argument("--archive-revision", required=True)
    parser.add_argument("--source-path", required=True)
    parser.add_argument("--source-identity", required=True)
    parser.add_argument("--rules-json", required=True)
    parser.add_argument("--duckdb-memory-limit", default="4GB")
    parser.add_argument("--duckdb-threads", type=int, default=4)
    parser.add_argument("--duckdb-temp-dir", default=".duckdb/tmp")
    return parser.parse_args()


def sql_string(value: str):
    return "'" + value.replace("'", "''") + "'"


def term_match_expression(column: str, term: str):
    normalized = term.lower()
    if re.fullmatch(r"[a-z0-9]+", normalized):
        pattern = rf"(^|[^a-z0-9]){re.escape(normalized)}([^a-z0-9]|$)"
        return f"regexp_matches({column}, {sql_string(pattern)})"
    return f"contains({column}, {sql_string(normalized)})"


def term_expression(column: str, terms: list[str]):
    return " OR ".join(term_match_expression(column, term) for term in terms)


def projection_sql(args):
    common = f"""
        id AS source_id,
        id AS raw_id,
        author,
        subreddit,
        score,
        to_timestamp(created_utc) AS created_at,
        created_utc,
        {sql_string(args.dataset_repo)} AS archive_repo,
        {sql_string(args.archive_revision)} AS archive_revision,
        {sql_string(args.source_path)} AS source_file,
        {sql_string(args.source_identity)} AS source_identity,
        {sql_string(args.manifest_id)} AS manifest_id,
        {sql_string(args.entry_id)} AS manifest_entry_id
    """
    if args.record_type == "comments":
        return f"""
            SELECT
                'comment' AS source_type,
                {common},
                link_id AS thread_id,
                parent_id,
                NULL::VARCHAR AS title,
                coalesce(body, '') AS original_text,
                trim(regexp_replace(coalesce(body, ''), '\\s+', ' ', 'g')) AS candidate_text,
                CASE
                    WHEN link_id IS NOT NULL AND starts_with(link_id, 't3_')
                        THEN 'https://www.reddit.com/comments/' || substr(link_id, 4) || '/_/' || id
                    ELSE NULL
                END AS source_url
            FROM read_parquet({sql_string(args.input_url)})
        """
    return f"""
        SELECT
            'submission' AS source_type,
            {common},
            id AS thread_id,
            NULL::VARCHAR AS parent_id,
            coalesce(title, '') AS title,
            CASE
                WHEN trim(coalesce(title, '')) = '' THEN coalesce(selftext, '')
                WHEN trim(coalesce(selftext, '')) = '' THEN coalesce(title, '')
                ELSE coalesce(title, '') || '\n\n' || coalesce(selftext, '')
            END AS original_text,
            trim(regexp_replace(
                coalesce(title, '') || ' ' || coalesce(selftext, ''),
                '\\s+',
                ' ',
                'g'
            )) AS candidate_text,
            'https://www.reddit.com/comments/' || id AS source_url
        FROM read_parquet({sql_string(args.input_url)})
    """


def scan(args, rules):
    try:
        import duckdb
    except ModuleNotFoundError as exc:
        raise RuntimeError(f"duckdb python package is not installed: {exc}") from exc

    output_path = Path(args.output_path)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    temp_output_path = output_path.with_name(output_path.name + ".tmp")
    temp_output_path.unlink(missing_ok=True)

    temp_dir = Path(args.duckdb_temp_dir)
    temp_dir.mkdir(parents=True, exist_ok=True)

    con = duckdb.connect()
    try:
        if re.match(r"^https?://", args.input_url):
            con.execute("INSTALL httpfs")
            con.execute("LOAD httpfs")
        con.execute(f"SET memory_limit = {sql_string(args.duckdb_memory_limit)}")
        con.execute(f"SET threads = {max(args.duckdb_threads, 1)}")
        con.execute(f"SET temp_directory = {sql_string(str(temp_dir))}")

        con.execute(f"CREATE TEMP TABLE projected_source AS {projection_sql(args)}")
        rows_seen = int(con.execute("SELECT count(*) FROM projected_source").fetchone()[0])

        group_aliases = []
        group_columns = []
        group_names = []
        matched_term_cases = []
        for index, group in enumerate(rules["rule_groups"]):
            alias = f"rule_{index}"
            expression = term_expression("lower(candidate_text)", group["terms"])
            group_aliases.append(alias)
            group_columns.append(f"({expression}) AS {alias}")
            group_names.append(group["name"])
            for term in group["terms"]:
                matched_term_cases.append(
                    f"CASE WHEN {term_match_expression('lower(candidate_text)', term)} "
                    f"THEN {sql_string(term)} ELSE NULL::VARCHAR END"
                )

        priors = ", ".join(sql_string(item.lower()) for item in rules["subreddit_priors"])
        prior_expression = (
            f"lower(coalesce(subreddit, '')) IN ({priors})" if priors else "false"
        )
        excluded = ", ".join(
            sql_string(item.lower()) for item in rules["excluded_exact_text"]
        )
        excluded_expression = (
            f"lower(trim(candidate_text)) NOT IN ({excluded})" if excluded else "true"
        )

        any_group = " OR ".join(group_aliases)
        group_name_cases = ", ".join(
            f"CASE WHEN {alias} THEN {sql_string(name)} ELSE NULL::VARCHAR END"
            for alias, name in zip(group_aliases, group_names)
        )
        term_cases = ", ".join(matched_term_cases)
        reason_cases = group_name_cases
        if reason_cases:
            reason_cases += ", "
        reason_cases += (
            "CASE WHEN subreddit_prior_match THEN 'subreddit_prior' "
            "ELSE NULL::VARCHAR END"
        )

        con.execute(
            f"""
            CREATE TEMP TABLE scanned_candidates AS
            WITH eligible AS (
                SELECT *
                FROM projected_source
                WHERE length(candidate_text) >= {int(rules["minimum_text_length"])}
                  AND {excluded_expression}
            ),
            annotated AS (
                SELECT
                    *,
                    {prior_expression} AS subreddit_prior_match,
                    {", ".join(group_columns)}
                FROM eligible
            )
            SELECT
                *,
                {sql_string(rules["version"])} AS candidate_version,
                to_json(list_filter([{group_name_cases}], item -> item IS NOT NULL))
                    AS matched_rule_groups,
                to_json(list_filter([{term_cases}], item -> item IS NOT NULL))
                    AS matched_terms,
                to_json(list_filter([{reason_cases}], item -> item IS NOT NULL))
                    AS candidate_reasons,
                current_timestamp AS scanned_at
            FROM annotated
            WHERE subreddit_prior_match OR {any_group}
            """
        )

        rows_candidates = int(
            con.execute("SELECT count(*) FROM scanned_candidates").fetchone()[0]
        )
        matched_by_group = {}
        for alias, name in zip(group_aliases, group_names):
            matched_by_group[name] = int(
                con.execute(
                    f"SELECT count(*) FROM scanned_candidates WHERE {alias}"
                ).fetchone()[0]
            )
        prior_only_filter = " AND ".join(f"NOT {alias}" for alias in group_aliases)
        subreddit_prior_candidates = int(
            con.execute(
                "SELECT count(*) FROM scanned_candidates "
                f"WHERE subreddit_prior_match AND {prior_only_filter}"
            ).fetchone()[0]
        )

        bytes_written = 0
        final_output_path = ""
        if rows_candidates > 0:
            excluded_aliases = ", ".join(group_aliases)
            output_sql = sql_string(str(temp_output_path))
            con.execute(
                f"""
                COPY (
                    SELECT * EXCLUDE ({excluded_aliases})
                    FROM scanned_candidates
                )
                TO {output_sql}
                (FORMAT PARQUET, COMPRESSION ZSTD)
                """
            )
            bytes_written = temp_output_path.stat().st_size
            temp_output_path.replace(output_path)
            final_output_path = str(output_path)

        return {
            "status": "completed" if rows_candidates > 0 else "completed_zero_rows",
            "entry_id": args.entry_id,
            "record_type": args.record_type,
            "rows_seen": rows_seen,
            "rows_candidates": rows_candidates,
            "rows_rejected_early": rows_seen - rows_candidates,
            "bytes_written": bytes_written,
            "output_path": final_output_path,
            "candidate_version": rules["version"],
            "matched_by_group": matched_by_group,
            "subreddit_prior_candidates": subreddit_prior_candidates,
        }
    finally:
        con.close()
        if temp_output_path.exists():
            temp_output_path.unlink()


def main():
    args = parse_args()
    try:
        rules = json.loads(args.rules_json)
        print(json.dumps(scan(args, rules)))
        return 0
    except Exception as exc:
        print(json.dumps({"status": "error", "error": str(exc)}))
        return 1


if __name__ == "__main__":
    sys.exit(main())
