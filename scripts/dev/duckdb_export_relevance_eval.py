#!/usr/bin/env python3
import argparse
import json
import sys
from pathlib import Path


STRATA = ("retain", "evaluate", "discard")


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--candidate-path", required=True)
    parser.add_argument("--score-path", required=True)
    parser.add_argument("--output-path", required=True)
    parser.add_argument("--sample-per-stratum", type=int, required=True)
    parser.add_argument("--retain-sample", type=int, required=True)
    parser.add_argument("--evaluate-sample", type=int, required=True)
    parser.add_argument("--discard-sample", type=int, required=True)
    parser.add_argument("--seed", required=True)
    return parser.parse_args()


def sql_string(value: str):
    return "'" + value.replace("'", "''") + "'"


def sample_limit(args, stratum: str) -> int:
    value = getattr(args, stratum.replace("-", "_") + "_sample")
    if value < 0:
        return args.sample_per_stratum
    return value


def export_evaluation(args):
    try:
        import duckdb
    except ModuleNotFoundError as exc:
        raise RuntimeError(f"duckdb python package is not installed: {exc}") from exc

    candidate_path = Path(args.candidate_path)
    score_path = Path(args.score_path)
    if not candidate_path.is_file():
        raise ValueError(f"candidate input does not exist: {candidate_path}")
    if not score_path.is_file():
        raise ValueError(f"score input does not exist: {score_path}")
    if args.sample_per_stratum <= 0:
        raise ValueError("sample per stratum must be positive")

    for stratum in STRATA:
        limit = sample_limit(args, stratum)
        if limit < 0:
            raise ValueError(f"{stratum} sample must be non-negative")

    output_path = Path(args.output_path)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    temp_output_path = output_path.with_name(output_path.name + ".tmp")
    temp_output_path.unlink(missing_ok=True)

    limits = {stratum: sample_limit(args, stratum) for stratum in STRATA}
    seed_literal = sql_string(args.seed)

    con = duckdb.connect()
    try:
        con.execute(
            f"""
            CREATE TEMP VIEW candidate_source AS
            SELECT * FROM read_parquet({sql_string(str(candidate_path))})
            """
        )
        con.execute(
            f"""
            CREATE TEMP VIEW score_source AS
            SELECT * FROM read_parquet({sql_string(str(score_path))})
            """
        )
        con.execute(
            f"""
            CREATE TEMP TABLE evaluation_sample AS
            WITH ranked_domains AS (
                SELECT
                    *,
                    row_number() OVER (
                        PARTITION BY source_type, source_id
                        ORDER BY relevance_score DESC, domain
                    ) AS domain_rank
                FROM score_source
            ),
            score_summary AS (
                SELECT
                    source_type,
                    source_id,
                    max(relevance_score) FILTER (WHERE domain = 'travel')
                        AS v1_travel_score,
                    max(relevance_score) FILTER (WHERE domain = 'saas_opportunity')
                        AS v1_saas_opportunity_score,
                    max(relevance_score) FILTER (WHERE domain = 'app_opportunity')
                        AS v1_app_opportunity_score,
                    max(domain) FILTER (WHERE domain_rank = 1) AS predicted_domain,
                    max(relevance_score) FILTER (WHERE domain_rank = 1) AS predicted_score,
                    max(relevance_tier) FILTER (WHERE domain_rank = 1) AS predicted_tier,
                    max(decision) FILTER (WHERE domain_rank = 1) AS predicted_decision,
                    CASE max(CASE decision
                        WHEN 'retain' THEN 2
                        WHEN 'evaluate' THEN 1
                        ELSE 0
                    END)
                        WHEN 2 THEN 'retain'
                        WHEN 1 THEN 'evaluate'
                        ELSE 'discard'
                    END AS sample_stratum
                FROM ranked_domains
                GROUP BY source_type, source_id
            ),
            eligible AS (
                SELECT
                    c.source_type,
                    c.source_id,
                    c.subreddit,
                    left(c.original_text, 1000) AS text_excerpt,
                    c.source_url,
                    c.matched_terms,
                    c.matched_rule_groups,
                    s.v1_travel_score,
                    s.v1_saas_opportunity_score,
                    s.v1_app_opportunity_score,
                    s.predicted_domain,
                    s.predicted_score,
                    s.predicted_tier,
                    s.predicted_decision,
                    s.sample_stratum,
                    count(*) OVER (PARTITION BY s.sample_stratum) AS stratum_population,
                    row_number() OVER (
                        PARTITION BY s.sample_stratum
                        ORDER BY md5(
                            c.source_type || ':' || c.source_id || ':' ||
                            {seed_literal}
                        )
                    ) AS sample_rank
                FROM candidate_source c
                JOIN score_summary s USING (source_type, source_id)
            )
            SELECT
                source_type,
                source_id,
                subreddit,
                text_excerpt,
                source_url,
                matched_terms,
                matched_rule_groups,
                v1_travel_score,
                v1_saas_opportunity_score,
                v1_app_opportunity_score,
                predicted_domain,
                predicted_score,
                predicted_tier,
                predicted_decision,
                sample_stratum,
                stratum_population,
                sample_rank,
                {seed_literal} AS sampling_seed,
                NULL::VARCHAR AS label_travel,
                NULL::VARCHAR AS label_saas_opportunity,
                NULL::VARCHAR AS label_app_opportunity,
                NULL::VARCHAR AS false_positive_category,
                NULL::VARCHAR AS label_notes
            FROM eligible
            WHERE sample_rank <= CASE sample_stratum
                WHEN 'retain' THEN {limits['retain'] if limits['retain'] > 0 else 'stratum_population'}
                WHEN 'evaluate' THEN {limits['evaluate'] if limits['evaluate'] > 0 else 'stratum_population'}
                ELSE {limits['discard'] if limits['discard'] > 0 else 'stratum_population'}
            END
            ORDER BY
                CASE sample_stratum
                    WHEN 'retain' THEN 1
                    WHEN 'evaluate' THEN 2
                    ELSE 3
                END,
                source_type,
                source_id
            """
        )

        con.execute(
            f"""
            COPY evaluation_sample
            TO {sql_string(str(temp_output_path))}
            (FORMAT CSV, HEADER, NULL '')
            """
        )
        temp_output_path.replace(output_path)

        stratum_counts = {
            stratum: count
            for stratum, count in con.execute(
                """
                SELECT sample_stratum, count(*)
                FROM evaluation_sample
                GROUP BY sample_stratum
                """
            ).fetchall()
        }
        stratum_populations = {
            stratum: population
            for stratum, population in con.execute(
                """
                SELECT sample_stratum, max(stratum_population)
                FROM evaluation_sample
                GROUP BY sample_stratum
                """
            ).fetchall()
        }
        for stratum in STRATA:
            stratum_counts.setdefault(stratum, 0)
            stratum_populations.setdefault(stratum, 0)
        return {
            "status": "completed",
            "rows_exported": sum(stratum_counts.values()),
            "stratum_counts": stratum_counts,
            "stratum_populations": stratum_populations,
            "sampling_seed": args.seed,
            "output_path": str(output_path),
        }
    finally:
        con.close()
        temp_output_path.unlink(missing_ok=True)


def main():
    args = parse_args()
    try:
        print(json.dumps(export_evaluation(args)))
        return 0
    except Exception as exc:
        print(json.dumps({"status": "error", "error": str(exc)}))
        return 1


if __name__ == "__main__":
    sys.exit(main())
