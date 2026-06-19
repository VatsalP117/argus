# Codex Review

## Verdict

`APPROVE`

## Summary

The fix pass resolves the blocking retry/idempotency issue from the first
review. The tiered retention implementation now keeps trusted `A/B` evidence as
the default durable commit behavior, allows `C` review-tier evidence only with
explicit `--include-review-tier`, preserves tier/decision metadata, and refuses
to silently reinterpret an already-validated batch under a different retention
scope.

The updated roadmap/docs also now describe the v4 and learned-fallback outcomes
in past tense, which removes the earlier roadmap contradiction.

Review checks I ran:

```bash
go test ./internal/candidate -run 'TestCommitCandidatesTieredRetention|TestCommitCandidatesReviewTierRetry' -v -count=1
git diff --check
python3 -m py_compile scripts/dev/*.py
go test ./internal/candidate ./cmd/commit-candidates ./cmd/query ./cmd/export-evidence
go test ./...
go vet ./...
```

All checks passed.

I also searched the changed task/docs/code for frozen shard references. The only
references to `comments-2021-01-002` / `comments-2021-01-003` are guardrail text
in the task file; I found no evidence of frozen-shard access.

## Blocking Issues

None.

## Non-Blocking Suggestions

- Keep unrelated untracked files out of this branch:
  - `docs/assets/`
  - `scripts/media/`
  - `docs/reports/pipeline-process-report.html`
- The executor generated a review package under
  `.agent/tasks/2026-06-18-tiered-retention-policy/review/`. It is fine to
  include it if that is your normal artifact practice, but the essential task
  artifacts are `TASK.md`, `PLAN.md`, `IMPLEMENTATION_REPORT.md`, `FIX_REPORT.md`,
  and this `REVIEW.md`.
- A later cleanup should choose one canonical precision gate per stage. Current
  docs still contain historical `70%` gate language while recent experiments
  used `75%`. This is not blocking for the tiered-retention policy change.

## Test Gaps

No blocking test gaps found.

The important cases are covered:

- default commit excludes `C`
- explicit review-tier commit includes `C`
- `C` preserves `relevance_tier = 'C'`, `decision = 'evaluate'`, and
  non-empty `decision_reasons`
- retrying a default-committed batch with review-tier scope errors instead of
  silently skipping
- same-scope retry remains idempotent

## Risk Areas

- `C` rows still share the same `documents` table as trusted rows. This is
  acceptable because `C` is opt-in and distinguishable through
  `document_relevance`, but downstream consumers must filter correctly.
- The current query layer still reads v0 marts and cannot directly enforce tier
  filters over durable `document_relevance`. The follow-up is documented and
  should happen before broad `C`-tier durable ingestion.
- A bounded shard/month trial is still required before wider review-tier
  retention.

## Exact Fix Instructions for Executor

No fixes required.

Before committing, stage only the intended tiered-retention files and leave
unrelated media/report files unstaged. Then commit and push the branch for PR or
merge.
