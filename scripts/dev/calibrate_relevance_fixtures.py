#!/usr/bin/env python3
import argparse
import csv
import json
import shutil
import subprocess
import sys
from pathlib import Path


APPROVED_FIXTURES = {
    "comments-2021-01-000",
    "comments-2021-01-001",
}
FROZEN_FIXTURES = {
    "comments-2021-01-002",
    "comments-2021-01-003",
}
DOMAINS = ("travel", "saas_opportunity", "app_opportunity")
DEFAULT_THRESHOLDS = {
    "minimum_retain_precision": 0.75,
    "minimum_weighted_recall": 0.50,
    "minimum_domain_precision": 0.65,
    "minimum_domain_retained_count": 10,
    "max_false_positive_category_rate": 0.20,
}


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--relevance-config", required=True)
    parser.add_argument("--output-dir", required=True)
    parser.add_argument(
        "--fixture",
        action="append",
        default=[],
        help="fixture_id|labels_csv|annotations_json for approved observed fixtures only",
    )
    parser.add_argument(
        "--minimum-retain-precision",
        type=float,
        default=DEFAULT_THRESHOLDS["minimum_retain_precision"],
    )
    parser.add_argument(
        "--minimum-weighted-recall",
        type=float,
        default=DEFAULT_THRESHOLDS["minimum_weighted_recall"],
    )
    parser.add_argument(
        "--minimum-domain-precision",
        type=float,
        default=DEFAULT_THRESHOLDS["minimum_domain_precision"],
    )
    parser.add_argument(
        "--minimum-domain-retained-count",
        type=int,
        default=DEFAULT_THRESHOLDS["minimum_domain_retained_count"],
    )
    parser.add_argument(
        "--max-false-positive-category-rate",
        type=float,
        default=DEFAULT_THRESHOLDS["max_false_positive_category_rate"],
    )
    return parser.parse_args()


def sql_string(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def run_json(cmd):
    completed = subprocess.run(cmd, capture_output=True, text=True, check=False)
    if completed.returncode != 0:
        raise RuntimeError(
            f"command failed ({completed.returncode}): {' '.join(cmd)}\n"
            f"stdout:\n{completed.stdout}\n"
            f"stderr:\n{completed.stderr}"
        )
    try:
        return json.loads(completed.stdout)
    except json.JSONDecodeError as exc:
        raise RuntimeError(
            f"command did not return JSON: {' '.join(cmd)}\n"
            f"stdout:\n{completed.stdout}\n"
            f"stderr:\n{completed.stderr}"
        ) from exc


def load_fixture_specs(values):
    specs = {}
    if not values:
        raise ValueError("at least one --fixture value is required")
    for raw in values:
        parts = raw.split("|")
        if len(parts) != 3:
            raise ValueError(
                f"fixture value must be fixture_id|labels_csv|annotations_json, got: {raw}"
            )
        fixture_id, labels_path, annotations_path = [part.strip() for part in parts]
        if fixture_id in FROZEN_FIXTURES:
            raise ValueError(f"fixture {fixture_id} is frozen validation input and cannot be used")
        if fixture_id not in APPROVED_FIXTURES:
            raise ValueError(
                f"fixture {fixture_id} is not an approved observed training fixture"
            )
        if fixture_id in specs:
            raise ValueError(f"fixture {fixture_id} was provided more than once")
        labels = Path(labels_path)
        annotations = Path(annotations_path)
        if not labels.is_file():
            raise ValueError(f"labels input does not exist: {labels}")
        if not annotations.is_file():
            raise ValueError(f"annotations input does not exist: {annotations}")
        specs[fixture_id] = {
            "fixture_id": fixture_id,
            "labels_path": labels.resolve(),
            "annotations_path": annotations.resolve(),
        }
    if set(specs) != APPROVED_FIXTURES:
        raise ValueError(
            "the calibration runner requires exactly the approved observed fixtures: "
            f"{sorted(APPROVED_FIXTURES)}"
        )
    return [specs[fixture_id] for fixture_id in sorted(specs)]


def has_population_metadata(labels_path: Path) -> bool:
    with labels_path.open(newline="") as file:
        reader = csv.reader(file)
        header = next(reader, [])
    required = {"sample_stratum", "stratum_population", "sample_rank", "sampling_seed"}
    return required.issubset(set(header))


def reconstruct_candidates(labels_path: Path, output_path: Path):
    try:
        import duckdb
    except ModuleNotFoundError as exc:
        raise RuntimeError(f"duckdb python package is not installed: {exc}") from exc

    output_path.parent.mkdir(parents=True, exist_ok=True)
    temp_output_path = output_path.with_name(output_path.name + ".tmp")
    temp_output_path.unlink(missing_ok=True)

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
        rows = int(con.execute("SELECT count(*) FROM labels").fetchone()[0])
        con.execute(
            f"""
            COPY (
                SELECT
                    source_type,
                    source_id,
                    subreddit,
                    text_excerpt AS candidate_text,
                    text_excerpt AS original_text,
                    source_url,
                    matched_terms,
                    matched_rule_groups,
                    false AS subreddit_prior_match,
                    false AS is_bot_like
                FROM labels
                ORDER BY source_type, source_id
            )
            TO {sql_string(str(temp_output_path))}
            (FORMAT PARQUET, COMPRESSION ZSTD)
            """
        )
    finally:
        con.close()

    bytes_written = temp_output_path.stat().st_size
    temp_output_path.replace(output_path)
    return {
        "status": "completed",
        "rows_reconstructed": rows,
        "output_path": str(output_path),
        "bytes_written": bytes_written,
    }


def write_historical_labels(labels_path: Path, output_path: Path):
    with labels_path.open(newline="") as file:
        rows = list(csv.DictReader(file))
        fieldnames = list(rows[0].keys()) if rows else []
    if not fieldnames:
        raise ValueError(f"labels file has no header: {labels_path}")
    filtered = [name for name in fieldnames if name not in {
        "sample_stratum",
        "stratum_population",
        "sample_rank",
        "sampling_seed",
    }]
    output_path.parent.mkdir(parents=True, exist_ok=True)
    with output_path.open("w", newline="") as file:
        writer = csv.DictWriter(file, fieldnames=filtered, lineterminator="\n")
        writer.writeheader()
        for row in rows:
            writer.writerow({name: row.get(name, "") for name in filtered})
    return output_path


def evaluate_historical(labels_path: Path, score_path: Path):
    try:
        import duckdb
    except ModuleNotFoundError as exc:
        raise RuntimeError(f"duckdb python package is not installed: {exc}") from exc

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
        score_columns = {row[0] for row in con.execute("DESCRIBE scores").fetchall()}
        relevance_version = "unknown"
        if "relevance_version" in score_columns:
            versions = [
                row[0]
                for row in con.execute(
                    "SELECT DISTINCT relevance_version FROM scores ORDER BY relevance_version"
                ).fetchall()
            ]
            relevance_version = ",".join(versions)

        con.execute(
            """
            CREATE TEMP VIEW candidate_predictions AS
            WITH ranked AS (
                SELECT
                    source_type,
                    source_id,
                    domain,
                    decision,
                    relevance_score,
                    row_number() OVER (
                        PARTITION BY source_type, source_id
                        ORDER BY relevance_score DESC, domain
                    ) AS domain_rank
                FROM scores
            )
            SELECT
                source_type,
                source_id,
                max(CASE decision
                    WHEN 'retain' THEN 2
                    WHEN 'evaluate' THEN 1
                    ELSE 0
                END) AS decision_rank,
                max(domain) FILTER (WHERE domain_rank = 1) AS predicted_domain
            FROM ranked
            GROUP BY source_type, source_id
            """
        )

        def metrics_for(domain=None):
            label_positive_expr = (
                "label_travel = '1' OR label_saas_opportunity = '1' OR label_app_opportunity = '1'"
                if domain is None
                else f"label_{domain} = '1'"
            )
            retained_expr = "p.decision_rank = 2"
            if domain is None:
                true_positive_expr = (
                    "p.decision_rank = 2 AND "
                    "(l.label_travel = '1' OR l.label_saas_opportunity = '1' OR l.label_app_opportunity = '1')"
                )
            else:
                true_positive_expr = (
                    f"p.decision_rank = 2 AND p.predicted_domain = '{domain}' AND l.label_{domain} = '1'"
                )
                retained_expr = f"p.decision_rank = 2 AND p.predicted_domain = '{domain}'"
            labeled_positive, retained_predictions, true_positive = con.execute(
                f"""
                SELECT
                    count(*) FILTER (WHERE {label_positive_expr}) AS labeled_positive,
                    count(*) FILTER (WHERE {retained_expr}) AS retained_predictions,
                    count(*) FILTER (WHERE {true_positive_expr}) AS true_positive
                FROM labels l
                JOIN candidate_predictions p USING (source_type, source_id)
                """
            ).fetchone()
            false_positive = retained_predictions - true_positive
            false_negative = labeled_positive - true_positive
            precision = true_positive / retained_predictions if retained_predictions else 0.0
            recall = true_positive / labeled_positive if labeled_positive else 0.0
            f1 = 2 * precision * recall / (precision + recall) if precision + recall else 0.0
            result = {
                "labeled_positive": int(labeled_positive),
                "retained_predictions": int(retained_predictions),
                "true_positive_retained": int(true_positive),
                "false_positive_retained": int(false_positive),
                "false_negative_retained": int(false_negative),
                "retained_precision": precision,
                "retained_recall": recall,
                "f1": f1,
            }
            if domain is None:
                result["exact_retained_precision"] = precision
                result["fixture_recall"] = recall
            return result

        trap_leakage = {
            category: count
            for category, count in con.execute(
                """
                SELECT false_positive_category, count(*)
                FROM labels l
                JOIN candidate_predictions p USING (source_type, source_id)
                WHERE p.decision_rank = 2
                  AND label_travel <> '1'
                  AND label_saas_opportunity <> '1'
                  AND label_app_opportunity <> '1'
                  AND coalesce(false_positive_category, '') <> ''
                GROUP BY false_positive_category
                ORDER BY false_positive_category
                """
            ).fetchall()
        }
        missing_source_url_count = int(
            con.execute(
                """
                SELECT count(*)
                FROM labels l
                JOIN candidate_predictions p USING (source_type, source_id)
                WHERE p.decision_rank = 2
                  AND coalesce(source_url, '') = ''
                """
            ).fetchone()[0]
        )
        visa_retained_false_positives = int(
            con.execute(
                """
                SELECT count(*)
                FROM labels l
                JOIN candidate_predictions p USING (source_type, source_id)
                WHERE p.decision_rank = 2
                  AND label_travel <> '1'
                  AND label_saas_opportunity <> '1'
                  AND label_app_opportunity <> '1'
                  AND false_positive_category = 'payment_brand_visa'
                """
            ).fetchone()[0]
        )
        promotion_bot_retained_false_positives = int(
            con.execute(
                """
                SELECT count(*)
                FROM labels l
                JOIN candidate_predictions p USING (source_type, source_id)
                WHERE p.decision_rank = 2
                  AND label_travel <> '1'
                  AND label_saas_opportunity <> '1'
                  AND label_app_opportunity <> '1'
                  AND false_positive_category = 'promotion_or_bot'
                """
            ).fetchone()[0]
        )

        candidate_metrics = metrics_for()
        strata = {}
        if has_population_metadata(labels_path):
            for row in con.execute(
                """
                SELECT
                    sample_stratum,
                    max(cast(stratum_population AS BIGINT)) AS population,
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
            ).fetchall():
                stratum, population, sample_size, positive_count = row
                sampled_positive_rate = positive_count / sample_size if sample_size else 0.0
                strata[stratum] = {
                    "population": int(population),
                    "sample_size": int(sample_size),
                    "positive_count": int(positive_count),
                    "sampled_positive_rate": sampled_positive_rate,
                }
            estimated_total_relevant = sum(
                metrics["population"] * metrics["sampled_positive_rate"]
                for metrics in strata.values()
            )
            candidate_metrics["estimated_total_relevant"] = estimated_total_relevant
            candidate_metrics["weighted_retained_recall_estimate"] = (
                candidate_metrics["true_positive_retained"] / estimated_total_relevant
                if estimated_total_relevant
                else 0.0
            )

        return {
            "status": "completed",
            "relevance_version": relevance_version,
            "rows_labeled": int(con.execute("SELECT count(*) FROM labels").fetchone()[0]),
            "domains": {domain: metrics_for(domain) for domain in DOMAINS},
            "candidate": candidate_metrics,
            "strata": strata,
            "trap_leakage": trap_leakage,
            "missing_source_url_count": missing_source_url_count,
            "visa_retained_false_positives": visa_retained_false_positives,
            "promotion_bot_retained_false_positives": promotion_bot_retained_false_positives,
        }
    finally:
        con.close()


def calibrate_fixture(spec, args, output_dir: Path):
    fixture_dir = output_dir / spec["fixture_id"]
    fixture_dir.mkdir(parents=True, exist_ok=True)

    candidate_path = fixture_dir / "reconstructed-candidates.parquet"
    score_path = fixture_dir / "scores.parquet"
    labels_exported_path = fixture_dir / "labels-exported.csv"
    labels_annotated_path = fixture_dir / "labels-annotated.csv"
    historical_labels_path = fixture_dir / "historical-labels.csv"

    reconstruct_result = reconstruct_candidates(spec["labels_path"], candidate_path)
    score_result = run_json(
        [
            "python3",
            "scripts/dev/duckdb_score_candidates.py",
            "--input-path",
            str(candidate_path),
            "--output-path",
            str(score_path),
            "--rules-json",
            json.dumps(
                load_relevance_config(Path(args.relevance_config)),
                separators=(",", ":"),
            ),
            "--duckdb-memory-limit",
            "4GB",
            "--duckdb-threads",
            "4",
            "--duckdb-temp-dir",
            str(output_dir / "duckdb-tmp"),
        ]
    )
    export_result = run_json(
        [
            "python3",
            "scripts/dev/duckdb_export_relevance_eval.py",
            "--candidate-path",
            str(candidate_path),
            "--score-path",
            str(score_path),
            "--output-path",
            str(labels_exported_path),
            "--sample-per-stratum",
            "100",
            "--retain-sample",
            "0",
            "--evaluate-sample",
            "0",
            "--discard-sample",
            "0",
            "--seed",
            f"calibrate-{spec['fixture_id']}-{Path(args.relevance_config).stem}",
        ]
    )
    apply_result = run_json(
        [
            "python3",
            "scripts/dev/apply_relevance_annotations.py",
            "--input-path",
            str(labels_exported_path),
            "--annotations-path",
            str(spec["annotations_path"]),
            "--output-path",
            str(labels_annotated_path),
        ]
    )
    standard_evaluate = run_json(
        [
            "python3",
            "scripts/dev/duckdb_evaluate_relevance.py",
            "--labels-path",
            str(labels_annotated_path),
            "--score-path",
            str(score_path),
            "--minimum-retain-precision",
            str(args.minimum_retain_precision),
            "--minimum-weighted-recall",
            str(args.minimum_weighted_recall),
            "--minimum-domain-precision",
            str(args.minimum_domain_precision),
            "--minimum-domain-retained-count",
            str(args.minimum_domain_retained_count),
            "--max-false-positive-category-rate",
            str(args.max_false_positive_category_rate),
        ]
    )
    write_historical_labels(spec["labels_path"], historical_labels_path)
    historical_evaluate = evaluate_historical(historical_labels_path, score_path)

    return {
        "fixture": spec["fixture_id"],
        "has_population_metadata": has_population_metadata(spec["labels_path"]),
        "reconstruct": reconstruct_result,
        "score": score_result,
        "export": export_result,
        "apply": apply_result,
        "standard_evaluate": standard_evaluate,
        "historical_evaluate": historical_evaluate,
        "paths": {
            "candidate_path": str(candidate_path),
            "score_path": str(score_path),
            "regenerated_labels_path": str(labels_annotated_path),
            "source_labels_path": str(spec["labels_path"]),
            "annotations_path": str(spec["annotations_path"]),
        },
    }


def load_relevance_config(path: Path):
    try:
        import yaml
    except ModuleNotFoundError as exc:
        raise RuntimeError(f"pyyaml is not installed: {exc}") from exc
    with path.open() as file:
        return yaml.safe_load(file)


def main():
    args = parse_args()
    try:
        fixture_specs = load_fixture_specs(args.fixture)
        output_dir = Path(args.output_dir)
        if output_dir.exists():
            shutil.rmtree(output_dir)
        output_dir.mkdir(parents=True, exist_ok=True)

        fixtures = [calibrate_fixture(spec, args, output_dir) for spec in fixture_specs]
        report = {
            "status": "completed",
            "relevance_config": args.relevance_config,
            "relevance_version": load_relevance_config(Path(args.relevance_config))["version"],
            "output_dir": str(output_dir),
            "thresholds": {
                "minimum_retain_precision": args.minimum_retain_precision,
                "minimum_weighted_recall": args.minimum_weighted_recall,
                "minimum_domain_precision": args.minimum_domain_precision,
                "minimum_domain_retained_count": args.minimum_domain_retained_count,
                "max_false_positive_category_rate": args.max_false_positive_category_rate,
            },
            "overall_quality_gate_passed": all(
                fixture["standard_evaluate"]["quality_gate_passed"] for fixture in fixtures
            ),
            "fixtures": fixtures,
        }
        print(json.dumps(report))
        return 0
    except Exception as exc:
        print(json.dumps({"status": "error", "error": str(exc)}))
        return 1


if __name__ == "__main__":
    sys.exit(main())
