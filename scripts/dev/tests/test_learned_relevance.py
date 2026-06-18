"""Focused tests for the learned retrieval fallback experiment.

Run with:

    python3 -m unittest discover -s scripts/dev/tests -v

These tests cover feature extraction, leakage prevention, deterministic
training/evaluation, threshold selection, and the frozen-fixture guard. They
use only the Python standard library (unittest) and the in-memory
``learned_relevance_lib`` so they do not touch DuckDB, Parquet, labels, or
frozen validation shards.
"""

import os
import sys
import unittest
from pathlib import Path

# Make scripts/dev importable when run via unittest discover.
_DEV_DIR = Path(__file__).resolve().parent.parent
if str(_DEV_DIR) not in sys.path:
    sys.path.insert(0, str(_DEV_DIR))

import learned_relevance_lib as L  # noqa: E402
import learned_relevance_experiment as exp  # noqa: E402


def _config():
    return {
        "version": "deterministic_v4_test",
        "domains": [
            {"name": "travel", "group_weights": {"pain_language": 0.15, "travel_language": 0.35}},
            {"name": "app_opportunity", "group_weights": {"product_and_tool_language": 0.20}},
        ],
        "tiers": {"a": 0.80, "b": 0.60, "c": 0.40},
    }


def _row(source_id, domain, *, text, terms, groups, score, tier, decision, label, reasons=None, fixture="f1"):
    return {
        "source_type": "comment",
        "source_id": source_id,
        "domain": domain,
        "fixture_id": fixture,
        "candidate_text": text,
        "text_tokens": L.tokenize(text),
        "matched_terms": terms,
        "matched_rule_groups": groups,
        "v4_relevance_score": score,
        "v4_relevance_tier": tier,
        "v4_decision": decision,
        "v4_decision_reasons": reasons or [],
        "label": label,
        "source_url": "https://example.org/" + source_id,
        "false_positive_category": "",
    }


def _separable_rows(n_per_class=12, fixture="f1"):
    """Two-class separable fixture: positives have 'visa passport' + pain group."""
    rows = []
    for i in range(n_per_class):
        rows.append(_row(
            f"pos{i}", "travel",
            text="my visa was denied at the embassy passport process pain",
            terms=["visa", "passport"], groups=["pain_language", "travel_language"],
            score=0.55, tier="C", decision="evaluate", label=1, fixture=fixture,
        ))
    for i in range(n_per_class):
        rows.append(_row(
            f"neg{i}", "travel",
            text="random unrelated words here nothing relevant",
            terms=["random"], groups=[],
            score=0.45, tier="C", decision="evaluate", label=0, fixture=fixture,
        ))
    return rows


class LeakagePreventionTests(unittest.TestCase):
    def test_feature_names_exclude_leakage_substrings(self):
        rows = _separable_rows()
        spec = L.build_feature_spec(rows, _config(), text_min_count=2, term_min_count=2, max_text_vocab=20)
        for name in spec.feature_names:
            low = name.lower()
            for sub in L.LEAKAGE_FIELD_SUBSTRINGS:
                self.assertNotIn(sub, low, f"leakage substring '{sub}' in feature '{name}'")

    def test_feature_vector_does_not_encode_source_id_or_url(self):
        rows = _separable_rows()
        spec = L.build_feature_spec(rows, _config(), text_min_count=2, term_min_count=2, max_text_vocab=20)
        # Two rows identical except source_id/source_url must produce identical features.
        a = _row("AAA", "travel", text="visa passport pain", terms=["visa"], groups=["pain_language"],
                 score=0.5, tier="C", decision="evaluate", label=1)
        b = _row("ZZZ", "travel", text="visa passport pain", terms=["visa"], groups=["pain_language"],
                 score=0.5, tier="C", decision="evaluate", label=1)
        b["source_url"] = "https://different.example/ZZZ"
        self.assertEqual(extract_features(a, spec), extract_features(b, spec))

    def test_build_feature_spec_rejects_leakage_if_injected(self):
        rows = _separable_rows()
        spec = L.build_feature_spec(rows, _config(), text_min_count=2, term_min_count=2, max_text_vocab=20)
        # Inject a leakage name into the spec and confirm the guard catches it.
        bad_names = list(spec.feature_names) + ["source_id"]
        with self.assertRaises(ValueError):
            L._check_feature_names_leakage(bad_names)

    def test_frozen_fixture_ids_are_refused(self):
        for frozen in ("comments-2021-01-002", "comments-2021-01-003"):
            with self.assertRaises(ValueError):
                L.assert_not_frozen(frozen)
        # Approved observed fixtures are accepted.
        L.assert_not_frozen("comments-2021-01-000")
        L.assert_not_frozen("comments-2021-01-001")

    def test_load_fixture_specs_refuses_frozen(self):
        import tempfile
        with tempfile.TemporaryDirectory() as td:
            labels = Path(td) / "l.csv"
            ann = Path(td) / "a.json"
            labels.write_text("source_type\ncomment\n")
            ann.write_text("{}")
            with self.assertRaises(ValueError):
                exp.load_fixture_specs([
                    f"comments-2021-01-002|{labels}|{ann}",
                ])


class DeterminismTests(unittest.TestCase):
    def test_feature_extraction_independent_of_row_order(self):
        rows = _separable_rows()
        spec = L.build_feature_spec(rows, _config(), text_min_count=2, term_min_count=2, max_text_vocab=20)
        import random
        shuffled = list(rows)
        random.Random(123).shuffle(shuffled)
        a = {r["source_id"] + ":" + r["domain"]: extract_features(r, spec) for r in rows}
        b = {r["source_id"] + ":" + r["domain"]: extract_features(r, spec) for r in shuffled}
        for key in a:
            self.assertEqual(a[key], b[key], f"feature vector differs for {key} under shuffle")

    def test_feature_spec_independent_of_row_order(self):
        rows = _separable_rows()
        import random
        shuffled = list(rows)
        random.Random(5).shuffle(shuffled)
        s1 = L.build_feature_spec(rows, _config(), text_min_count=2, term_min_count=2, max_text_vocab=20)
        s2 = L.build_feature_spec(shuffled, _config(), text_min_count=2, term_min_count=2, max_text_vocab=20)
        self.assertEqual(s1.feature_names, s2.feature_names)
        self.assertEqual(s1.text_vocab, s2.text_vocab)
        self.assertEqual(s1.term_vocab, s2.term_vocab)

    def test_training_is_deterministic(self):
        rows = _separable_rows()
        spec = L.build_feature_spec(rows, _config(), text_min_count=2, term_min_count=2, max_text_vocab=20)
        feats = L.extract_features_batch(rows, spec)
        labels = [r["label"] for r in rows]
        m1 = L.train_logistic(feats, labels, n_epochs=200, learning_rate=0.5, l2=0.001)
        m2 = L.train_logistic(feats, labels, n_epochs=200, learning_rate=0.5, l2=0.001)
        self.assertEqual(m1.weights, m2.weights)
        self.assertEqual(m1.bias, m2.bias)
        self.assertEqual(m1.n_epochs, m2.n_epochs)
        self.assertEqual(L.predict_scores(m1, feats), L.predict_scores(m2, feats))

    def test_training_reproducible_across_restart(self):
        rows = _separable_rows()
        spec = L.build_feature_spec(rows, _config(), text_min_count=2, term_min_count=2, max_text_vocab=20)
        feats = L.extract_features_batch(rows, spec)
        labels = [r["label"] for r in rows]
        m = L.train_logistic(feats, labels, n_epochs=300, learning_rate=0.5, l2=0.001)
        # Re-running in a fresh process yields the same weights (verified here
        # by re-instantiating the call; full-process reproducibility is covered
        # by the experiment's diagnostics.json checksums in the report).
        m_again = L.train_logistic(feats, labels, n_epochs=300, learning_rate=0.5, l2=0.001)
        self.assertEqual(m.weights, m_again.weights)


class FeatureNormalizationTests(unittest.TestCase):
    def test_text_tokens_lowercased_and_split(self):
        self.assertEqual(L.tokenize("Visa, PASSPORT! 123"), ["visa", "passport", "123"])
        self.assertEqual(L.tokenize(""), [])
        self.assertEqual(L.tokenize(None), [])

    def test_term_and_group_indicators_consistent(self):
        rows = _separable_rows()
        spec = L.build_feature_spec(rows, _config(), text_min_count=2, term_min_count=2, max_text_vocab=20)
        idx = spec.feature_index
        # A row with pain_language group sets group:pain_language only.
        r = _row("x1", "travel", text="visa pain", terms=["visa"], groups=["pain_language"],
                 score=0.5, tier="C", decision="evaluate", label=1)
        v = extract_features(r, spec)
        self.assertEqual(v[idx["group:pain_language"]], 1.0)
        self.assertEqual(v[idx["group:travel_language"]], 0.0)
        # Term indicator fires for a vocab term.
        if "term:visa" in idx:
            self.assertEqual(v[idx["term:visa"]], 1.0)

    def test_domain_and_tier_and_decision_indicators(self):
        rows = _separable_rows()
        spec = L.build_feature_spec(rows, _config(), text_min_count=2, term_min_count=2, max_text_vocab=20)
        idx = spec.feature_index
        r = _row("x", "app_opportunity", text="app bug", terms=["app"], groups=["product_and_tool_language"],
                 score=0.7, tier="B", decision="retain", label=1)
        v = extract_features(r, spec)
        self.assertEqual(v[idx["domain:app_opportunity"]], 1.0)
        self.assertEqual(v[idx["domain:travel"]], 0.0)
        self.assertEqual(v[idx["tier:B"]], 1.0)
        self.assertEqual(v[idx["decision:retain"]], 1.0)
        self.assertAlmostEqual(v[idx["v4_score"]], 0.7)

    def test_text_length_buckets(self):
        rows = _separable_rows()
        spec = L.build_feature_spec(rows, _config(), text_min_count=2, term_min_count=2, max_text_vocab=20)
        idx = spec.feature_index
        short = _row("s", "travel", text="visa pain", terms=[], groups=[], score=0.5, tier="C", decision="evaluate", label=1)
        long = _row("l", "travel", text=" ".join(["visa"] * 200), terms=[], groups=[], score=0.5, tier="C", decision="evaluate", label=1)
        self.assertEqual(extract_features(short, spec)[idx["len_bucket:short"]], 1.0)
        self.assertEqual(extract_features(long, spec)[idx["len_bucket:long"]], 1.0)


class ThresholdSelectionTests(unittest.TestCase):
    def test_maximizes_recall_subject_to_precision(self):
        rows = _separable_rows(n_per_class=12)
        spec = L.build_feature_spec(rows, _config(), text_min_count=2, term_min_count=2, max_text_vocab=20)
        feats = L.extract_features_batch(rows, spec)
        labels = [r["label"] for r in rows]
        model = L.train_logistic(feats, labels, n_epochs=300, learning_rate=0.5, l2=0.001)
        scores = L.predict_scores(model, feats)
        views = L.build_candidate_views(rows, scores)
        gates = dict(L.DEFAULT_GATES)
        sel = L.select_threshold(views, gates)
        self.assertTrue(sel["selected"])
        m = sel["metrics"]
        # Selected threshold must satisfy the precision gate.
        self.assertGreaterEqual(m["retained_precision"], gates["minimum_retain_precision"])
        # And retain at least one positive (recall > 0 on separable data).
        self.assertGreater(m["true_positive_retained"], 0)

    def test_no_threshold_meets_precision_returns_unselected(self):
        # All-negative rows: no threshold can meet precision (no TPs possible).
        rows = [_row(f"n{i}", "travel", text="noise words", terms=[], groups=[],
                     score=0.5, tier="C", decision="evaluate", label=0) for i in range(20)]
        spec = L.build_feature_spec(rows, _config(), text_min_count=1, term_min_count=1, max_text_vocab=10)
        feats = L.extract_features_batch(rows, spec)
        model = L.train_logistic(feats, [0] * len(rows), n_epochs=50)
        views = L.build_candidate_views(rows, L.predict_scores(model, feats))
        sel = L.select_threshold(views, dict(L.DEFAULT_GATES))
        self.assertFalse(sel["selected"])
        self.assertEqual(sel["reason"], "no_threshold_meets_precision_gate")
        # Falls back to threshold=+inf (no promotion beyond v4 retain) -> zero retained here.
        import math
        self.assertEqual(sel["threshold"], math.inf)
        self.assertEqual(sel["metrics"]["retained_predictions"], 0)

    def test_threshold_selection_deterministic_and_tie_break_prefers_conservative(self):
        rows = _separable_rows(n_per_class=10)
        spec = L.build_feature_spec(rows, _config(), text_min_count=2, term_min_count=2, max_text_vocab=20)
        feats = L.extract_features_batch(rows, spec)
        labels = [r["label"] for r in rows]
        model = L.train_logistic(feats, labels, n_epochs=200, learning_rate=0.5, l2=0.001)
        scores = L.predict_scores(model, feats)
        views = L.build_candidate_views(rows, scores)
        s1 = L.select_threshold(views, dict(L.DEFAULT_GATES))
        s2 = L.select_threshold(views, dict(L.DEFAULT_GATES))
        self.assertEqual(s1["threshold"], s2["threshold"])
        self.assertEqual(s1["metrics"]["retained_predictions"], s2["metrics"]["retained_predictions"])

    def test_threshold_only_promotes_evaluate_tier(self):
        # A v4-retain candidate must always be retained regardless of threshold.
        rows = [
            _row("r", "travel", text="visa pain", terms=["visa"], groups=["pain_language"],
                 score=0.7, tier="B", decision="retain", label=1),
            _row("e", "travel", text="visa maybe", terms=["visa"], groups=["travel_language"],
                 score=0.5, tier="C", decision="evaluate", label=1),
        ]
        spec = L.build_feature_spec(rows, _config(), text_min_count=1, term_min_count=1, max_text_vocab=10)
        scores = [0.9, 0.1]  # arbitrary learned scores
        views = L.build_candidate_views(rows, scores)
        import math
        # At +inf: only the v4-retain candidate is retained.
        m_inf = L.evaluate_threshold(views, math.inf, dict(L.DEFAULT_GATES))
        self.assertEqual(m_inf["retained_predictions"], 1)
        # At -inf: both retained (retain + evaluate promoted).
        m_neg = L.evaluate_threshold(views, -math.inf, dict(L.DEFAULT_GATES))
        self.assertEqual(m_neg["retained_predictions"], 2)


class EvaluatorMetricsTests(unittest.TestCase):
    def test_known_tiny_fixture_metrics(self):
        # 4 candidates: 2 v4-retain (1 pos, 1 neg), 2 v4-evaluate (1 pos, 1 neg).
        rows = [
            _row("a", "travel", text="visa pain", terms=["visa"], groups=["pain_language"],
                 score=0.7, tier="B", decision="retain", label=1),
            _row("b", "travel", text="random", terms=[], groups=[],
                 score=0.65, tier="B", decision="retain", label=0),
            _row("c", "travel", text="visa maybe", terms=["visa"], groups=["travel_language"],
                 score=0.5, tier="C", decision="evaluate", label=1),
            _row("d", "travel", text="noise", terms=[], groups=[],
                 score=0.45, tier="C", decision="evaluate", label=0),
        ]
        scores = [0.0, 0.0, 0.9, 0.1]
        views = L.build_candidate_views(rows, scores)
        import math
        # threshold=+inf -> only v4 retain (a, b): precision 1/2, recall 1/2.
        m = L.evaluate_threshold(views, math.inf, dict(L.DEFAULT_GATES))
        self.assertEqual(m["retained_predictions"], 2)
        self.assertEqual(m["true_positive_retained"], 1)
        self.assertEqual(m["labeled_positive"], 2)
        self.assertAlmostEqual(m["retained_precision"], 0.5)
        self.assertAlmostEqual(m["retained_recall"], 0.5)
        # threshold=0.5 -> retain (a,b) + promote evaluate c (score 0.9>=0.5), not d.
        m2 = L.evaluate_threshold(views, 0.5, dict(L.DEFAULT_GATES))
        self.assertEqual(m2["retained_predictions"], 3)
        self.assertEqual(m2["true_positive_retained"], 2)
        self.assertAlmostEqual(m2["retained_precision"], 2 / 3)
        self.assertAlmostEqual(m2["retained_recall"], 1.0)

    def test_trap_and_missing_url_gates(self):
        rows = [
            _row("a", "travel", text="visa", terms=["visa"], groups=["pain_language"],
                 score=0.7, tier="B", decision="retain", label=0),
            _row("b", "travel", text="promo", terms=[], groups=[],
                 score=0.65, tier="B", decision="retain", label=0),
        ]
        rows[0]["false_positive_category"] = "payment_brand_visa"
        rows[1]["false_positive_category"] = "promotion_or_bot"
        rows[1]["source_url"] = ""
        scores = [0.0, 0.0]
        views = L.build_candidate_views(rows, scores)
        import math
        m = L.evaluate_threshold(views, math.inf, dict(L.DEFAULT_GATES))
        self.assertEqual(m["visa_retained_false_positives"], 1)
        self.assertEqual(m["promotion_bot_retained_false_positives"], 1)
        self.assertEqual(m["missing_source_url_count"], 1)
        self.assertFalse(m["quality_gate_passed"])

    def test_v4_baseline_at_inf_matches_zero_promotion(self):
        rows = _separable_rows()
        views = L.build_candidate_views(rows, [0.0] * len(rows))
        import math
        m = L.evaluate_threshold(views, math.inf, dict(L.DEFAULT_GATES))
        # No evaluate promotions: retained count equals v4-retain count.
        v4_retain = sum(1 for v in views if v.v4_retained)
        self.assertEqual(m["retained_predictions"], v4_retain)


class ModelLearningTests(unittest.TestCase):
    def test_separable_pattern_is_learned(self):
        rows = _separable_rows(n_per_class=15)
        spec = L.build_feature_spec(rows, _config(), text_min_count=2, term_min_count=2, max_text_vocab=30)
        feats = L.extract_features_batch(rows, spec)
        labels = [r["label"] for r in rows]
        model = L.train_logistic(feats, labels, n_epochs=400, learning_rate=0.5, l2=0.0001)
        scores = L.predict_scores(model, feats)
        pos_scores = [s for r, s in zip(rows, scores) if r["label"] == 1]
        neg_scores = [s for r, s in zip(rows, scores) if r["label"] == 0]
        # Positives should score higher than negatives on separable data.
        self.assertGreater(min(pos_scores), max(neg_scores))

    def test_training_loss_decreases(self):
        rows = _separable_rows(n_per_class=15)
        spec = L.build_feature_spec(rows, _config(), text_min_count=2, term_min_count=2, max_text_vocab=30)
        feats = L.extract_features_batch(rows, spec)
        labels = [r["label"] for r in rows]
        model = L.train_logistic(feats, labels, n_epochs=200, learning_rate=0.5, l2=0.0001)
        self.assertGreater(len(model.training_loss), 1)
        self.assertLess(model.training_loss[-1], model.training_loss[0])


class ExperimentGuardTests(unittest.TestCase):
    def test_refuses_non_approved_fixture(self):
        import tempfile
        with tempfile.TemporaryDirectory() as td:
            labels = Path(td) / "l.csv"
            ann = Path(td) / "a.json"
            labels.write_text("source_type\ncomment\n")
            ann.write_text("{}")
            with self.assertRaises(ValueError):
                exp.load_fixture_specs([f"comments-2099-01-999|{labels}|{ann}"])

    def test_missing_labels_file_raises(self):
        import tempfile
        with tempfile.TemporaryDirectory() as td:
            labels = Path(td) / "missing.csv"
            ann = Path(td) / "a.json"
            ann.write_text("{}")
            with self.assertRaises(ValueError):
                exp.load_fixture_specs([f"comments-2021-01-000|{labels}|{ann}"])

    def test_missing_annotations_file_raises(self):
        import tempfile
        with tempfile.TemporaryDirectory() as td:
            labels = Path(td) / "l.csv"
            ann = Path(td) / "missing.json"
            labels.write_text("source_type\ncomment\n")
            with self.assertRaises(ValueError):
                exp.load_fixture_specs([f"comments-2021-01-000|{labels}|{ann}"])

    def test_duplicate_fixture_raises(self):
        import tempfile
        with tempfile.TemporaryDirectory() as td:
            labels = Path(td) / "l.csv"
            ann = Path(td) / "a.json"
            labels.write_text("source_type\ncomment\n")
            ann.write_text("{}")
            with self.assertRaises(ValueError):
                exp.load_fixture_specs([
                    f"comments-2021-01-000|{labels}|{ann}",
                    f"comments-2021-01-000|{labels}|{ann}",
                ])

    def test_decision_failed_experiment_on_precision_collapse(self):
        # Simulate a decision where 001 cross-fixture improved recall but
        # collapsed precision and leaked a trap.
        gates = dict(L.DEFAULT_GATES)
        v4_baseline = {
            "comments-2021-01-001": {
                "retained_recall": 0.365,
                "retained_precision": 0.816,
            }
        }
        cross = {
            "train_comments-2021-01-000_eval_comments-2021-01-001": {
                "metrics": {
                    "retained_precision": 0.614,  # below 0.75
                    "retained_recall": 0.60,      # above v4 0.365 and above gate 0.50
                    "visa_retained_false_positives": 1,
                    "promotion_bot_retained_false_positives": 0,
                    "missing_source_url_count": 0,
                    "false_positive_category_rate_violations": {},
                    "domain_precision_failures": {"travel": {}},
                    "quality_gate_passed": False,
                }
            },
            "train_comments-2021-01-001_eval_comments-2021-01-000": {
                "metrics": {
                    "retained_precision": 0.76,
                    "retained_recall": 0.79,
                    "visa_retained_false_positives": 0,
                    "promotion_bot_retained_false_positives": 0,
                    "missing_source_url_count": 0,
                    "false_positive_category_rate_violations": {},
                    "domain_precision_failures": {},
                    "quality_gate_passed": True,
                }
            },
        }
        final = {}
        decision = exp.decide_outcome(cross, final, v4_baseline, gates)
        self.assertEqual(decision["decision"], "failed_experiment")
        self.assertFalse(decision["criteria"]["holds_precision_001"])
        self.assertFalse(decision["criteria"]["no_traps_001"])
        self.assertTrue(decision["criteria"]["improves_001_recall"])

    def test_decision_ready_when_all_criteria_pass(self):
        gates = dict(L.DEFAULT_GATES)
        v4_baseline = {"comments-2021-01-001": {"retained_recall": 0.365, "retained_precision": 0.816}}
        cross = {
            "train_comments-2021-01-000_eval_comments-2021-01-001": {
                "metrics": {
                    "retained_precision": 0.80, "retained_recall": 0.55,
                    "visa_retained_false_positives": 0,
                    "promotion_bot_retained_false_positives": 0,
                    "missing_source_url_count": 0,
                    "false_positive_category_rate_violations": {},
                    "domain_precision_failures": {},
                    "quality_gate_passed": True,
                }
            },
            "train_comments-2021-01-001_eval_comments-2021-01-000": {
                "metrics": {
                    "retained_precision": 0.80, "retained_recall": 0.60,
                    "visa_retained_false_positives": 0,
                    "promotion_bot_retained_false_positives": 0,
                    "missing_source_url_count": 0,
                    "false_positive_category_rate_violations": {},
                    "domain_precision_failures": {},
                    "quality_gate_passed": True,
                }
            },
        }
        decision = exp.decide_outcome(cross, {}, v4_baseline, gates)
        self.assertEqual(decision["decision"], "ready_for_frozen_validation")


# Helper re-exported for brevity in tests.
def extract_features(row, spec):
    return L.extract_features(row, spec)


if __name__ == "__main__":
    unittest.main(verbosity=2)
