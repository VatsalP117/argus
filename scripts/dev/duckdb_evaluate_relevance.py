#!/usr/bin/env python3
import argparse
import json
import sys
from pathlib import Path


DOMAINS = ("travel", "saas_opportunity", "app_opportunity")


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--labels-path", required=True)
    parser.add_argument("--score-path", required=True)
    parser.add_argument("--minimum-retain-precision", type=float, required=True)
    parser.add_argument("--minimum-weighted-recall", type=float, default=0.60)
    parser.add_argument("--minimum-domain-precision", type=float, default=0.60)
    parser.add_argument("--minimum-domain-retained-count", type=int, default=10)
    parser.add_argument("--max-false-positive-category-rate", type=float, default=0.20)
    return parser.parse_args()


def sql_string(value: str):
    return "'" + value.replace("'", "''") + "'"


def metric(labeled_positive, retained_predictions, true_positive):
    false_positive = retained_predictions - true_positive
    false_negative = labeled_positive - true_positive
    precision = true_positive / retained_predictions if retained_predictions else 0.0
    recall = true_positive / labeled_positive if labeled_positive else 0.0
    f1 = 2 * precision * recall / (precision + recall) if precision + recall else 0.0
    return {
        "labeled_positive": labeled_positive,
        "retained_predictions": retained_predictions,
        "true_positive_retained": true_positive,
        "false_positive_retained": false_positive,
        "false_negative_retained": false_negative,
        "retained_precision": precision,
        "retained_recall": recall,
        "f1": f1,
    }


def evaluate(args):
    try:
        import duckdb
    except ModuleNotFoundError as exc:
        raise RuntimeError(f"duckdb python package is not installed: {exc}") from exc

    labels_path = Path(args.labels_path)
    score_path = Path(args.score_path)
    if not labels_path.is_file():
        raise ValueError(f"labels input does not exist: {labels_path}")
    if not score_path.is_file():
        raise ValueError(f"score input does not exist: {score_path}")
    if not 0 < args.minimum_retain_precision <= 1:
        raise ValueError("minimum retain precision must be within (0, 1]")
    if not 0 <= args.minimum_weighted_recall <= 1:
        raise ValueError("minimum weighted recall must be within [0, 1]")
    if not 0 < args.minimum_domain_precision <= 1:
        raise ValueError("minimum domain precision must be within (0, 1]")
    if args.minimum_domain_retained_count < 0:
        raise ValueError("minimum domain retained count must be non-negative")
    if not 0 <= args.max_false_positive_category_rate <= 1:
        raise ValueError("max false-positive category rate must be within [0, 1]")

    con = duckdb.connect()
    try:
        con.execute(
            f"""
            CREATE TEMP TABLE labels AS
            SELECT *
            FROM read_csv(
                {sql_string(str(labels_path))},
                header = true,
                all_varchar = true
            )
            """
        )
        con.execute(
            f"""
            CREATE TEMP VIEW scores AS
            SELECT * FROM read_parquet({sql_string(str(score_path))})
            """
        )
        rows_labeled = int(con.execute("SELECT count(*) FROM labels").fetchone()[0])
        if rows_labeled == 0:
            raise ValueError("label fixture is empty")
        duplicate_labels = int(
            con.execute(
                """
                SELECT count(*) - count(DISTINCT source_type || ':' || source_id)
                FROM labels
                """
            ).fetchone()[0]
        )
        if duplicate_labels:
            raise ValueError("label fixture contains duplicate source IDs")

        required_columns = {
            "source_type",
            "source_id",
            "sample_stratum",
            "stratum_population",
            "sample_rank",
            "sampling_seed",
        }
        label_columns = {
            row[0] for row in con.execute("DESCRIBE labels").fetchall()
        }
        missing = required_columns - label_columns
        if missing:
            raise ValueError(f"label fixture missing population metadata columns: {sorted(missing)}")

        for domain in DOMAINS:
            column = f"label_{domain}"
            invalid = int(
                con.execute(
                    f"""
                    SELECT count(*)
                    FROM labels
                    WHERE {column} IS NULL OR {column} NOT IN ('0', '1')
                    """
                ).fetchone()[0]
            )
            if invalid:
                raise ValueError(f"label column {column} has {invalid} invalid values")

        # Derive each candidate's stratum and all stratum populations from scores.
        con.execute(
            """
            CREATE TEMP VIEW derived_strata AS
            WITH ranked_domains AS (
                SELECT
                    *,
                    row_number() OVER (
                        PARTITION BY source_type, source_id
                        ORDER BY relevance_score DESC, domain
                    ) AS domain_rank
                FROM scores
            ),
            score_summary AS (
                SELECT
                    source_type,
                    source_id,
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
            )
            SELECT * FROM score_summary
            """
        )
        con.execute(
            """
            CREATE TEMP VIEW derived_populations AS
            SELECT sample_stratum, count(*) AS population
            FROM derived_strata
            GROUP BY sample_stratum
            """
        )

        # Every label must correspond to a scored candidate.
        unknown_label_ids = int(
            con.execute(
                """
                SELECT count(*)
                FROM labels l
                LEFT JOIN derived_strata d USING (source_type, source_id)
                WHERE d.source_id IS NULL
                """
            ).fetchone()[0]
        )
        if unknown_label_ids:
            raise ValueError("label fixture contains source IDs not present in score input")

        # Every label's claimed stratum must match the stratum derived from scores.
        stratum_mismatch = int(
            con.execute(
                """
                SELECT count(*)
                FROM labels l
                JOIN derived_strata d USING (source_type, source_id)
                WHERE l.sample_stratum <> d.sample_stratum
                """
            ).fetchone()[0]
        )
        if stratum_mismatch:
            raise ValueError(f"{stratum_mismatch} label rows have a sample_stratum that does not match the score-derived stratum")

        # stratum_population must be numeric, non-null, and match the derived population row-by-row.
        non_numeric_population = int(
            con.execute(
                """
                SELECT count(*)
                FROM labels
                WHERE stratum_population IS NULL
                   OR try_cast(stratum_population AS BIGINT) IS NULL
                """
            ).fetchone()[0]
        )
        if non_numeric_population:
            raise ValueError("stratum_population contains null or non-numeric values")

        population_mismatch = int(
            con.execute(
                """
                SELECT count(*)
                FROM labels l
                JOIN derived_populations d ON l.sample_stratum = d.sample_stratum
                WHERE cast(l.stratum_population AS BIGINT) <> d.population
                """
            ).fetchone()[0]
        )
        if population_mismatch:
            raise ValueError(
                f"{population_mismatch} label rows have a stratum_population that does not match the score-derived population"
            )

        # Require every stratum that has a non-zero derived population to be represented in labels.
        missing_strata = [
            stratum
            for stratum, in con.execute(
                """
                SELECT d.sample_stratum
                FROM derived_populations d
                LEFT JOIN (
                    SELECT DISTINCT sample_stratum FROM labels
                ) l ON d.sample_stratum = l.sample_stratum
                WHERE d.population > 0 AND l.sample_stratum IS NULL
                ORDER BY d.sample_stratum
                """
            ).fetchall()
        ]
        if missing_strata:
            raise ValueError(f"label fixture is missing strata with non-zero populations: {missing_strata}")

        # The retained label IDs must exactly match the score-derived retained IDs.
        score_retained_count = int(
            con.execute(
                """
                SELECT count(*) FROM derived_strata WHERE sample_stratum = 'retain'
                """
            ).fetchone()[0]
        )
        label_retained_count = int(
            con.execute(
                """
                SELECT count(*) FROM labels WHERE sample_stratum = 'retain'
                """
            ).fetchone()[0]
        )
        if score_retained_count != label_retained_count:
            raise ValueError(
                f"retained label count ({label_retained_count}) does not match score-derived retained count ({score_retained_count})"
            )

        unlabeled_retained_scores = int(
            con.execute(
                """
                SELECT count(*)
                FROM derived_strata d
                LEFT JOIN labels l USING (source_type, source_id)
                WHERE d.sample_stratum = 'retain' AND l.source_id IS NULL
                """
            ).fetchone()[0]
        )
        if unlabeled_retained_scores:
            raise ValueError(
                f"{unlabeled_retained_scores} score-derived retained candidates are missing from the label fixture"
            )

        extra_retained_labels = int(
            con.execute(
                """
                SELECT count(*)
                FROM labels l
                LEFT JOIN derived_strata d USING (source_type, source_id)
                WHERE l.sample_stratum = 'retain' AND d.source_id IS NULL
                """
            ).fetchone()[0]
        )
        if extra_retained_labels:
            raise ValueError(
                f"{extra_retained_labels} retained labels are not score-derived retained candidates"
            )

        invalid_score_cardinality = int(
            con.execute(
                """
                WITH expected AS (
                    SELECT l.source_type, l.source_id, d.domain
                    FROM labels l
                    CROSS JOIN (
                        SELECT 'travel' AS domain
                        UNION ALL SELECT 'saas_opportunity'
                        UNION ALL SELECT 'app_opportunity'
                    ) d
                ),
                observed AS (
                    SELECT source_type, source_id, domain, count(*) AS n
                    FROM scores
                    WHERE domain IN ('travel', 'saas_opportunity', 'app_opportunity')
                    GROUP BY source_type, source_id, domain
                )
                SELECT count(*)
                FROM expected e
                LEFT JOIN observed o
                  ON o.source_type = e.source_type
                 AND o.source_id = e.source_id
                 AND o.domain = e.domain
                WHERE coalesce(o.n, 0) <> 1
                """
            ).fetchone()[0]
        )
        if invalid_score_cardinality != 0:
            raise ValueError(
                "score input does not contain exactly one row per labelled candidate and domain"
            )

        score_columns = {
            row[0] for row in con.execute("DESCRIBE scores").fetchall()
        }
        relevance_version = "unknown"
        if "relevance_version" in score_columns:
            versions = [
                row[0]
                for row in con.execute(
                    "SELECT DISTINCT relevance_version FROM scores ORDER BY relevance_version"
                ).fetchall()
            ]
            relevance_version = ",".join(versions)

        domains = {}
        for domain in DOMAINS:
            label_column = f"label_{domain}"
            counts = con.execute(
                f"""
                SELECT
                    count(*) FILTER (WHERE l.{label_column} = '1'),
                    count(*) FILTER (WHERE s.decision = 'retain'),
                    count(*) FILTER (
                        WHERE l.{label_column} = '1' AND s.decision = 'retain'
                    )
                FROM labels l
                JOIN scores s USING (source_type, source_id)
                WHERE s.domain = {sql_string(domain)}
                """
            ).fetchone()
            domains[domain] = metric(*map(int, counts))

        candidate_counts = con.execute(
            """
            WITH candidate_scores AS (
                SELECT
                    source_type,
                    source_id,
                    bool_or(decision = 'retain') AS retained
                FROM scores
                GROUP BY source_type, source_id
            ),
            candidate_labels AS (
                SELECT
                    *,
                    label_travel = '1'
                    OR label_saas_opportunity = '1'
                    OR label_app_opportunity = '1' AS relevant
                FROM labels
            )
            SELECT
                count(*) FILTER (WHERE relevant),
                count(*) FILTER (WHERE retained),
                count(*) FILTER (WHERE relevant AND retained)
            FROM candidate_labels
            JOIN candidate_scores USING (source_type, source_id)
            """
        ).fetchone()
        candidate = metric(*map(int, candidate_counts))
        candidate["exact_retained_precision"] = candidate["retained_precision"]
        candidate["fixture_recall"] = candidate["retained_recall"]

        stratum_stats = con.execute(
            """
            SELECT
                d.sample_stratum,
                d.population,
                count(l.source_id) AS sample_size,
                count(l.source_id) FILTER (
                    WHERE l.label_travel = '1'
                       OR l.label_saas_opportunity = '1'
                       OR l.label_app_opportunity = '1'
                ) AS positive_count
            FROM derived_populations d
            LEFT JOIN labels l ON d.sample_stratum = l.sample_stratum
            GROUP BY d.sample_stratum, d.population
            ORDER BY d.sample_stratum
            """
        ).fetchall()
        strata = {}
        estimated_total_relevant = 0.0
        for stratum, population, sample_size, positive_count in stratum_stats:
            population = int(population or 0)
            sample_size = int(sample_size or 0)
            positive_count = int(positive_count or 0)
            rate = positive_count / sample_size if sample_size else 0.0
            strata[stratum] = {
                "population": population,
                "sample_size": sample_size,
                "positive_count": positive_count,
                "sampled_positive_rate": rate,
            }
            estimated_total_relevant += population * rate

        candidate["estimated_total_relevant"] = estimated_total_relevant
        candidate["weighted_retained_recall_estimate"] = (
            candidate["true_positive_retained"] / estimated_total_relevant
            if estimated_total_relevant > 0 else 0.0
        )

        trap_leakage = {
            category: int(count)
            for category, count in con.execute(
                """
                WITH candidate_scores AS (
                    SELECT
                        source_type,
                        source_id,
                        bool_or(decision = 'retain') AS retained
                    FROM scores
                    GROUP BY source_type, source_id
                )
                SELECT false_positive_category, count(*)
                FROM labels
                JOIN candidate_scores USING (source_type, source_id)
                WHERE retained
                  AND coalesce(false_positive_category, '') <> ''
                GROUP BY false_positive_category
                ORDER BY false_positive_category
                """
            ).fetchall()
        }

        missing_source_url_count = int(
            con.execute(
                """
                WITH candidate_scores AS (
                    SELECT
                        source_type,
                        source_id,
                        bool_or(decision = 'retain') AS retained
                    FROM scores
                    GROUP BY source_type, source_id
                )
                SELECT count(*)
                FROM labels
                JOIN candidate_scores USING (source_type, source_id)
                WHERE retained
                  AND coalesce(source_url, '') = ''
                """
            ).fetchone()[0]
        )

        retained_count = int(candidate["retained_predictions"])
        category_rate_violations = {
            category: count
            for category, count in trap_leakage.items()
            if retained_count > 0 and count / retained_count > args.max_false_positive_category_rate
        }

        domain_precision_failures = {
            domain: metrics
            for domain, metrics in domains.items()
            if metrics["retained_predictions"] >= args.minimum_domain_retained_count
            and metrics["retained_precision"] < args.minimum_domain_precision
        }

        visa_retained_false_positives = int(
            con.execute(
                """
                WITH candidate_scores AS (
                    SELECT
                        source_type,
                        source_id,
                        bool_or(decision = 'retain') AS retained
                    FROM scores
                    GROUP BY source_type, source_id
                )
                SELECT count(*)
                FROM labels
                JOIN candidate_scores USING (source_type, source_id)
                WHERE retained
                  AND false_positive_category = 'payment_brand_visa'
                """
            ).fetchone()[0]
        )

        promotion_bot_retained_false_positives = int(
            con.execute(
                """
                WITH candidate_scores AS (
                    SELECT
                        source_type,
                        source_id,
                        bool_or(decision = 'retain') AS retained
                    FROM scores
                    GROUP BY source_type, source_id
                )
                SELECT count(*)
                FROM labels
                JOIN candidate_scores USING (source_type, source_id)
                WHERE retained
                  AND false_positive_category = 'promotion_or_bot'
                """
            ).fetchone()[0]
        )

        gate_passed = (
            retained_count > 0
            and candidate["exact_retained_precision"] >= args.minimum_retain_precision
            and candidate["weighted_retained_recall_estimate"] >= args.minimum_weighted_recall
            and not domain_precision_failures
            and missing_source_url_count == 0
            and visa_retained_false_positives == 0
            and promotion_bot_retained_false_positives == 0
            and not category_rate_violations
        )

        return {
            "status": "completed",
            "relevance_version": relevance_version,
            "rows_labeled": rows_labeled,
            "domains": domains,
            "candidate": candidate,
            "strata": strata,
            "trap_leakage": trap_leakage,
            "missing_source_url_count": missing_source_url_count,
            "visa_retained_false_positives": visa_retained_false_positives,
            "promotion_bot_retained_false_positives": promotion_bot_retained_false_positives,
            "false_positive_category_rate_violations": category_rate_violations,
            "domain_precision_failures": domain_precision_failures,
            "minimum_retain_precision": args.minimum_retain_precision,
            "minimum_weighted_recall": args.minimum_weighted_recall,
            "quality_gate_passed": gate_passed,
        }
    finally:
        con.close()


def main():
    args = parse_args()
    try:
        print(json.dumps(evaluate(args)))
        return 0
    except Exception as exc:
        print(json.dumps({"status": "error", "error": str(exc)}))
        return 1


if __name__ == "__main__":
    sys.exit(main())
