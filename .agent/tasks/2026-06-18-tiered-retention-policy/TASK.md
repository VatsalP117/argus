# Tiered Retention Policy Task

## Objective

Revise Argus from a strict binary retrieval gate into a tiered retention system
that can store useful medium-confidence evidence without pretending every
retained row has the same trust level.

The goal is to let Argus move toward bounded durable ingestion while preserving
research quality through explicit confidence tiers, query defaults, provenance,
and storage guardrails.

## Background

The retrieval-quality work produced three important findings:

- `deterministic_v4` improved precision and recall but failed the strict recall
  gate on `comments-2021-01-001`.
- A simple learned reranker recovered recall but collapsed precision and leaked
  a payment-brand Visa false positive.
- Both failures suggest the problem is not just one missing scorer feature; the
  product needs to distinguish strong evidence from exploratory evidence.

The user has decided that some unwanted rows entering the durable DB is
acceptable if:

- confidence is explicit
- provenance and decision reasons are preserved
- default research workflows use trusted tiers
- weaker rows remain available for exploration/review
- storage budgets remain protected

This task should turn that product decision into an implementation-ready policy
and a narrow code/docs update.

## Product Decision

Argus should use tiered retention:

- `A`: strong evidence, default for summaries and high-confidence answers
- `B`: normal retained evidence, usable in research with evidence backing
- `C`: weak/borderline evidence, stored as review/exploration pool when storage
  budget allows
- `D`: discard

The product should not treat `C` rows as equally trusted evidence. `C` rows are
for exploration, recall recovery, manual review, and future training data.

## Required Outcomes

1. Update the roadmap/product documentation to reflect tiered retention as the
   approved near-term direction.
2. Define default research-query behavior for tiers:
   - default answers use `A/B`
   - exploratory/review workflows may include `C`
   - `D` remains excluded
3. Define durable-ingest policy for `C` rows, including storage/yield guardrails.
4. Audit current commit/query/export paths to identify where binary
   `retain/evaluate/discard` assumptions exist.
5. Implement the smallest code/config change needed to support tier-aware
   retention in the existing pipeline, or write `CHANGE_REQUEST.md` if the
   needed change is broader than this task.
6. Add focused tests for any changed behavior.
7. Publish an implementation report with the exact policy and verification
   commands.

## Constraints

- Work on branch `agent/tiered-retention-policy`.
- Do not access frozen shards `comments-2021-01-002` or
  `comments-2021-01-003`.
- Do not run full-month durable ingestion.
- Do not mutate durable DuckDB except through tests that use temporary DBs.
- Do not add new models, LLM calls, embeddings, vector DBs, or dependencies.
- Do not change labels.
- Do not lower quality gates silently; document the new interpretation of gates.
- Do not make unrelated frontend/product polish changes.

## Acceptance Criteria

- Tiered retention policy is documented clearly.
- Existing v4 / learned-retrieval results are reinterpreted consistently under
  the new policy.
- Query/default behavior is explicit: trusted research uses `A/B`, exploratory
  review can opt into `C`.
- Code changes, if any, are small and covered by tests.
- No frozen validation shard was accessed.
- `go test ./...`, relevant focused tests, Python compilation, and
  `git diff --check` pass.
- `IMPLEMENTATION_REPORT.md` is complete.

## Deliverables

- Updated roadmap/product/runbook docs
- Any required config/code updates for tier-aware retention
- Focused tests
- `.agent/tasks/2026-06-18-tiered-retention-policy/IMPLEMENTATION_REPORT.md`
