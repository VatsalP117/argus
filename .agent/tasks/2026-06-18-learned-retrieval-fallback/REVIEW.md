# Codex Review

## Verdict

`APPROVE`

## Summary

The executor implemented the learned retrieval fallback as a bounded experiment
and reported the result honestly as `failed_experiment`. The implementation
matches the plan: it uses only observed fixtures `000` and `001`, refuses frozen
fixtures, avoids new dependencies and services, leaves labels unchanged, and
does not write a model artifact or promote learned scoring.

The experiment answers the intended question. A simple deterministic logistic
reranker can recover recall on `comments-2021-01-001`, but not without precision
collapse and a payment-brand Visa trap leak. That means this branch should be
preserved as a negative result / research artifact, not treated as a retrieval
layer to advance toward frozen validation.

Review checks I ran:

```bash
python3 -m unittest discover -s scripts/dev/tests -v
python3 -m py_compile scripts/dev/*.py
git diff --check
go test ./internal/config ./internal/relevance ./cmd/score-candidates ./cmd/evaluate-relevance
go test ./...
go vet ./...
python3 scripts/dev/learned_relevance_experiment.py \
  --relevance-config configs/relevance/deterministic-v4.yaml \
  --output-dir .tmp/learned-retrieval-review \
  --fixture 'comments-2021-01-000|evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv|evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json' \
  --fixture 'comments-2021-01-001|evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv|evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json'
```

All checks passed. The reproduced experiment matches the report:

- train `000` -> eval `001`: recall improves to `60.0%`, but precision falls
  to `61.4%` and one payment-brand Visa false positive is retained.
- train `001` -> eval `000`: passes, with `76.0%` precision and `79.2%` recall.
- pooled out-of-fold CV lands just below the precision gate at `74.4%`.
- final in-sample observed-only model passes, but is correctly documented as an
  overfit ceiling and not used to create a frozen model artifact.

## Blocking Issues

None for the scoped experiment.

Do not proceed to frozen validation from this branch. The branch result is
`failed_experiment`.

## Non-Blocking Suggestions

- Stage only the learned-retrieval task artifacts, scripts, tests, and report.
  The following untracked files are unrelated to this task and should remain
  unstaged unless the human explicitly asks otherwise:
  - `docs/assets/`
  - `scripts/media/`
  - `docs/reports/pipeline-process-report.html`
- Consider adding a short roadmap update after merge saying the first learned
  fallback failed and the next options are more labels or richer candidate
  context.

## Test Gaps

- No blocking test gaps for this experiment.
- The learned model is not validated on frozen shards by design.
- The experiment does not test richer context features such as parent/thread
  context, because candidate generation changes were out of scope.

## Risk Areas

- The in-sample model passes, but cross-fixture results do not. The report
  correctly treats in-sample success as overfit and should not be used to justify
  frozen validation.
- Feature vocabularies include common text tokens; the leakage guard excludes
  identity and label fields, but this remains a small-data experiment with high
  overfit risk.
- The current negative result suggests that a future attempt should probably
  focus on more labels or richer candidate context rather than another slightly
  more complex model.

## Exact Fix Instructions for Executor

No code fixes required.

Before commit:

1. Stage the intended learned-retrieval files only:
   - `.agent/tasks/2026-06-18-learned-retrieval-fallback/TASK.md`
   - `.agent/tasks/2026-06-18-learned-retrieval-fallback/PLAN.md`
   - `.agent/tasks/2026-06-18-learned-retrieval-fallback/IMPLEMENTATION_REPORT.md`
   - `.agent/tasks/2026-06-18-learned-retrieval-fallback/REVIEW.md`
   - `docs/reports/learned-retrieval-fallback-2026-06-18.md`
   - `scripts/dev/learned_relevance_experiment.py`
   - `scripts/dev/learned_relevance_lib.py`
   - `scripts/dev/tests/__init__.py`
   - `scripts/dev/tests/test_learned_relevance.py`
2. Do not stage `docs/assets/`, `scripts/media/`, or
   `docs/reports/pipeline-process-report.html`.
3. Commit as a failed experiment artifact, not as a promoted retrieval layer.
4. Do not create a model artifact, do not promote learned scoring, and do not
   access frozen shards.
