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

        return {
            "status": "completed",
            "relevance_version": relevance_version,
            "rows_labeled": rows_labeled,
            "domains": domains,
            "candidate": candidate,
            "trap_leakage": trap_leakage,
            "minimum_retain_precision": args.minimum_retain_precision,
            "quality_gate_passed": (
                candidate["retained_predictions"] > 0
                and candidate["retained_precision"] >= args.minimum_retain_precision
            ),
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
