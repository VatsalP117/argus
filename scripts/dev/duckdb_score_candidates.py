#!/usr/bin/env python3
import argparse
import json
import re
import sys
from pathlib import Path


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--input-path", required=True)
    parser.add_argument("--output-path", required=True)
    parser.add_argument("--rules-json", required=True)
    parser.add_argument("--duckdb-memory-limit", default="4GB")
    parser.add_argument("--duckdb-threads", type=int, default=4)
    parser.add_argument("--duckdb-temp-dir", default=".duckdb/tmp")
    return parser.parse_args()


def sql_string(value: str):
    return "'" + value.replace("'", "''") + "'"


def group_match(group: str):
    return f"json_contains(matched_rule_groups, {sql_string(json.dumps(group))})"


def term_match(term: str):
    return f"json_contains(matched_terms, {sql_string(json.dumps(term))})"


def text_match(term: str):
    normalized = term.lower()
    if re.fullmatch(r"[a-z0-9]+", normalized):
        pattern = rf"(^|[^a-z0-9]){re.escape(normalized)}([^a-z0-9]|$)"
        return f"regexp_matches(candidate_match_text, {sql_string(pattern)})"
    return f"contains(candidate_match_text, {sql_string(normalized)})"


def build_domain_query(domain: dict):
    score_parts = []
    penalty_parts = []
    reason_parts = []
    matched_group_parts = []
    for group, weight in domain["group_weights"].items():
        matches = group_match(group)
        score_parts.append(f"CASE WHEN {matches} THEN {float(weight)} ELSE 0 END")
        matched_group_parts.append(f"CASE WHEN {matches} THEN 1 ELSE 0 END")
        reason_parts.append(
            f"CASE WHEN {matches} THEN {sql_string(group)} ELSE NULL::VARCHAR END"
        )
    for term, weight in (domain.get("context_weights") or {}).items():
        matches = text_match(term)
        score_parts.append(f"CASE WHEN {matches} THEN {float(weight)} ELSE 0 END")
        reason_parts.append(
            f"CASE WHEN {matches} THEN {sql_string('context:' + term)} "
            "ELSE NULL::VARCHAR END"
        )
    for term, weight in (domain.get("context_penalty_weights") or {}).items():
        matches = text_match(term)
        penalty_parts.append(f"CASE WHEN {matches} THEN {float(weight)} ELSE 0 END")
        reason_parts.append(
            f"CASE WHEN {matches} THEN {sql_string('penalty:' + term)} "
            "ELSE NULL::VARCHAR END"
        )

    prior_weight = float(domain.get("subreddit_prior_weight", 0))
    prior_score = (
        f"CASE WHEN subreddit_prior_match THEN {prior_weight} ELSE 0 END"
    )
    raw_score_expression = " + ".join(score_parts + [prior_score])
    penalty_expression = " + ".join(penalty_parts) if penalty_parts else "0"
    required_terms = domain.get("required_any_terms") or []
    if required_terms:
        requirement = " OR ".join(term_match(term) for term in required_terms)
    else:
        requirement = "true"
    required_groups = domain.get("required_any_groups") or []
    if required_groups:
        group_requirement = " OR ".join(group_match(group) for group in required_groups)
    else:
        group_requirement = "true"
    minimum_group_matches = int(domain.get("minimum_group_matches", 0))
    if minimum_group_matches > 0:
        minimum_group_requirement = (
            "(" + " + ".join(matched_group_parts) + f") >= {minimum_group_matches}"
        )
    else:
        minimum_group_requirement = "true"
    eligible = (
        f"(NOT is_bot_like AND ({requirement}) AND ({group_requirement}) "
        f"AND ({minimum_group_requirement}))"
    )
    score_expression = (
        f"CASE WHEN {eligible} "
        f"THEN greatest(0, ({raw_score_expression}) - ({penalty_expression})) "
        "ELSE 0 END"
    )
    reasons = ", ".join(
        reason_parts
        + [
            "CASE WHEN subreddit_prior_match "
            "THEN 'subreddit_prior' ELSE NULL::VARCHAR END"
        ]
    )

    return f"""
        SELECT
            source_type,
            source_id,
            {sql_string(domain["name"])} AS domain,
            least(1.0, {score_expression})::DOUBLE AS relevance_score,
            ({score_expression})::DOUBLE AS raw_score,
            CASE WHEN {eligible} AND subreddit_prior_match THEN {prior_weight} ELSE 0 END::DOUBLE
                AS subreddit_prior,
            greatest(
                0,
                least(
                    1.0,
                    {score_expression}
                    - CASE WHEN {eligible} THEN {prior_score} ELSE 0 END
                )
            )::DOUBLE AS signal_prior,
            matched_terms,
            matched_rule_groups AS matched_rules,
            to_json(list_filter([{reasons}], item -> item IS NOT NULL))
                AS decision_reasons
        FROM candidate_source
    """


def score(args, rules):
    try:
        import duckdb
    except ModuleNotFoundError as exc:
        raise RuntimeError(f"duckdb python package is not installed: {exc}") from exc

    input_path = Path(args.input_path)
    if not input_path.is_file():
        raise ValueError(f"candidate input does not exist: {input_path}")

    output_path = Path(args.output_path)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    temp_output_path = output_path.with_name(output_path.name + ".tmp")
    temp_output_path.unlink(missing_ok=True)

    temp_dir = Path(args.duckdb_temp_dir)
    temp_dir.mkdir(parents=True, exist_ok=True)

    con = duckdb.connect()
    try:
        con.execute(f"SET memory_limit = {sql_string(args.duckdb_memory_limit)}")
        con.execute(f"SET threads = {max(args.duckdb_threads, 1)}")
        con.execute(f"SET temp_directory = {sql_string(str(temp_dir))}")
        con.execute(
            f"""
            CREATE TEMP VIEW candidate_source AS
            SELECT
                *,
                lower(trim(coalesce(candidate_text, ''))) AS candidate_match_text
            FROM read_parquet({sql_string(str(input_path))})
            """
        )

        rows_candidates = int(
            con.execute("SELECT count(*) FROM candidate_source").fetchone()[0]
        )
        domain_queries = [build_domain_query(domain) for domain in rules["domains"]]
        union_query = "\nUNION ALL\n".join(domain_queries)
        tiers = rules["tiers"]

        con.execute(
            f"""
            CREATE TEMP TABLE scored_candidates AS
            WITH domain_scores AS (
                {union_query}
            )
            SELECT
                * EXCLUDE (raw_score),
                CASE
                    WHEN relevance_score >= {float(tiers["a"])} THEN 'A'
                    WHEN relevance_score >= {float(tiers["b"])} THEN 'B'
                    WHEN relevance_score >= {float(tiers["c"])} THEN 'C'
                    ELSE 'D'
                END AS relevance_tier,
                CASE
                    WHEN relevance_score >= {float(tiers["b"])} THEN 'retain'
                    WHEN relevance_score >= {float(tiers["c"])} THEN 'evaluate'
                    ELSE 'discard'
                END AS decision,
                {sql_string(rules["version"])} AS relevance_version,
                current_timestamp AS scored_at
            FROM domain_scores
            """
        )

        rows_scored = int(
            con.execute("SELECT count(*) FROM scored_candidates").fetchone()[0]
        )
        decision_counts = con.execute(
            """
            WITH candidate_decisions AS (
                SELECT
                    source_type,
                    source_id,
                    max(CASE decision
                        WHEN 'retain' THEN 2
                        WHEN 'evaluate' THEN 1
                        ELSE 0
                    END) AS decision_rank
                FROM scored_candidates
                GROUP BY source_type, source_id
            )
            SELECT
                count(*) FILTER (WHERE decision_rank = 2),
                count(*) FILTER (WHERE decision_rank = 1),
                count(*) FILTER (WHERE decision_rank = 0)
            FROM candidate_decisions
            """
        ).fetchone()
        tier_counts = {
            tier: count
            for tier, count in con.execute(
                """
                SELECT relevance_tier, count(*)
                FROM scored_candidates
                GROUP BY relevance_tier
                """
            ).fetchall()
        }
        for tier in ["A", "B", "C", "D"]:
            tier_counts.setdefault(tier, 0)

        con.execute(
            f"""
            COPY scored_candidates
            TO {sql_string(str(temp_output_path))}
            (FORMAT PARQUET, COMPRESSION ZSTD)
            """
        )
        bytes_written = temp_output_path.stat().st_size
        temp_output_path.replace(output_path)

        return {
            "status": "completed",
            "rows_candidates": rows_candidates,
            "rows_scored": rows_scored,
            "rows_retained_candidates": decision_counts[0],
            "rows_evaluation_candidates": decision_counts[1],
            "rows_discarded_candidates": decision_counts[2],
            "tier_counts": tier_counts,
            "output_path": str(output_path),
            "bytes_written": bytes_written,
            "relevance_version": rules["version"],
        }
    finally:
        con.close()
        if temp_output_path.exists():
            temp_output_path.unlink()


def main():
    args = parse_args()
    try:
        rules = json.loads(args.rules_json)
        print(json.dumps(score(args, rules)))
        return 0
    except Exception as exc:
        print(json.dumps({"status": "error", "error": str(exc)}))
        return 1


if __name__ == "__main__":
    sys.exit(main())
