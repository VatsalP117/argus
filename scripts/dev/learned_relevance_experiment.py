#!/usr/bin/env python3
"""Learned retrieval fallback experiment.

Sits on top of the existing deterministic v4 candidate + scoring outputs and
tests whether a tiny, local, deterministic logistic reranker can recover
v4-evaluate-tier candidates that deterministic scoring cannot retain without
precision collapse.

Discipline:
* Reuses the existing candidate reconstruction + v4 scoring path (no scorer
  changes, no new dependencies).
* Trains/evaluates only on approved observed fixtures ``000`` and ``001``.
* Refuses frozen validation shards ``002`` / ``003``.
* Cross-fixture (leave-one-fixture-out) and pooled k-fold CV are the honest
  generalization signals; a final observed-only model is produced only if
  cross-fixture diagnostics are credible.
* Never mutates durable DuckDB (in-memory connections only).
* Never promotes the learned layer into default scoring.

See:
- .agent/tasks/2026-06-18-learned-retrieval-fallback/TASK.md
- .agent/tasks/2026-06-18-learned-retrieval-fallback/PLAN.md
"""

from __future__ import annotations

import argparse
import json
import math
import os
import shutil
import subprocess
import sys
from pathlib import Path

# Make sibling dev modules importable when run as a script.
_SCRIPT_DIR = Path(__file__).resolve().parent
if str(_SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(_SCRIPT_DIR))

import calibrate_relevance_fixtures as cal  # noqa: E402
import learned_relevance_lib as L  # noqa: E402


def parse_args():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--relevance-config", required=True)
    parser.add_argument("--output-dir", required=True)
    parser.add_argument(
        "--fixture",
        action="append",
        default=[],
        help="fixture_id|labels_csv|annotations_json (observed fixtures only)",
    )
    parser.add_argument("--text-min-count", type=int, default=5)
    parser.add_argument("--term-min-count", type=int, default=3)
    parser.add_argument("--max-text-vocab", type=int, default=120)
    parser.add_argument("--n-epochs", type=int, default=400)
    parser.add_argument("--learning-rate", type=float, default=0.5)
    parser.add_argument("--l2", type=float, default=0.01)
    parser.add_argument("--cv-folds", type=int, default=5)
    parser.add_argument("--minimum-retain-precision", type=float, default=0.75)
    parser.add_argument("--minimum-weighted-recall", type=float, default=0.50)
    parser.add_argument("--minimum-domain-precision", type=float, default=0.65)
    parser.add_argument("--minimum-domain-retained-count", type=int, default=10)
    parser.add_argument("--max-false-positive-category-rate", type=float, default=0.20)
    parser.add_argument(
        "--write-model-artifact",
        action="store_true",
        help="write configs/relevance/learned-v1.json if diagnostics are credible",
    )
    return parser.parse_args()


def sql_string(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def run_json(cmd):
    completed = subprocess.run(cmd, capture_output=True, text=True, check=False)
    if completed.returncode != 0:
        raise RuntimeError(
            f"command failed ({completed.returncode}): {' '.join(cmd)}\n"
            f"stdout:\n{completed.stdout}\nstderr:\n{completed.stderr}"
        )
    try:
        return json.loads(completed.stdout)
    except json.JSONDecodeError as exc:
        raise RuntimeError(
            f"command did not return JSON: {' '.join(cmd)}\n"
            f"stdout:\n{completed.stdout}\nstderr:\n{completed.stderr}"
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
        fixture_id, labels_path, annotations_path = [p.strip() for p in parts]
        L.assert_not_frozen(fixture_id)
        if fixture_id not in L.APPROVED_FIXTURES:
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
    return [specs[fixture_id] for fixture_id in sorted(specs)]


def reconstruct_and_score(spec, relevance_config, output_dir: Path):
    """Reuse the calibration path to reconstruct candidates and score with v4.

    Returns the candidate parquet path, v4 score parquet path, and the v4
    historical-evaluate baseline (full-label metrics).
    """
    fixture_dir = output_dir / spec["fixture_id"]
    fixture_dir.mkdir(parents=True, exist_ok=True)
    candidate_path = fixture_dir / "reconstructed-candidates.parquet"
    score_path = fixture_dir / "v4-scores.parquet"

    cal.reconstruct_candidates(spec["labels_path"], candidate_path)

    rules = cal.load_relevance_config(Path(relevance_config))
    run_json(
        [
            "python3",
            str(_SCRIPT_DIR / "duckdb_score_candidates.py"),
            "--input-path",
            str(candidate_path),
            "--output-path",
            str(score_path),
            "--rules-json",
            json.dumps(rules, separators=(",", ":")),
            "--duckdb-memory-limit",
            "4GB",
            "--duckdb-threads",
            "4",
            "--duckdb-temp-dir",
            str(output_dir / "duckdb-tmp"),
        ]
    )
    return candidate_path, score_path, rules


def _parse_json_list(value):
    if value is None or value == "":
        return []
    if isinstance(value, list):
        return value
    try:
        parsed = json.loads(value)
    except (json.JSONDecodeError, TypeError):
        return []
    return parsed if isinstance(parsed, list) else []


def load_fixture_rows(spec, candidate_path: Path, score_path: Path, labels_path: Path):
    """Build per (candidate, domain) rows joining candidates, v4 scores, labels."""
    try:
        import duckdb
    except ModuleNotFoundError as exc:
        raise RuntimeError(f"duckdb python package is not installed: {exc}") from exc

    con = duckdb.connect()
    try:
        con.execute(
            f"""
            CREATE TEMP TABLE labels AS
            SELECT * FROM read_csv(
                {sql_string(str(labels_path))},
                header = true,
                all_varchar = true
            )
            """
        )
        con.execute(
            f"""
            CREATE TEMP VIEW candidates AS
            SELECT * FROM read_parquet({sql_string(str(candidate_path))})
            """
        )
        con.execute(
            f"""
            CREATE TEMP VIEW scores AS
            SELECT * FROM read_parquet({sql_string(str(score_path))})
            """
        )
        rows = con.execute(
            """
            SELECT
                s.source_type AS source_type,
                s.source_id AS source_id,
                s.domain AS domain,
                c.candidate_text AS candidate_text,
                c.matched_terms AS candidate_matched_terms,
                c.matched_rule_groups AS candidate_matched_rule_groups,
                s.matched_terms AS score_matched_terms,
                s.matched_rules AS score_matched_rules,
                s.decision_reasons AS decision_reasons,
                CAST(s.relevance_score AS DOUBLE) AS relevance_score,
                s.relevance_tier AS relevance_tier,
                s.decision AS decision,
                l.label_travel AS label_travel,
                l.label_saas_opportunity AS label_saas_opportunity,
                l.label_app_opportunity AS label_app_opportunity,
                COALESCE(l.false_positive_category, '') AS false_positive_category,
                COALESCE(l.source_url, '') AS source_url
            FROM scores s
            JOIN candidates c USING (source_type, source_id)
            JOIN labels l USING (source_type, source_id)
            ORDER BY s.source_type, s.source_id, s.domain
            """
        ).fetchall()
    finally:
        con.close()

    columns = [
        "source_type", "source_id", "domain", "candidate_text",
        "candidate_matched_terms", "candidate_matched_rule_groups",
        "score_matched_terms", "score_matched_rules", "decision_reasons",
        "relevance_score", "relevance_tier", "decision",
        "label_travel", "label_saas_opportunity", "label_app_opportunity",
        "false_positive_category", "source_url",
    ]
    fixture_id = spec["fixture_id"]
    out = []
    for row in rows:
        rec = dict(zip(columns, row))
        domain = rec["domain"]
        label_field = {
            "travel": "label_travel",
            "saas_opportunity": "label_saas_opportunity",
            "app_opportunity": "label_app_opportunity",
        }.get(domain)
        label = int(rec[label_field] == "1") if label_field and rec[label_field] in ("0", "1") else 0
        matched_terms = _parse_json_list(rec.get("candidate_matched_terms")) or _parse_json_list(rec.get("score_matched_terms"))
        matched_rule_groups = _parse_json_list(rec.get("candidate_matched_rule_groups")) or _parse_json_list(rec.get("score_matched_rules"))
        decision_reasons = _parse_json_list(rec.get("decision_reasons"))
        text_tokens = L.tokenize(rec.get("candidate_text") or "")
        out.append({
            "source_type": rec["source_type"],
            "source_id": rec["source_id"],
            "domain": domain,
            "fixture_id": fixture_id,
            "candidate_text": rec.get("candidate_text") or "",
            "text_tokens": text_tokens,
            "matched_terms": [str(t) for t in matched_terms],
            "matched_rule_groups": [str(g) for g in matched_rule_groups],
            "v4_relevance_score": float(rec.get("relevance_score") or 0.0),
            "v4_relevance_tier": rec.get("relevance_tier") or "D",
            "v4_decision": rec.get("decision") or "discard",
            "v4_decision_reasons": [str(r) for r in decision_reasons],
            "label": label,
            "source_url": rec.get("source_url") or "",
            "false_positive_category": rec.get("false_positive_category") or "",
        })
    return out


def write_learned_scores_parquet(rows, learned_scores, threshold, output_path: Path):
    """Write a per-(candidate,domain) parquet with v4 + learned columns.

    Schema stays compatible with the existing score parquet (one row per
    candidate/domain) so downstream tooling can read it, while adding learned
    columns for inspection. Uses a temp CSV -> parquet COPY to avoid any
    pandas/pyarrow dependency.
    """
    import csv

    try:
        import duckdb
    except ModuleNotFoundError as exc:
        raise RuntimeError(f"duckdb python package is not installed: {exc}") from exc

    output_path.parent.mkdir(parents=True, exist_ok=True)
    temp_parquet = output_path.with_name(output_path.name + ".tmp")
    temp_parquet.unlink(missing_ok=True)
    temp_csv = output_path.with_name(output_path.name + ".csv.tmp")
    temp_csv.unlink(missing_ok=True)

    fields = [
        "source_type", "source_id", "domain", "relevance_score",
        "relevance_tier", "decision", "relevance_version",
        "learned_score", "learned_threshold", "learned_promote", "fixture_id",
    ]
    with temp_csv.open("w", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=fields, lineterminator="\n")
        writer.writeheader()
        for row, score in zip(rows, learned_scores):
            writer.writerow({
                "source_type": row["source_type"],
                "source_id": row["source_id"],
                "domain": row["domain"],
                "relevance_score": f"{float(row['v4_relevance_score']):.6f}",
                "relevance_tier": row["v4_relevance_tier"],
                "decision": row["v4_decision"],
                "relevance_version": "learned_v1",
                "learned_score": f"{float(score):.6f}",
                "learned_threshold": (
                    f"{float(threshold):.6f}" if math.isfinite(threshold) else ""
                ),
                "learned_promote": "true" if (
                    row["v4_decision"] == "evaluate" and math.isfinite(threshold)
                    and float(score) >= threshold
                ) else "false",
                "fixture_id": row["fixture_id"],
            })

    con = duckdb.connect()
    try:
        con.execute(
            f"""
            COPY (
                SELECT
                    source_type,
                    source_id,
                    domain,
                    CAST(relevance_score AS DOUBLE) AS relevance_score,
                    relevance_tier,
                    decision,
                    relevance_version,
                    CAST(learned_score AS DOUBLE) AS learned_score,
                    CASE WHEN learned_threshold = '' THEN NULL
                         ELSE CAST(learned_threshold AS DOUBLE) END AS learned_threshold,
                    (learned_promote = 'true') AS learned_promote,
                    fixture_id
                FROM read_csv({sql_string(str(temp_csv))}, header = true, all_varchar = true)
                ORDER BY source_type, source_id, domain
            )
            TO {sql_string(str(temp_parquet))}
            (FORMAT PARQUET, COMPRESSION ZSTD)
            """
        )
    finally:
        con.close()
        temp_csv.unlink(missing_ok=True)
    temp_parquet.replace(output_path)


def evaluate_v4_baseline(rows, gates):
    """v4 baseline = learned evaluator at threshold=+inf (no learned promotion)."""
    views = L.build_candidate_views(rows, [0.0] * len(rows))
    return L.evaluate_threshold(views, math.inf, gates)


def cross_fixture_diagnostic(fixtures_rows, args, gates):
    """Leave-one-fixture-out: train on one fixture, evaluate on the other."""
    results = {}
    fixture_ids = sorted(fixtures_rows.keys())
    for train_id in fixture_ids:
        eval_ids = [fid for fid in fixture_ids if fid != train_id]
        if len(eval_ids) != 1:
            # Single-fixture run: cross-fixture not possible.
            continue
        eval_id = eval_ids[0]
        train_rows = fixtures_rows[train_id]
        eval_rows = fixtures_rows[eval_id]
        spec = L.build_feature_spec(
            train_rows, args.rules, args.text_min_count, args.term_min_count, args.max_text_vocab
        )
        feats = L.extract_features_batch(train_rows, spec)
        labels = [r["label"] for r in train_rows]
        model = L.train_logistic(
            feats, labels, args.n_epochs, args.learning_rate, args.l2, spec.feature_names
        )
        train_scores = L.predict_scores(model, feats)
        train_views = L.build_candidate_views(train_rows, train_scores)
        threshold_sel = L.select_threshold(train_views, gates)
        threshold = threshold_sel["threshold"]

        eval_feats = L.extract_features_batch(eval_rows, spec)
        eval_scores = L.predict_scores(model, eval_feats)
        eval_views = L.build_candidate_views(eval_rows, eval_scores)
        eval_metrics = L.evaluate_threshold(eval_views, threshold, gates)
        results[f"train_{train_id}_eval_{eval_id}"] = {
            "train_fixture": train_id,
            "eval_fixture": eval_id,
            "threshold": threshold,
            "threshold_selection": threshold_sel,
            "n_train_rows": len(train_rows),
            "n_eval_rows": len(eval_rows),
            "n_features": len(spec.feature_names),
            "feature_names": list(spec.feature_names),
            "metrics": eval_metrics,
        }
    return results


def pooled_cv_diagnostic(fixtures_rows, args, gates, k=5):
    """Deterministic candidate-level k-fold CV across all observed fixtures.

    Candidates (not (candidate,domain) rows) are assigned to folds so that all
    domain rows of one candidate stay together. Each candidate is held out
    exactly once; its learned scores and fold threshold are therefore
    out-of-fold. Pooled metrics apply each candidate's own fold threshold by
    rescaling evaluate-domain scores to (learned - threshold) and running the
    lib evaluator at threshold 0.0, which preserves the exact retention rule.
    """
    all_rows = []
    for fid in sorted(fixtures_rows.keys()):
        all_rows.extend(fixtures_rows[fid])

    candidates = sorted({(r["source_type"], r["source_id"]) for r in all_rows})
    candidate_fold = {key: i % k for i, key in enumerate(candidates)}

    held_out_scores = {}   # key -> {domain: learned_score}
    held_out_threshold = {}  # key -> threshold
    fold_reports = []
    for fold in range(k):
        train_rows, eval_rows = [], []
        for r in all_rows:
            key = (r["source_type"], r["source_id"])
            (eval_rows if candidate_fold[key] == fold else train_rows).append(r)
        if not train_rows or not eval_rows:
            continue
        spec = L.build_feature_spec(
            train_rows, args.rules, args.text_min_count, args.term_min_count, args.max_text_vocab
        )
        feats = L.extract_features_batch(train_rows, spec)
        labels = [r["label"] for r in train_rows]
        model = L.train_logistic(
            feats, labels, args.n_epochs, args.learning_rate, args.l2, spec.feature_names
        )
        train_views = L.build_candidate_views(train_rows, L.predict_scores(model, feats))
        threshold_sel = L.select_threshold(train_views, gates)
        threshold = threshold_sel["threshold"]
        eval_feats = L.extract_features_batch(eval_rows, spec)
        eval_scores = L.predict_scores(model, eval_feats)
        eval_views = L.build_candidate_views(eval_rows, eval_scores)
        fold_reports.append({
            "fold": fold,
            "n_train": len(train_rows),
            "n_eval": len(eval_rows),
            "n_eval_candidates": len({(r["source_type"], r["source_id"]) for r in eval_rows}),
            "threshold": threshold,
            "threshold_selected": threshold_sel["selected"],
            "metrics": L.evaluate_threshold(eval_views, threshold, gates),
        })
        for r, sc in zip(eval_rows, eval_scores):
            key = (r["source_type"], r["source_id"])
            held_out_scores.setdefault(key, {})[r["domain"]] = sc
            held_out_threshold[key] = threshold

    # Pooled out-of-fold evaluation with per-candidate fold thresholds.
    pooled_rows, pooled_scores = [], []
    for r in all_rows:
        key = (r["source_type"], r["source_id"])
        if key not in held_out_scores:
            continue
        threshold = held_out_threshold[key]
        if r["v4_decision"] == "evaluate" and math.isfinite(threshold):
            pooled_scores.append(held_out_scores[key].get(r["domain"], 0.0) - threshold)
        else:
            # Retain/discard domains: score is irrelevant to retention at t=0.
            pooled_scores.append(0.0)
        pooled_rows.append(dict(r))
    pooled_views = L.build_candidate_views(pooled_rows, pooled_scores)
    pooled_metrics = L.evaluate_threshold(pooled_views, 0.0, gates)
    pooled_metrics["note"] = (
        "pooled out-of-fold CV; each candidate evaluated with its own fold's "
        "threshold via score rescaling (evaluate scores shifted by -threshold, "
        "evaluator run at t=0.0)"
    )
    return {
        "k": k,
        "n_candidates": len(candidates),
        "folds": fold_reports,
        "pooled_metrics": pooled_metrics,
    }


def final_observed_model(fixtures_rows, args, gates):
    """Train on both fixtures, select threshold on both (in-sample), evaluate.

    This is the optimistic observed-only candidate. Its metrics are in-sample
    and must be read alongside the cross-fixture diagnostics.
    """
    all_rows = []
    for fid in sorted(fixtures_rows.keys()):
        all_rows.extend(fixtures_rows[fid])
    spec = L.build_feature_spec(
        all_rows, args.rules, args.text_min_count, args.term_min_count, args.max_text_vocab
    )
    feats = L.extract_features_batch(all_rows, spec)
    labels = [r["label"] for r in all_rows]
    model = L.train_logistic(
        feats, labels, args.n_epochs, args.learning_rate, args.l2, spec.feature_names
    )
    scores = L.predict_scores(model, feats)
    views = L.build_candidate_views(all_rows, scores)
    threshold_sel = L.select_threshold(views, gates)
    metrics = L.evaluate_threshold(views, threshold_sel["threshold"], gates)
    return {
        "spec": spec,
        "model": model,
        "scores": scores,
        "rows": all_rows,
        "threshold": threshold_sel["threshold"],
        "threshold_selection": threshold_sel,
        "metrics": metrics,
        "n_features": len(spec.feature_names),
        "feature_names": list(spec.feature_names),
    }


def decide_outcome(cross_fixture, final, v4_baseline, gates):
    """Decide ready_for_frozen_validation / needs_more_labels / failed_experiment.

    The honest signal is cross-fixture (leave-one-fixture-out). We require the
    train-000/eval-001 direction to improve 001 recall over v4 while holding
    precision >= 75% and zero traps, and the reverse direction not to collapse.
    """
    v4_001 = v4_baseline.get("comments-2021-01-001", {})
    v4_001_recall = v4_001.get("retained_recall", 0.0)
    v4_001_precision = v4_001.get("retained_precision", 0.0)

    key_001 = "train_comments-2021-01-000_eval_comments-2021-01-001"
    key_000 = "train_comments-2021-01-001_eval_comments-2021-01-000"
    cf_001 = cross_fixture.get(key_001, {}).get("metrics", {})
    cf_000 = cross_fixture.get(key_000, {}).get("metrics", {})

    def clean(m):
        return {
            "precision": m.get("retained_precision", 0.0),
            "recall": m.get("retained_recall", 0.0),
            "visa_fp": m.get("visa_retained_false_positives", 0),
            "promo_fp": m.get("promotion_bot_retained_false_positives", 0),
            "missing_url": m.get("missing_source_url_count", 0),
            "category_violations": bool(m.get("false_positive_category_rate_violations")),
            "domain_failures": bool(m.get("domain_precision_failures")),
            "gate_passed": bool(m.get("quality_gate_passed", False)),
        }

    cf_001_clean = clean(cf_001)
    cf_000_clean = clean(cf_000)

    improves_001 = cf_001_clean["recall"] > v4_001_recall + 1e-9
    holds_precision_001 = cf_001_clean["precision"] >= gates["minimum_retain_precision"]
    no_traps_001 = (
        cf_001_clean["visa_fp"] == 0
        and cf_001_clean["promo_fp"] == 0
        and cf_001_clean["missing_url"] == 0
        and not cf_001_clean["category_violations"]
    )
    no_collapse_000 = cf_000_clean["precision"] >= gates["minimum_retain_precision"]

    ready = (
        improves_001
        and holds_precision_001
        and no_traps_001
        and no_collapse_000
        and cf_001_clean["recall"] >= gates["minimum_weighted_recall"]
    )
    if ready:
        decision = "ready_for_frozen_validation"
    elif improves_001 and holds_precision_001 and no_traps_001:
        # Improved but did not clear the 50% recall gate cross-fixture.
        decision = "needs_more_labels"
    elif cf_001_clean["recall"] > v4_001_recall and holds_precision_001 and no_traps_001:
        decision = "needs_more_labels"
    else:
        decision = "failed_experiment"

    return {
        "decision": decision,
        "v4_001_recall": v4_001_recall,
        "v4_001_precision": v4_001_precision,
        "cross_fixture_001": cf_001_clean,
        "cross_fixture_000": cf_000_clean,
        "criteria": {
            "improves_001_recall": improves_001,
            "holds_precision_001": holds_precision_001,
            "no_traps_001": no_traps_001,
            "no_collapse_000": no_collapse_000,
            "meets_001_recall_gate_cross_fixture": cf_001_clean["recall"] >= gates["minimum_weighted_recall"],
        },
    }


def write_model_artifact(final, cross_fixture, decision, args, fixture_checksums, config_checksum):
    spec = final["spec"]
    model = final["model"]
    artifact = {
        "version": "learned_v1",
        "status": decision,
        "created_for_task": "2026-06-18-learned-retrieval-fallback",
        "not_default": True,
        "description": (
            "Bounded local learned reranker on top of deterministic v4. Additive "
            "only: promotes v4-evaluate-tier candidates; never demotes v4 retain."
        ),
        "base_relevance_config": str(Path(args.relevance_config).name),
        "base_relevance_config_checksum": config_checksum,
        "training_fixture_ids": sorted(L.APPROVED_FIXTURES & set(fixture_checksums.keys())),
        "label_checksums": fixture_checksums,
        "feature_names": list(spec.feature_names),
        "weights": list(model.weights),
        "bias": model.bias,
        "threshold": final["threshold"],
        "n_epochs": model.n_epochs,
        "learning_rate": model.learning_rate,
        "l2": model.l2,
        "text_min_count": spec.text_min_count,
        "term_min_count": spec.term_min_count,
        "max_text_vocab": spec.max_text_vocab,
        "gates": dict(args.gates),
        "cross_fixture_decision": decision,
        "note": (
            "Observed-only, in-sample metrics. Generalization must be confirmed "
            "by cross-fixture diagnostics and, later, frozen validation on "
            "shards 002/003 in a separate task."
        ),
    }
    out_path = Path("configs/relevance/learned-v1.json")
    out_path.parent.mkdir(parents=True, exist_ok=True)
    with out_path.open("w") as f:
        json.dump(artifact, f, indent=2, sort_keys=True)
        f.write("\n")
    return out_path


def main():
    args = parse_args()
    try:
        specs = load_fixture_specs(args.fixture)
        args.gates = {
            "minimum_retain_precision": args.minimum_retain_precision,
            "minimum_weighted_recall": args.minimum_weighted_recall,
            "minimum_domain_precision": args.minimum_domain_precision,
            "minimum_domain_retained_count": args.minimum_domain_retained_count,
            "max_false_positive_category_rate": args.max_false_positive_category_rate,
        }
        output_dir = Path(args.output_dir)
        if output_dir.exists():
            shutil.rmtree(output_dir)
        output_dir.mkdir(parents=True, exist_ok=True)

        args.rules = cal.load_relevance_config(Path(args.relevance_config))
        config_checksum = L.sha256_file(args.relevance_config)

        fixtures_rows = {}
        v4_baseline = {}
        fixture_checksums = {}
        for spec in specs:
            candidate_path, score_path, _rules = reconstruct_and_score(
                spec, args.relevance_config, output_dir
            )
            rows = load_fixture_rows(spec, candidate_path, score_path, spec["labels_path"])
            fixtures_rows[spec["fixture_id"]] = rows
            v4_baseline[spec["fixture_id"]] = evaluate_v4_baseline(rows, args.gates)
            fixture_checksums[spec["fixture_id"]] = {
                "labels": L.sha256_file(str(spec["labels_path"])),
                "annotations": L.sha256_file(str(spec["annotations_path"])),
            }
            # v4-scores.parquet already lives at output_dir/<fixture>/v4-scores.parquet
            # for side-by-side comparison with learned-scores.parquet.

        cross = cross_fixture_diagnostic(fixtures_rows, args, args.gates)
        pooled = pooled_cv_diagnostic(fixtures_rows, args, args.gates, k=args.cv_folds)
        final = final_observed_model(fixtures_rows, args, args.gates)
        decision = decide_outcome(cross, final, v4_baseline, args.gates)

        # Write learned scores parquet per fixture using the final observed model.
        for spec in specs:
            fid = spec["fixture_id"]
            rows = fixtures_rows[fid]
            fid_scores = [
                final["scores"][i]
                for i, r in enumerate(final["rows"])
                if r["fixture_id"] == fid
            ]
            write_learned_scores_parquet(
                rows, fid_scores, final["threshold"],
                output_dir / fid / "learned-scores.parquet",
            )

        report = {
            "status": "completed",
            "relevance_config": args.relevance_config,
            "relevance_config_checksum": config_checksum,
            "output_dir": str(output_dir),
            "gates": args.gates,
            "hyperparameters": {
                "n_epochs": args.n_epochs,
                "learning_rate": args.learning_rate,
                "l2": args.l2,
                "text_min_count": args.text_min_count,
                "term_min_count": args.term_min_count,
                "max_text_vocab": args.max_text_vocab,
                "cv_folds": args.cv_folds,
            },
            "fixture_checksums": fixture_checksums,
            "v4_baseline": v4_baseline,
            "cross_fixture": cross,
            "pooled_cv": pooled,
            "final_observed_only": {
                "threshold": final["threshold"],
                "threshold_selection": final["threshold_selection"],
                "metrics": final["metrics"],
                "n_features": final["n_features"],
                "n_train_rows": len(final["rows"]),
                "in_sample": True,
            },
            "decision": decision,
            "frozen_shards_accessed": [],
        }

        if decision["decision"] == "ready_for_frozen_validation" and args.write_model_artifact:
            artifact_path = write_model_artifact(
                final, cross, decision["decision"], args, fixture_checksums, config_checksum
            )
            report["model_artifact"] = str(artifact_path)
        else:
            report["model_artifact"] = None

        out_json = output_dir / "diagnostics.json"
        with out_json.open("w") as f:
            json.dump(report, f, indent=2, sort_keys=True)
            f.write("\n")

        print(json.dumps(report, indent=2, sort_keys=True))
        return 0
    except Exception as exc:
        print(json.dumps({"status": "error", "error": str(exc)}))
        return 1


if __name__ == "__main__":
    sys.exit(main())
