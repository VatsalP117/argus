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

        joined_score_rows = int(
            con.execute(
                """
                SELECT count(*)
                FROM labels l
                JOIN scores s USING (source_type, source_id)
                WHERE s.domain IN ('travel', 'saas_opportunity', 'app_opportunity')
                """
            ).fetchone()[0]
        )
        if joined_score_rows != rows_labeled * len(DOMAINS):
            raise ValueError(
                "score input does not contain exactly one row per labelled candidate and domain"
            )

        inconsistent_population = int(
            con.execute(
                """
                SELECT count(*)
                FROM (
                    SELECT sample_stratum, count(DISTINCT stratum_population) AS distinct_populations
                    FROM labels
                    GROUP BY sample_stratum
                )
                WHERE distinct_populations > 1
                """
            ).fetchone()[0]
        )
        if inconsistent_population:
            raise ValueError("stratum_population is inconsistent within a stratum")

        retain_population, retain_sample_size = con.execute(
            """
            SELECT
                max(stratum_population),
                count(*)
            FROM labels
            WHERE sample_stratum = 'retain'
            """
        ).fetchone()
        retain_population = int(retain_population or 0)
        retain_sample_size = int(retain_sample_size or 0)
        if retain_sample_size != retain_population:
            raise ValueError(
                f"retain fixture must include all retained rows: sample={retain_sample_size}, population={retain_population}"
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
                sample_stratum,
                max(stratum_population) AS population,
                count(*) AS sample_size,
                count(*) FILTER (
                    WHERE label_travel = '1'
                       OR label_saas_opportunity = '1'
                       OR label_app_opportunity = '1'
                ) AS positive_count
            FROM labels
            GROUP BY sample_stratum
            ORDER BY sample_stratum
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
