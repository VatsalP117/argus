# Codex Review

## Verdict

`APPROVE`

## Summary

The executor implemented the v4 proximity scorer, documented the mixed
calibration result, then correctly handled the human label-boundary decision in
the fix pass. The passport/citizenship/vaccine-passport boundary is now
reconciled consistently across the two observed fixtures, and the remaining
failure on `comments-2021-01-001` is clearly documented as a deterministic
recall ceiling.

This approval is for the branch as a completed calibration result, not for
promoting v4 to production/default scoring. V4 still fails the observed recall
gate on `001`, so the next roadmap step should be the learned retrieval fallback
unless a human explicitly lowers the gate or authorizes frozen validation of the
mixed result.

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
  --output-dir .tmp/relevance-v4-label-reconciled-review \
  --fixture 'comments-2021-01-000|evaluations/relevance/broad-shard-2021-01-000-v1-labels.csv|evaluations/relevance/broad-shard-2021-01-000-v1-annotations.json' \
  --fixture 'comments-2021-01-001|evaluations/relevance/adjacent-comments-2021-01-001-v2-labels.csv|evaluations/relevance/adjacent-comments-2021-01-001-v2-annotations.json'
```

All code checks passed. Calibration reproduced the reported result:

- `comments-2021-01-000`: pass, retained precision `85.1%`, recall `59.4%`
- `comments-2021-01-001`: fail, retained precision `81.6%`, recall `36.5%`
- overall quality gate: fail

I also checked for frozen-shard references in the task artifacts, config,
report, labels, annotations, and changed scorer files. References to
`comments-2021-01-002` and `comments-2021-01-003` appear only in guardrail text;
I found no evidence they were accessed or used.

## Blocking Issues

None.

The remaining recall failure is an expected calibration outcome and is
documented in `CHANGE_REQUEST.md` and `FIX_REPORT.md`.

## Non-Blocking Suggestions

- Fix the small markdown typo in the report table:
  `**40.9%`** should be `**40.9%**`.
- Before commit/PR, leave unrelated `docs/reports/pipeline-process-report.html`
  unstaged.
- Consider updating the roadmap after merge to say v4 produced a mixed result
  and the next recommended branch is learned retrieval fallback.

## Test Gaps

- No blocking test gaps for this scoped calibration task.
- Full-month performance of Python DuckDB proximity UDFs remains untested and
  should be profiled before any production ingest use.
- Frozen validation remains intentionally untested and must be a separate task
  if later authorized.

## Risk Areas

- V4 is not a passing retrieval configuration. Treat it as a useful deterministic
  front door improvement and calibration artifact, not as a promoted scorer.
- Continuing deterministic rule work from here risks fixture-shaped heuristics
  or threshold collapse. The documented evidence supports moving to the learned
  retrieval fallback.
- Label reconciliation changed observed training labels, so future reports
  should reference the reconciled-label checksums now recorded in the v4 report.

## Exact Fix Instructions for Executor

No blocking fixes required.

Before final commit, optionally fix the markdown typo noted above. Then stage
only the intended v4 calibration files and leave
`docs/reports/pipeline-process-report.html` out of the commit.

Do not promote v4, do not access frozen shards, and do not start frozen
validation on this branch. The next implementation task should plan the learned
retrieval fallback.

