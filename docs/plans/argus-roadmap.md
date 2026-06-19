# Argus Roadmap

## Status

Canonical roadmap for near-term execution and agent handoff.

Date: `2026-06-17`

## 1. Purpose

This document exists to answer four questions for humans and agents:

1. What is Argus trying to become?
2. Where do we stand right now?
3. What should happen next, in order?
4. What signals tell us to keep going, pivot, or stop?

This roadmap is intentionally more execution-oriented than the broader
[local market research product plan](/Users/vatsalpatel/Desktop/Projects/argus/docs/plans/local-market-research-product-plan.md).

Use this document as the default orientation point before opening new
implementation tasks.

## 2. Product North Star

Argus should become a useful local-first research product for one operator that
can ingest bounded archive slices, retain the most relevant evidence, and make
it easy to answer questions such as:

- What recurring pain points are users describing?
- Which problems suggest strong SaaS or app opportunities?
- What workarounds or toolchains are people using today?
- Which travel problems recur across communities and time?
- What evidence supports each conclusion?

The target product is not “all of Reddit in one database.” It is a disciplined,
evidence-backed local corpus that is good enough for market research and idea
discovery.

## 3. Current Position

### What is already true

- Argus is committed to a local-first DuckDB-centered architecture.
- The POC can read Parquet, query curated views, and run an LLM-backed `ask`
  workflow over a guarded query layer.
- Durable database migration and status tooling exist.
- Broad candidate scanning exists and is restart-safe.
- Deterministic relevance scoring, evaluation, and bounded calibration workflows
  exist.
- Evidence export and repeatable research workflows exist for the POC slice.
- The engineering pipeline for planner -> executor -> reviewer work is now
  established and working.

### What is not solved yet

- Retrieval quality is not yet strong enough for wider autonomous ingestion.
- `deterministic_v3` improved precision but still failed recall on the adjacent
  training shard.
- We do not yet have approval to validate on frozen shards `002` and `003`.
- We do not yet have the full month-by-month production ingest loop operating
  over the durable product corpus.
- We should not yet widen sources beyond Reddit until the retention path is more
  trustworthy.

### Honest summary

The platform foundation is real. The current bottleneck is retrieval quality,
not storage, not query plumbing, and not LLM integration.

## 4. Strategic Principles

These principles should govern roadmap decisions:

### Local-first

DuckDB is the durable source of truth. Remote Parquet is the recoverable
upstream source. Temporary staging data is expendable once validation and
reconciliation are complete.

### Evidence-backed

Every answer should link back to source evidence. Argus should prefer auditable
retrieval and explainable transformations over opaque summarization.

### Bounded growth

The product should remain inside the approved storage envelope:

- final durable target: `30 GB`
- temporary working envelope: up to `70 GB`

### Retrieval before scale

Do not automate broad ingestion of weakly filtered data. First prove that the
retrieval layer retains the right evidence.

### Thin vertical slices

Prefer narrow end-to-end progress over broad speculative rewrites.

### Honest gates

If evaluation says a retrieval version failed, record the failure and pivot.
Do not redefine success by changing the metric after the fact.

## 5. Roadmap Shape

The project now breaks into six major tracks:

1. Retrieval quality
2. Durable ingestion and cleanup
3. Corpus expansion
4. Research workflows and query UX
5. Learned retrieval upgrade, if needed
6. Local product polish

These tracks are not equally urgent. Retrieval quality is the active gating
track.

## 6. Active Gate: Retrieval Quality

### Why this is the gate

If relevance retention is poor, then:

- the durable DuckDB corpus fills with weak evidence
- `ask` answers become noisy
- research outputs become less trustworthy
- storage is wasted on low-value rows
- every later workflow gets harder

### Current retrieval state

- `deterministic_v2` calibrated well on the original fixture but failed badly on
  the adjacent shard
- `deterministic_v3` improved trap handling and reporting discipline
- `deterministic_v3` still failed the observed-fixture recall target on
  `comments-2021-01-001`
- `deterministic_v4` improved every metric versus v3 on both observed fixtures
  with reconciled labels, but still missed the `50%` recall gate on
  `comments-2021-01-001` (`36.5%`). This is a deterministic recall ceiling, not
  a label-boundary conflict.
- A bounded learned-retrieval fallback experiment recovered recall but collapsed
  precision and leaked a payment-brand Visa false positive. It is recorded as
  `failed_experiment` and was not promoted.
- conclusion: additive deterministic scoring has reached its recall ceiling on
  the observed fixtures; the product should stop treating retrieval as a single
  binary truth filter.

### Tiered retention policy (approved 2026-06-18)

Because a perfect binary scorer is not reachable right now, Argus uses tiered
retention instead of a strict binary truth filter. The scorer already emits
four tiers and three decisions; the durable and query paths must respect them.

Tier meanings:

- `A`: strong evidence. Default for summaries and high-confidence answers.
- `B`: trusted retained evidence. Usable in research with evidence backing.
- `C`: weak/borderline review or exploration evidence. Stored only with explicit
  opt-in and not trusted by default.
- `D`: discard. Never retained.

Default behavior:

- Durable commit commits only `A`/`B` (`decision = retain`) by default.
- `C` (`decision = evaluate`) may be committed only behind an explicit
  `--include-review-tier` opt-in, as a review/exploration pool.
- Default research/query behavior uses `A`/`B` only. Exploratory or review
  workflows may explicitly opt into `C`.
- `D` is always excluded.

Quality gate interpretation:

- The existing evaluation gates (retained precision `>= 70%`, retained recall,
  zero payment-brand Visa false positives, zero promotion/bot false positives,
  lexical-ambiguity share `<= 20%`) still measure the `decision = retain`
  trusted tier (`A`/`B`). They are not lowered or redefined.
- `C` is not judged by the trusted-tier gate. `C` exists for recall recovery,
  manual review, and future training data, not for trusted answers.
- This is not "v4 passed." It is "v4 is a useful tiered front door when its
  tiers are respected."

Storage guardrails:

- `C` is opt-in for durable commit until a bounded shard/month trial measures
  storage/yield and query-quality impact.
- The next task must report retained rows by tier, bytes per retained document,
  and the effect of including `C` on query results.

### Immediate next step

The `deterministic_v4` proximity calibration is complete. It added
proximity-aware conjunction rules, recalibrated only on observed fixtures `000`
and `001`, and did not touch frozen shards `002` and `003`.

### Observed outcome

`v4` produced a `mixed_calibration` result (see
`docs/reports/deterministic-v4-calibration-2026-06-18.md`):

- It improved every metric versus v3 on both observed fixtures with reconciled
  labels, with no trap leakage and no precision gate violation.
- It still missed the `001` recall gate (`36.5%` against `50%`). The remaining
  gap is a deterministic recall ceiling, not a label-boundary conflict.
- A bounded learned-retrieval fallback experiment then recovered recall but
  collapsed precision and leaked a payment-brand Visa false positive
  (`docs/reports/learned-retrieval-fallback-2026-06-18.md`). It is recorded as
  `failed_experiment` and was not promoted.

The retrieval decision tree below was resolved into the tiered retention policy
above: deterministic scoring is the tiered front door, not a perfect binary
classifier. Frozen-shard validation remains blocked until a bounded
tiered-retention trial confirms the corpus is useful by tier.

## 7. Retrieval Decision Tree (resolved)

The `v4` outcome has been observed. The tree resolved as follows:

### Branch B/C: deterministic ceiling reached

`v4` did not pass the observed recall gate on `001`, and the learned fallback
failed its hard requirements. This is the deterministic ceiling described by
Branch B/C.

Resolved next focus:

1. freeze deterministic candidate retrieval as the tiered high-recall front door
2. adopt tiered retention (see `Tiered retention policy` above) so `A`/`B` are
   trusted and `C` is review/exploration evidence
3. keep DuckDB and current evaluation tooling; do not promote the failed learned
   layer
4. run a bounded tiered-retention trial before any wider `C`-tier commit or
   frozen-shard validation

### Branch A (not reached)

`v4` did not pass observed calibration cleanly, so the Branch A path (promote
deterministic retrieval for unattended durable ingest, validate on frozen
shards `002` then `003`) was not taken. Frozen-shard validation remains a
gated follow-up after the tiered-retention trial.

## 8. Durable Ingestion Roadmap

This track should accelerate only after retrieval reaches an acceptable bar.

### Goal

Operate an unattended month-by-month bounded ingestion loop that can:

- pin source manifests
- process one shard at a time
- stage candidates
- clean, score, validate, enrich, and commit retained rows
- checkpoint progress
- delete temporary raw and staging files after safe validation
- resume cleanly after interruption

### Current state

Pieces of this already exist:

- durable DuckDB foundation
- migrations and status
- broad candidate scanning
- transactional candidate commit
- cleanup guards
- run metadata and checkpoints

### Remaining work

- prove the whole loop on a small bounded month
- add operator-facing yield reporting
- confirm storage thresholds behave correctly in real runs
- verify restart behavior across interrupted shard sequences
- prove deletion safety after commit and reconciliation

### Definition of done

A fresh checkout plus config and credentials should be able to run a bounded
month ingestion without manual babysitting, aside from reviewing the final
yield/quality report.

## 9. Corpus Expansion Roadmap

Expand only after the bounded ingestion loop is trustworthy.

### First expansion

- Reddit remains the only source
- move beyond narrow subreddit assumptions
- use broad candidate scanning across pinned archive shards
- retain only validated, relevant rows in DuckDB

### Second expansion

Widen Reddit time range and domains:

- travel
- SaaS workflow pain
- app opportunity pain

### Third expansion

Only after the product is useful on Reddit:

- consider additional public discussion or review sources
- define source-specific ingestion and provenance rules
- do not treat all new sources as equivalent to Reddit comments

### Expansion policy

The limiting resource is not raw archive availability. It is retrieval quality
plus durable storage budget. Adding more source volume is only good if retained
evidence quality holds up.

## 10. Query and Research Workflow Roadmap

This track is already partially alive and should keep improving in parallel with
retrieval.

### Short-term goal

Make it easy to answer common research questions repeatably:

- top recurring pain points by topic/domain
- strongest opportunity clusters
- repeated workaround/tool mentions
- pain points by time slice or community
- evidence bundles behind any summary

### Near-term improvements

- richer deterministic marts and saved queries
- consistent backlink support in outputs
- better output formats for “answer + evidence + source excerpts”
- reusable research templates for travel and SaaS/app discovery

### LLM role

LLMs should sit on top of the query layer for:

- query planning
- result synthesis
- evidence-backed answer formatting

LLMs should not be responsible for silently deciding the durable corpus content
row by row during this phase.

## 11. Learned Retrieval Fallback Roadmap

This is the most important likely pivot if deterministic retrieval stalls.

### When to activate it

Activate this track if:

- `v4` still fails materially
- unseen validation fails after observed calibration success
- deterministic rules become too brittle, long, or fixture-shaped

### What “learned retrieval” should mean here

Not a giant system. Start small:

1. keep broad candidate retrieval as the high-recall first pass
2. keep DuckDB as source of truth
3. train or apply a lightweight classifier or reranker using current labelled
   fixtures
4. continue exporting evidence-backed outputs and evaluations through the same
   workflow

### What not to do immediately

- do not jump to a separate vector database
- do not build a fully semantic ingestion system first
- do not let LLMs classify every raw row online
- do not throw away the deterministic evaluation harness

## 12. Local Product Roadmap

Once retrieval and bounded ingestion are stable, the product should become more
operator-friendly.

### Likely milestones

1. local CLI remains the primary operator interface
2. saved research runs become easier to inspect
3. outputs become easier to compare over time
4. a small local web UI may be added later for exploration and evidence review

### UI priority

UI is useful, but not the current bottleneck. Do not build a polished frontend
to compensate for weak retrieval.

## 13. Suggested Execution Order

This is the recommended order unless a later explicit decision changes it:

1. complete `deterministic_v4` proximity calibration task
2. evaluate outcome against the retrieval decision tree
3. if retrieval passes, validate on frozen shards `002` and `003`
4. if unseen validation passes, run one bounded month durable ingest loop
5. inspect storage, yield, and evidence quality
6. expand Reddit corpus month by month
7. strengthen research workflows and answer formats
8. only then consider non-Reddit expansion or local UI polish

If retrieval fails before step `3`, pivot to the learned retrieval roadmap
instead of pushing ahead mechanically.

## 14. Success Metrics By Stage

### Retrieval stage

- observed-fixture calibration gates pass
- unseen validation gates pass
- false-positive traps remain controlled
- retained evidence is visibly useful in research outputs

### Ingestion stage

- restart-safe shard processing works
- cleanup is safe and reproducible
- storage stays within thresholds
- month ingestion completes without manual rescue steps

### Corpus stage

- retained rows show good yield across domains
- evidence quality remains high as coverage widens
- the corpus stays inside the durable budget

### Product stage

- common research questions can be answered in minutes, not hours
- answers consistently include useful evidence backlinks
- the operator can trust the product enough to use it for real idea discovery

## 15. Stop Signs and Pivot Signals

Agents should not blindly continue if these signals appear:

### Retrieval stop signs

- repeated calibration passes still fail adjacent or unseen validation
- scoring config grows quickly without stable gains
- improvements depend on fixture-specific phrases or hidden memorization

### Ingestion stop signs

- retained yield is too low to justify storage
- cleanup safety cannot be proved
- restart behavior is unreliable

### Product stop signs

- answer quality remains poor even when source evidence looks good
- research workflows require too much manual SQL to be worth using

## 16. Agent Operating Guidance

Before starting substantial work, agents should read:

- [README.md](/Users/vatsalpatel/Desktop/Projects/argus/README.md)
- [Argus local market research product plan](/Users/vatsalpatel/Desktop/Projects/argus/docs/plans/local-market-research-product-plan.md)
- this roadmap
- the most recent calibration or validation report relevant to the task

### Planning guidance

- prefer thin execution slices
- explicitly state the gate a task is trying to unblock
- separate “prove the retrieval idea” from “scale ingestion”

### Execution guidance

- do not widen scope when a task is failing
- stop and write `CHANGE_REQUEST.md` when the approved plan no longer matches
  reality
- keep planner, executor, and reviewer artifacts complete

### Review guidance

- review against the task’s stated gate, not generic code beauty
- prioritize correctness, evaluation integrity, and scope discipline

## 17. Current Next Goal

The current next goal is:

tiered retention policy implementation and a bounded tiered-retention trial

Task branch:

- `agent/tiered-retention-policy`

Task artifacts:

- [TASK.md](/Users/vatsalpatel/Desktop/Projects/argus/.agent/tasks/2026-06-18-tiered-retention-policy/TASK.md)
- [PLAN.md](/Users/vatsalpatel/Desktop/Projects/argus/.agent/tasks/2026-06-18-tiered-retention-policy/PLAN.md)

The retrieval gate is now interpreted as a tiered front door (see
`Tiered retention policy` above). The next step after this task is a bounded
shard/month trial of tiered retention that measures storage/yield and
query-quality by tier before any wider `C`-tier commit.

## 18. Canonical Interpretation

If another document seems to conflict with this roadmap:

- architecture direction comes from the ADRs
- product boundaries come from the local market research product plan
- immediate execution order comes from this roadmap
- task-specific details come from the active task artifacts

When in doubt, update this roadmap instead of leaving the project’s direction
implicit in chat history.
