"""Learned relevance fallback library.

Pure-stdlib, DuckDB-free library that powers the learned retrieval fallback
experiment in ``learned_relevance_experiment.py``.

Design goals (see ``.agent/tasks/2026-06-18-learned-retrieval-fallback/``):

* Sit on top of existing deterministic v4 candidate + score outputs.
* Extract auditable, non-leaky features per (candidate, domain) row.
* Train a tiny deterministic logistic regression with full-batch gradient
  descent (no shuffle, no randomness, no external dependencies).
* Select a retain threshold on training folds only.
* Evaluate with the same metric shape as
  ``calibrate_relevance_fixtures.evaluate_historical`` plus the trap and
  false-positive-category gates used by the retrieval quality bar.

Nothing in this module touches durable DuckDB, frozen validation shards, label
files, or network services. It operates on in-memory Python rows.
"""

from __future__ import annotations

import hashlib
import math
import re
from dataclasses import dataclass, field
from typing import Dict, Iterable, List, Sequence, Tuple


DOMAINS = ("travel", "saas_opportunity", "app_opportunity")
TIERS = ("A", "B", "C", "D")
DECISIONS = ("retain", "evaluate", "discard")

APPROVED_FIXTURES = {
    "comments-2021-01-000",
    "comments-2021-01-001",
}
FROZEN_FIXTURES = {
    "comments-2021-01-002",
    "comments-2021-01-003",
}

DEFAULT_GATES = {
    "minimum_retain_precision": 0.75,
    "minimum_weighted_recall": 0.50,
    "minimum_domain_precision": 0.65,
    "minimum_domain_retained_count": 10,
    "max_false_positive_category_rate": 0.20,
}

# Fields that must never appear as model feature names or feature inputs.
# Using any of these as a feature would leak identity, sampling metadata,
# labels, or annotation categories and would invalidate the experiment.
LEAKAGE_FIELD_SUBSTRINGS = (
    "source_id",
    "source_url",
    "source_type",
    "fixture",
    "stratum",
    "sample_rank",
    "sampling_seed",
    "sampling",
    "label_",
    "label_travel",
    "label_saas",
    "label_app",
    "false_positive",
    "fp_category",
    "subreddit",
    "row_order",
    "row_index",
)

_TOKEN_RE = re.compile(r"[a-z0-9]+")


def tokenize(text: str) -> List[str]:
    return _TOKEN_RE.findall((text or "").lower())


def sigmoid(z: float) -> float:
    if z >= 0:
        ez = math.exp(-z)
        return 1.0 / (1.0 + ez)
    ez = math.exp(z)
    return ez / (1.0 + ez)


def assert_not_frozen(fixture_id: str) -> None:
    if fixture_id in FROZEN_FIXTURES:
        raise ValueError(
            f"fixture {fixture_id} is a frozen validation shard and must not be "
            "accessed by the learned experiment"
        )


def assert_approved_observed(fixture_ids: Iterable[str]) -> None:
    for fixture_id in fixture_ids:
        assert_not_frozen(fixture_id)
        if fixture_id not in APPROVED_FIXTURES:
            raise ValueError(
                f"fixture {fixture_id} is not an approved observed training fixture"
            )


# ---------------------------------------------------------------------------
# Feature specification
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class FeatureSpec:
    """A frozen, deterministic feature specification built from training rows.

    The spec records exactly which features will be produced and in which order
    so that evaluation rows are encoded identically regardless of input order.
    """

    feature_names: Tuple[str, ...]
    # Vocabularies learned from training folds only.
    text_vocab: Tuple[str, ...] = ()
    term_vocab: Tuple[str, ...] = ()
    # Canonical indicator groups derived from the v4 config (not from data).
    rule_groups: Tuple[str, ...] = ()
    proximity_rules: Tuple[str, ...] = ()
    domains: Tuple[str, ...] = ()
    text_min_count: int = 5
    term_min_count: int = 3
    max_text_vocab: int = 120

    def index_of(self, name: str) -> int:
        return self.feature_index[name]

    @property
    def feature_index(self) -> Dict[str, int]:
        return {name: i for i, name in enumerate(self.feature_names)}


def _canonical_rule_groups(config: dict) -> Tuple[str, ...]:
    seen: List[str] = []
    for domain in config.get("domains", []):
        for group in (domain.get("group_weights") or {}).keys():
            if group not in seen:
                seen.append(group)
    return tuple(seen)


def _canonical_proximity_rules(config: dict) -> Tuple[str, ...]:
    seen: List[str] = []
    for domain in config.get("domains", []):
        for rule in domain.get("proximity_rules") or []:
            name = rule.get("name")
            if name and name not in seen:
                seen.append(name)
    return tuple(seen)


def _canonical_domains(config: dict) -> Tuple[str, ...]:
    seen: List[str] = []
    for domain in config.get("domains", []):
        name = domain.get("name")
        if name and name not in seen:
            seen.append(name)
    return tuple(seen)


def _build_vocab(
    training_rows: Sequence[dict],
    field: str,
    min_count: int,
    max_size: int,
) -> Tuple[str, ...]:
    counts: Dict[str, int] = {}
    for row in training_rows:
        for token in row.get(field, []) or []:
            counts[token] = counts.get(token, 0) + 1
    # Deterministic ordering: by descending count, then alphabetical.
    eligible = [
        token
        for token, count in counts.items()
        if count >= min_count
    ]
    eligible.sort(key=lambda t: (-counts[t], t))
    return tuple(eligible[:max_size])


def _check_feature_names_leakage(names: Sequence[str]) -> None:
    bad = []
    for name in names:
        low = name.lower()
        for sub in LEAKAGE_FIELD_SUBSTRINGS:
            if sub in low:
                bad.append((name, sub))
                break
    if bad:
        raise ValueError(
            "feature spec contains leakage-prone feature names: "
            + ", ".join(f"{n} (matches '{s}')" for n, s in bad)
        )


def build_feature_spec(
    training_rows: Sequence[dict],
    config: dict,
    text_min_count: int = 5,
    term_min_count: int = 3,
    max_text_vocab: int = 120,
) -> FeatureSpec:
    """Build a frozen feature spec from training rows + the v4 config.

    Vocabulary features (text tokens, matched terms) are derived from training
    rows only. Indicator groups (rule groups, proximity rules, domains, tiers,
    decisions) are derived from the deterministic v4 config, not from data.
    """
    rule_groups = _canonical_rule_groups(config)
    proximity_rules = _canonical_proximity_rules(config)
    domains = _canonical_domains(config) or DOMAINS
    text_vocab = _build_vocab(training_rows, "text_tokens", text_min_count, max_text_vocab)
    term_vocab = _build_vocab(training_rows, "matched_terms", term_min_count, 1000)

    names: List[str] = []
    for d in domains:
        names.append(f"domain:{d}")
    names.append("v4_score")
    for tier in TIERS:
        names.append(f"tier:{tier}")
    for dec in DECISIONS:
        names.append(f"decision:{dec}")
    for group in rule_groups:
        names.append(f"group:{group}")
    for rule in proximity_rules:
        names.append(f"proximity:{rule}")
    names.append("matched_terms_count")
    names.append("matched_rule_group_count")
    for bucket in ("short", "medium", "long"):
        names.append(f"len_bucket:{bucket}")
    for term in term_vocab:
        names.append(f"term:{term}")
    for token in text_vocab:
        names.append(f"text:{token}")

    _check_feature_names_leakage(names)

    return FeatureSpec(
        feature_names=tuple(names),
        text_vocab=text_vocab,
        term_vocab=term_vocab,
        rule_groups=rule_groups,
        proximity_rules=proximity_rules,
        domains=tuple(domains),
        text_min_count=text_min_count,
        term_min_count=term_min_count,
        max_text_vocab=max_text_vocab,
    )


def _text_length_bucket(token_count: int) -> str:
    if token_count < 40:
        return "short"
    if token_count < 160:
        return "medium"
    return "long"


def extract_features(row: dict, spec: FeatureSpec) -> List[float]:
    """Encode a single (candidate, domain) row into a dense feature vector.

    The encoding is a pure function of (row, spec) and does not depend on
    surrounding row order, fixture identity, or label fields.
    """
    vec = [0.0] * len(spec.feature_names)
    idx = spec.feature_index

    domain = row.get("domain")
    for d in spec.domains:
        if domain == d:
            vec[idx[f"domain:{d}"]] = 1.0

    score = float(row.get("v4_relevance_score") or 0.0)
    vec[idx["v4_score"]] = max(0.0, min(1.0, score))

    tier = row.get("v4_relevance_tier") or "D"
    if tier in TIERS:
        vec[idx[f"tier:{tier}"]] = 1.0

    decision = row.get("v4_decision") or "discard"
    if decision in DECISIONS:
        vec[idx[f"decision:{decision}"]] = 1.0

    matched_groups = set(row.get("matched_rule_groups") or [])
    for group in spec.rule_groups:
        if group in matched_groups:
            vec[idx[f"group:{group}"]] = 1.0

    reasons = set(row.get("v4_decision_reasons") or [])
    for rule in spec.proximity_rules:
        if f"proximity:{rule}" in reasons:
            vec[idx[f"proximity:{rule}"]] = 1.0

    matched_terms = list(row.get("matched_terms") or [])
    vec[idx["matched_terms_count"]] = min(len(matched_terms), 12) / 12.0
    vec[idx["matched_rule_group_count"]] = min(len(matched_groups), 8) / 8.0

    text_tokens = row.get("text_tokens")
    if text_tokens is None:
        text_tokens = tokenize(row.get("candidate_text") or "")
    bucket = _text_length_bucket(len(text_tokens))
    vec[idx[f"len_bucket:{bucket}"]] = 1.0

    term_set = set(matched_terms)
    for term in spec.term_vocab:
        if term in term_set:
            vec[idx[f"term:{term}"]] = 1.0

    text_set = set(text_tokens)
    for token in spec.text_vocab:
        if token in text_set:
            vec[idx[f"text:{token}"]] = 1.0

    return vec


def extract_features_batch(rows: Sequence[dict], spec: FeatureSpec) -> List[List[float]]:
    # extract_features is a pure per-row function and the vocab in `spec` is
    # built from counts, so the result is identical regardless of input row
    # order. Preserve input order so callers can align scores back to rows.
    return [extract_features(row, spec) for row in rows]


# ---------------------------------------------------------------------------
# Deterministic logistic regression (full-batch gradient descent)
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class LogisticModel:
    weights: Tuple[float, ...]
    bias: float
    feature_names: Tuple[str, ...]
    n_epochs: int
    learning_rate: float
    l2: float
    training_loss: Tuple[float, ...] = ()


def train_logistic(
    features: Sequence[Sequence[float]],
    labels: Sequence[int],
    n_epochs: int = 400,
    learning_rate: float = 0.5,
    l2: float = 0.01,
    feature_names: Sequence[str] = (),
    loss_tolerance: float = 1e-7,
    loss_patience: int = 25,
) -> LogisticModel:
    """Train a binary logistic regression with full-batch gradient descent.

    Fully deterministic: no randomness, no shuffle, weights init to zero.
    Stops early once the training loss stops improving by ``loss_tolerance``
    for ``loss_patience`` consecutive epochs. Early stopping is deterministic
    (identical inputs always stop at the same epoch) and keeps the model
    inspectable. Suitable for a few hundred rows and a few hundred features.
    """
    n = len(features)
    if n == 0:
        raise ValueError("cannot train on zero rows")
    if len(labels) != n:
        raise ValueError("features and labels length mismatch")
    dim = len(features[0])
    weights = [0.0] * dim
    bias = 0.0
    losses: List[float] = []
    best_loss = float("inf")
    stale = 0
    stopped_early = False

    for _epoch in range(n_epochs):
        # Forward pass (zip-based hot loop).
        preds = [sigmoid(sum(w * x for w, x in zip(weights, x)) + bias) for x in features]
        # Gradients of mean logistic loss.
        grad_w = [0.0] * dim
        grad_b = 0.0
        for i in range(n):
            err = preds[i] - float(labels[i])
            xi = features[i]
            if err == 0.0:
                continue
            for j in range(dim):
                grad_w[j] += err * xi[j]
            grad_b += err
        inv = 1.0 / n
        for j in range(dim):
            grad_w[j] = grad_w[j] * inv + l2 * weights[j]
        grad_b = grad_b * inv
        for j in range(dim):
            weights[j] -= learning_rate * grad_w[j]
        bias -= learning_rate * grad_b

        # Track loss for diagnostics + deterministic early stopping.
        loss = 0.0
        for i in range(n):
            p = preds[i]
            if p <= 0.0:
                p = 1e-12
            elif p >= 1.0:
                p = 1.0 - 1e-12
            yi = float(labels[i])
            loss += -(yi * math.log(p) + (1.0 - yi) * math.log(1.0 - p))
        loss = loss * inv
        losses.append(loss)
        if best_loss - loss > loss_tolerance:
            best_loss = loss
            stale = 0
        else:
            stale += 1
            if stale >= loss_patience:
                stopped_early = True
                break

    return LogisticModel(
        weights=tuple(weights),
        bias=bias,
        feature_names=tuple(feature_names),
        n_epochs=len(losses),
        learning_rate=learning_rate,
        l2=l2,
        training_loss=tuple(losses),
    )


def _dot(weights: Sequence[float], x: Sequence[float]) -> float:
    s = 0.0
    for j in range(len(weights)):
        s += weights[j] * x[j]
    return s


def predict_scores(model: LogisticModel, features: Sequence[Sequence[float]]) -> List[float]:
    return [sigmoid(_dot(model.weights, x) + model.bias) for x in features]


# ---------------------------------------------------------------------------
# Candidate-level retain decisions + threshold selection
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class CandidateView:
    """Candidate-level rollup of per-domain learned scores + v4 decisions."""

    source_type: str
    source_id: str
    fixture_id: str
    v4_retained: bool
    # (domain, v4_score, v4_decision, learned_score, label_domain)
    domain_rows: Tuple[Tuple[str, float, str, float, int], ...]
    label_any_positive: bool
    source_url: str
    false_positive_category: str


def build_candidate_views(
    rows: Sequence[dict],
    learned_scores: Sequence[float],
) -> List[CandidateView]:
    """Roll per-domain rows up to one view per candidate."""
    if len(rows) != len(learned_scores):
        raise ValueError("rows and learned_scores length mismatch")
    by_key: Dict[Tuple[str, str], dict] = {}
    order: List[Tuple[str, str]] = []
    for i in range(len(rows)):
        row = rows[i]
        key = (row.get("source_type", ""), row.get("source_id", ""))
        if key not in by_key:
            by_key[key] = {
                "source_type": key[0],
                "source_id": key[1],
                "fixture_id": row.get("fixture_id", ""),
                "domain_rows": [],
                "source_url": row.get("source_url", "") or "",
                "false_positive_category": row.get("false_positive_category", "") or "",
            }
            order.append(key)
        by_key[key]["domain_rows"].append(
            (
                row.get("domain", ""),
                float(row.get("v4_relevance_score") or 0.0),
                row.get("v4_decision", "discard"),
                float(learned_scores[i]),
                int(row.get("label") or 0),
            )
        )
    views: List[CandidateView] = []
    for key in order:
        data = by_key[key]
        domain_rows = tuple(sorted(data["domain_rows"], key=lambda t: (-t[3], t[0])))
        v4_retained = any(d[2] == "retain" for d in domain_rows)
        label_any = any(d[4] == 1 for d in domain_rows)
        views.append(
            CandidateView(
                source_type=data["source_type"],
                source_id=data["source_id"],
                fixture_id=data["fixture_id"],
                v4_retained=v4_retained,
                domain_rows=domain_rows,
                label_any_positive=label_any,
                source_url=data["source_url"],
                false_positive_category=data["false_positive_category"],
            )
        )
    return views


def candidate_retained(view: CandidateView, threshold: float) -> bool:
    if view.v4_retained:
        return True
    # Additive promotion: only v4-evaluate candidates may be promoted.
    return any(d[2] == "evaluate" and d[3] >= threshold for d in view.domain_rows)


def candidate_predicted_domain(view: CandidateView, threshold: float) -> str:
    # If v4 retained the candidate, predicted domain is the scorer's own argmax
    # among v4-retain domains (highest v4 score, deterministic tie-break). This
    # matches the deterministic v4 evaluator and avoids mixing v4 and learned
    # score scales. Only pure learned promotions use the learned score.
    retain_domains = [
        (v4_score, domain)
        for domain, v4_score, decision, _learned, _label in view.domain_rows
        if decision == "retain"
    ]
    if retain_domains:
        retain_domains.sort(key=lambda t: (-t[0], t[1]))
        return retain_domains[0][1]
    promoted = [
        (learned, domain)
        for domain, _v4, decision, learned, _label in view.domain_rows
        if decision == "evaluate" and learned >= threshold
    ]
    if promoted:
        promoted.sort(key=lambda t: (-t[0], t[1]))
        return promoted[0][1]
    return ""


def evaluate_threshold(
    views: Sequence[CandidateView],
    threshold: float,
    gates: dict = DEFAULT_GATES,
) -> dict:
    """Compute the full metric block at a given threshold."""
    retained_views = [v for v in views if candidate_retained(v, threshold)]
    retained_count = len(retained_views)
    true_positive = sum(1 for v in retained_views if v.label_any_positive)
    false_positive = retained_count - true_positive
    labeled_positive = sum(1 for v in views if v.label_any_positive)
    false_negative = labeled_positive - true_positive
    precision = true_positive / retained_count if retained_count else 0.0
    recall = true_positive / labeled_positive if labeled_positive else 0.0
    f1 = 2 * precision * recall / (precision + recall) if precision + recall else 0.0

    domain_metrics: Dict[str, dict] = {}
    for domain in DOMAINS:
        dom_retained = [
            v for v in retained_views if candidate_predicted_domain(v, threshold) == domain
        ]
        dom_count = len(dom_retained)
        dom_tp = sum(1 for v in dom_retained if any(d[0] == domain and d[4] == 1 for d in v.domain_rows))
        dom_fp = dom_count - dom_tp
        dom_labeled = sum(1 for v in views if any(d[0] == domain and d[4] == 1 for d in v.domain_rows))
        dom_prec = dom_tp / dom_count if dom_count else 0.0
        dom_rec = dom_tp / dom_labeled if dom_labeled else 0.0
        domain_metrics[domain] = {
            "labeled_positive": dom_labeled,
            "retained_predictions": dom_count,
            "true_positive_retained": dom_tp,
            "false_positive_retained": dom_fp,
            "retained_precision": dom_prec,
            "retained_recall": dom_rec,
        }

    trap_leakage: Dict[str, int] = {}
    visa_fp = 0
    promo_fp = 0
    missing_url = 0
    for v in retained_views:
        if not v.label_any_positive:
            cat = v.false_positive_category
            if cat:
                trap_leakage[cat] = trap_leakage.get(cat, 0) + 1
            if cat == "payment_brand_visa":
                visa_fp += 1
            if cat == "promotion_or_bot":
                promo_fp += 1
        if not v.source_url:
            missing_url += 1

    category_rate_violations = {
        cat: count
        for cat, count in trap_leakage.items()
        if retained_count > 0 and count / retained_count > gates["max_false_positive_category_rate"]
    }
    domain_precision_failures = {
        domain: metrics
        for domain, metrics in domain_metrics.items()
        if metrics["retained_predictions"] >= gates["minimum_domain_retained_count"]
        and metrics["retained_precision"] < gates["minimum_domain_precision"]
    }

    gate_passed = (
        retained_count > 0
        and precision >= gates["minimum_retain_precision"]
        and recall >= gates["minimum_weighted_recall"]
        and not domain_precision_failures
        and missing_url == 0
        and visa_fp == 0
        and promo_fp == 0
        and not category_rate_violations
    )

    return {
        "threshold": threshold,
        "labeled_positive": labeled_positive,
        "retained_predictions": retained_count,
        "true_positive_retained": true_positive,
        "false_positive_retained": false_positive,
        "false_negative_retained": false_negative,
        "retained_precision": precision,
        "retained_recall": recall,
        "exact_retained_precision": precision,
        "fixture_recall": recall,
        "f1": f1,
        "domains": domain_metrics,
        "trap_leakage": trap_leakage,
        "missing_source_url_count": missing_url,
        "visa_retained_false_positives": visa_fp,
        "promotion_bot_retained_false_positives": promo_fp,
        "false_positive_category_rate_violations": category_rate_violations,
        "domain_precision_failures": domain_precision_failures,
        "quality_gate_passed": gate_passed,
    }


def candidate_thresholds(views: Sequence[CandidateView]) -> List[float]:
    """All candidate thresholds drawn from v4-evaluate learned scores.

    Includes +/- infinity boundaries so the sweep covers the empty-promotion
    and promote-all-evaluate extremes.
    """
    scores: List[float] = []
    for v in views:
        for _domain, _v4, decision, learned, _label in v.domain_rows:
            if decision == "evaluate":
                scores.append(learned)
    unique = sorted(set(scores))
    return [math.inf] + unique + [-math.inf]


def select_threshold(
    views: Sequence[CandidateView],
    gates: dict = DEFAULT_GATES,
) -> dict:
    """Select a retain threshold on training-fold candidates.

    Maximizes recall subject to precision >= minimum_retain_precision. Ties are
    broken by higher precision, then higher threshold (more conservative), then
    by the threshold value itself for full determinism.
    """
    min_prec = gates["minimum_retain_precision"]
    best = None
    for t in candidate_thresholds(views):
        m = evaluate_threshold(views, t, gates)
        if m["retained_predictions"] == 0:
            continue
        if m["retained_precision"] < min_prec:
            continue
        # Sort key: maximize recall, then precision, then prefer a HIGHER
        # threshold (more conservative, fewer promotions). `t` may be +/- inf,
        # both of which compare correctly in Python.
        key = (
            m["retained_recall"],
            m["retained_precision"],
            t,
            m["retained_predictions"],
        )
        if best is None or key > best[0]:
            best = (key, t, m)
    if best is None:
        # No threshold meets precision gate: fall back to the most conservative
        # threshold (promote nothing beyond v4 retain) and mark as constrained.
        m = evaluate_threshold(views, math.inf, gates)
        return {
            "threshold": math.inf,
            "selected": False,
            "reason": "no_threshold_meets_precision_gate",
            "metrics": m,
        }
    return {
        "threshold": best[1],
        "selected": True,
        "reason": "max_recall_subject_to_precision",
        "metrics": best[2],
    }


# ---------------------------------------------------------------------------
# Checksums for the frozen model artifact
# ---------------------------------------------------------------------------


def sha256_file(path: str) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(65536), b""):
            h.update(chunk)
    return f"sha256:{h.hexdigest()}"


def sha256_bytes(data: bytes) -> str:
    return f"sha256:{hashlib.sha256(data).hexdigest()}"
