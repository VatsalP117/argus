# Codex Review

## Verdict

`NEEDS_HUMAN_DECISION`

## Summary

The implementation follows the approved v4 plan: it adds a small
proximity-aware scoring capability, validates the new config shape, preserves
the existing score output schema, adds focused tests, and documents the
observed-only calibration result. I found no code-level blocking issue and no
evidence that frozen shards `comments-2021-01-002` or `comments-2021-01-003`
were accessed.

The reason this is not `APPROVE` is methodological, not implementation quality:
the task gate did not pass. The executor correctly stopped with a
`CHANGE_REQUEST.md` after v4 improved both observed fixtures but still failed
the `comments-2021-01-001` recall gate. A human/planner decision is needed
before this branch should merge or proceed to frozen validation.

Review checks I ran:

```bash
git status --short --branch
git diff --check
go test ./internal/config ./internal/relevance ./cmd/score-candidates ./cmd/evaluate-relevance
go test ./...
go vet ./...
python3 -m py_compile scripts/dev/*.py
python3 scripts/dev/calibrate_relevance_fixtures.py \
  --relevance-config configs/relevance/deterministic-v4.yaml \
  --output-dir .tmp/relevance-v4-review \
  --fixture 'comments-2021-01-000|evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv|evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json' \
  --fixture 'comments-2021-01-001|evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv|evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json'
```

All checks passed except the expected calibration quality gate:

- `comments-2021-01-000`: pass, retained precision `88.1%`, recall `59.0%`
- `comments-2021-01-001`: fail, retained precision `81.6%`, recall `31.6%`
- overall gate: fail

## Blocking Issues

None at the code or artifact level.

The remaining blocker is a product/evaluation decision: whether to reconcile the
passport/citizenship label boundary, pivot to learned retrieval, or explicitly
authorize frozen validation of this mixed v4 result.

## Non-Blocking Suggestions

- Before opening a PR, stage/commit all intended v4 files together and leave
  unrelated `docs/reports/pipeline-process-report.html` out of the commit.
- Consider noting in the roadmap that v4 produced `mixed_calibration` once the
  human/planner decision is made.
- If v4 is kept, a later performance pass should profile the Python DuckDB UDFs
  on a larger shard before any full-month or durable-ingest use.

## Test Gaps

- The focused scorer tests cover near/far proximity and representative trap
  behavior. I did not find a blocking test gap for this calibration task.
- Full-month performance and production-scale behavior remain intentionally
  untested and out of scope.
- Frozen shard behavior remains intentionally untested and must stay in a
  separate validation task if authorized.

## Risk Areas

- The label-boundary conflict on passport/citizenship examples is now the main
  risk. Continuing deterministic rule work without resolving that boundary
  would likely invite fixture-shaped rules or threshold collapse.
- Proximity scoring calls Python UDFs from DuckDB. This is acceptable for the
  bounded calibration use case, but it may become a throughput concern later.
- The working tree includes uncommitted implementation files and untracked task
  artifacts. Review was performed against the working tree, not only committed
  branch history.

## Exact Fix Instructions for Executor

No code fixes requested.

Wait for a human/planner decision on `CHANGE_REQUEST.md`:

1. Reconcile the passport/citizenship label boundary and then re-run observed
   calibration.
2. Accept deterministic ceiling and plan the learned retrieval fallback.
3. Explicitly authorize a separate frozen-validation task for v4 despite the
   mixed observed result.

Do not access frozen shards, promote v4, or merge this branch until that
decision is made.

