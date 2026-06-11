# Argus Implementation Guide

> Document Version: 1.0.0
> Last Updated: 2026-06-11
> Companion Document: `SPEC.md`
> Intended Audience: humans and coding agents executing Argus phase by phase

## 1. Purpose

This document translates the Argus spec into an execution-grade implementation plan.

It exists to answer:

- what to build first
- what to avoid building too early
- what files and artifacts each phase must produce
- how to verify each phase is complete
- what metrics allow us to move to the next phase

This guide is intentionally detailed. Agents should treat it as the operational source of truth for implementation order and definition of done.

## 2. Execution Principles

All phases must follow these principles.

### 2.1 Local-First

- Build for one-machine execution first.
- Assume no paid SaaS is required for phase 0 through phase 4.
- Prefer local files, deterministic scripts, and portable datasets over hosted infrastructure.

### 2.2 Narrow-First

- Do not build for full Reddit history first.
- Do not build for all domains first.
- Do not build a polished product UI first.
- Always prove one bounded research workflow before broadening scope.

### 2.3 Evidence-First

- Every research output must preserve links back to raw evidence.
- Summaries without source rows are not sufficient.
- Derived signals must always be explainable through rules, labels, or reproducible transformations.

### 2.4 Rebuildable-By-Design

- Raw data should be reproducible from source manifests.
- Cleaned data should be reproducible from raw data.
- Research marts should be reproducible from cleaned data.
- Temporary data may be deleted if it can be regenerated cheaply.

### 2.5 Phase Gates

- Do not start a later phase because it feels interesting.
- Advance only when the current phase has produced its required artifacts and passed its verification checks.

## 3. Recommended Local Baseline

This guide assumes the current execution target is a local machine with roughly:

- Apple Silicon laptop or desktop
- 16 GB RAM minimum
- about 100 GB free SSD space available before serious pilot ingest

With the current machine state, local implementation is feasible for:

- phase 0 through phase 4
- a bounded pilot dataset
- DuckDB-driven research workflows

Broad historical backfill is explicitly out of scope until later.

## 4. Target Repository Layout

Agents should converge on the following structure unless a better layout emerges for a concrete reason.

```text
argus/
  SPEC.md
  IMPLEMENTATION_GUIDE.md
  README.md
  .gitignore
  docs/
    decisions/
    research/
    runbooks/
  configs/
    domains/
    pipelines/
    signals/
  manifests/
    archive/
    pilot/
  scripts/
    dev/
    qa/
  sql/
    discovery/
    staging/
    marts/
    checks/
  cmd/
    argus/
    manifest-builder/
    ingest-worker/
    clean-transform/
    enrich-signals/
  internal/
    archive/
    manifest/
    storage/
    checkpoint/
    reddit/
    clean/
    signals/
    entities/
    metrics/
    export/
    qa/
  notebooks/
  data/
    raw/
    clean/
    marts/
    tmp/
    exports/
  state/
    checkpoints/
    runs/
    logs/
  testdata/
    samples/
    fixtures/
```

## 5. Global Conventions

These conventions apply to every phase.

### 5.1 Run Tracking

Every non-trivial pipeline run should write a machine-readable run record.

Minimum fields:

- `run_id`
- `phase`
- `started_at`
- `finished_at`
- `git_sha` if available
- `config_hash`
- `status`
- `records_seen`
- `records_written`
- `error_count`
- `notes`

Store these under `state/runs/`.

### 5.2 Checkpoints

Long-running steps must checkpoint progress by shard or file.

Minimum checkpoint fields:

- `job_name`
- `source_path`
- `started_at`
- `finished_at`
- `status`
- `attempt_count`
- `rows_written`
- `output_path`

Store these under `state/checkpoints/`.

### 5.3 Data Layer Separation

Never blur the layers.

- `data/raw/`: source-faithful records
- `data/clean/`: normalized and quality-annotated records
- `data/marts/`: research-facing aggregates and signals
- `data/exports/`: outputs prepared for people or downstream tools

### 5.4 SQL Discipline

- Every important analysis query should exist as a checked-in SQL file.
- One-off notebook logic should be promoted into checked-in SQL once it proves useful.
- SQL files should be named by purpose, not by date.

### 5.5 Configuration Discipline

Configuration must be externalized rather than hard-coded.

Important config categories:

- target domains
- subreddit allowlists
- year ranges
- regex dictionaries
- boilerplate filters
- signal rules
- export shapes

### 5.6 Verification Discipline

Every phase must include:

- structural checks
- data quality checks
- sample inspection
- repeatability checks

## 6. Phase Overview

Argus should be implemented in eight phases.

1. Phase 0: Project foundation and execution scaffolding
2. Phase 1: Archive discovery and pilot definition
3. Phase 2: Manifest generation and raw ingest
4. Phase 3: Cleaning, normalization, and quality flags
5. Phase 4: Deterministic research enrichment
6. Phase 5: Research workflows and evidence exports
7. Phase 6: Operational hardening and repeatability
8. Phase 7: Scale-out decision and optional infrastructure upgrade

## 7. Phase 0: Project Foundation And Execution Scaffolding

### Objective

Create a repository that agents can work in safely and repeatedly without inventing structure every time.

### Why This Phase Exists

Without a stable structure, later phases will produce untracked scripts, fragile notebooks, and non-reproducible local state.

### Required Inputs

- `SPEC.md`
- this implementation guide
- local machine with enough disk for a small pilot

### Tasks

1. Initialize the repository for implementation work.
2. Add a top-level `README.md` describing purpose, non-goals, and current phase.
3. Create the directory structure described in section 4.
4. Add a `.gitignore` suitable for:
   - local data files
   - DuckDB database files
   - temporary exports
   - logs
   - build outputs
5. Decide the primary execution style:
   - plain CLI first
   - optional `Makefile` or simple task runner later
6. Create a basic run metadata schema under `docs/runbooks/`.
7. Create placeholder config files for:
   - domain config
   - pipeline config
   - signal config
8. Create placeholder SQL directories with a short README or naming convention note.
9. Create a small `testdata/samples/` area for tiny local fixtures.
10. Write an ADR or decision note explaining:
   - why DuckDB is the default engine
   - why ClickHouse is deferred
   - why local-first is the v1 operating model

### Outputs

- stable repo structure
- repository hygiene files
- phase-aware docs and runbook conventions
- placeholder configs and folders for later work

### Verification Checks

- the repo contains all expected top-level directories
- `.gitignore` excludes local data and build artifacts
- `README.md` explains Argus at a high level
- at least one runbook or decision doc exists
- another agent can infer where to place:
   - SQL
   - configs
   - manifests
   - output data

### Success Metrics

- 100 percent of planned core directories exist
- zero ambiguity about where new files should go
- zero implementation code written outside the intended layout

### Completion Rule

Phase 0 is complete when the repository is structurally ready for execution work and no future phase needs to invent its own folder conventions.

## 8. Phase 1: Archive Discovery And Pilot Definition

### Objective

Understand the external Reddit archive well enough to define a realistic pilot slice.

### Why This Phase Exists

We should not build ingestion assumptions before confirming file layout, schema shape, and likely pilot volume.

### Required Inputs

- DuckDB installed locally
- remote access to the archive source
- phase 0 scaffolding in place

### Tasks

1. Verify the current remote archive path and access method.
2. Confirm whether the archive exposes:
   - submissions
   - comments
   - partitions by year, month, or subreddit
3. Enumerate available Parquet paths or index files.
4. Inspect representative files for both submissions and comments.
5. Capture exact column lists and types observed.
6. Identify column inconsistencies across files if present.
7. Determine which columns are essential for the pilot.
8. Estimate pilot volume for candidate domains.
9. Select the first pilot domain.
10. Select:
    - target subreddits
    - time range
    - record types to include
11. Decide the maximum local pilot size allowed.
12. Write the pilot definition into checked-in config and docs.

### Recommended Pilot Shape

Keep the first pilot intentionally bounded.

Suggested shape:

- one domain
- 3 to 8 subreddits
- 1 to 3 years
- both submissions and comments

Good default domain:

- travel

### Required Artifacts

- `manifests/archive/` discovery outputs
- `configs/domains/<domain>.yaml`
- `docs/research/pilot-definition.md`
- checked-in schema notes
- sample queries under `sql/discovery/`

### Verification Checks

- the archive can be queried remotely from DuckDB
- at least one submissions file and one comments file were sampled successfully
- essential columns are documented
- the pilot domain and scope are frozen in config
- local disk budget is estimated before ingest starts

### Success Metrics

- 100 percent of required pilot columns mapped
- at least 20 sample records manually inspected for each record type
- estimated pilot disk size documented before moving forward
- the selected pilot fits within the local disk budget with safety margin

### Completion Rule

Phase 1 is complete when there is a documented pilot definition, a validated schema understanding, and a justified estimate that the pilot is locally feasible.

## 9. Phase 2: Manifest Generation And Raw Ingest

### Objective

Create a reproducible ingest path from remote archive shards into local raw Parquet datasets.

### Why This Phase Exists

This phase creates the durable raw layer that later phases depend on.

### Required Inputs

- validated pilot definition
- confirmed archive paths
- repository scaffolding

### Tasks

1. Define a manifest schema for ingestable archive units.
2. Build a manifest generator that:
   - discovers archive files matching pilot criteria
   - records file path, type, time partition, and estimated size
   - writes deterministic manifest files
3. Decide raw output layout under `data/raw/`.
4. Build an ingest worker that:
   - reads a manifest
   - pulls remote records
   - writes local Parquet outputs
   - writes checkpoints
   - supports retries
5. Ensure ingest is resumable at the shard level.
6. Preserve source fidelity in raw outputs.
7. Record provenance fields such as:
   - source file
   - ingest timestamp
   - manifest id
8. Build structural validation queries for raw outputs.
9. Run a tiny smoke ingest before the full pilot ingest.
10. Run the full pilot ingest.
11. Measure throughput, disk usage, and failure rate.

### Raw Data Rules

- never overwrite a successful raw shard silently
- never mutate raw text fields during ingest
- never drop columns without documenting why
- quarantine bad files instead of blocking the full run

### Required Artifacts

- manifest schema definition
- manifest generator implementation
- ingest worker implementation
- raw Parquet outputs in `data/raw/`
- checkpoints and run logs
- raw validation SQL in `sql/checks/`

### Verification Checks

- manifests are deterministic across repeated runs with the same config
- smoke ingest produces valid local Parquet
- full pilot ingest can resume after interruption
- output shard counts match manifest expectations
- row counts are non-zero for both submissions and comments
- raw fields required for later phases are present

### Success Metrics

- 100 percent of manifest entries end in either `completed` or `quarantined`
- fewer than 1 percent unrecoverable shard failures in the pilot
- successful resume after a forced interruption test
- raw output row count matches expected ingest count within documented variance

### Completion Rule

Phase 2 is complete when the pilot raw layer exists locally, is resumable, and can be regenerated from manifests without manual improvisation.

## 10. Phase 3: Cleaning, Normalization, And Quality Flags

### Objective

Turn raw Reddit data into a cleaned analytical layer that is queryable, comparable, and quality-aware.

### Why This Phase Exists

Raw Reddit text is too noisy to support reliable market research without normalization and quality annotation.

### Required Inputs

- complete raw pilot dataset
- documented essential columns
- raw validation checks

### Tasks

1. Define cleaning rules for submissions and comments separately.
2. Implement transforms for:
   - timestamp normalization
   - null handling
   - deleted and removed markers
   - bot-like and automoderator heuristics
   - text length features
   - combined submission text fields
3. Decide whether author fields are preserved as-is, hashed, or duplicated into both forms.
4. Create cleaned outputs under `data/clean/`.
5. Preserve source identifiers linking cleaned rows back to raw rows.
6. Implement quality-check SQL for:
   - null rates
   - unexpected empty text
   - invalid timestamps
   - duplicate ids
   - deleted content ratios
7. Build representative sample reports for manual review.
8. Record cleaning assumptions in docs.

### Cleaning Rules That Must Exist

- `[deleted]` and `[removed]` must be flagged explicitly
- text cleanup must not erase the original text
- bot detection must be explainable and reversible
- suspicious records must be annotated, not silently discarded

### Required Artifacts

- cleaning implementation
- clean Parquet outputs in `data/clean/`
- cleaning rule documentation
- data quality SQL and reports

### Verification Checks

- cleaned datasets can be regenerated from raw data only
- raw-to-clean row lineage is preserved
- duplicate id checks pass or are explicitly explained
- timestamps are valid and queryable
- deleted and removed ratios look plausible on manual inspection
- at least 50 cleaned rows are manually spot-checked

### Success Metrics

- 100 percent of cleaned rows retain traceable source ids
- 0 critical schema drift between planned and actual cleaned outputs
- 0 unexplained duplicate key failures for the pilot
- manual QA passes for at least 50 sampled records across both record types

### Completion Rule

Phase 3 is complete when the clean layer is reproducible, traceable, and trusted enough to power research signals.

## 11. Phase 4: Deterministic Research Enrichment

### Objective

Create the first research-facing signal layer without depending on expensive models.

### Why This Phase Exists

Argus must prove it can detect pain points, requests, and workarounds from repeatable logic before introducing model complexity.

### Required Inputs

- cleaned pilot dataset
- target domain definition
- initial phrase heuristics from the spec

### Tasks

1. Define the first signal taxonomy.
2. Start with deterministic signal types:
   - pain point
   - feature request
   - recommendation request
   - workaround
   - comparison
3. Create config-driven phrase dictionaries and regex rules.
4. Build a signal extraction step that emits one row per matched unit.
5. Decide whether the extraction unit is:
   - whole post
   - whole comment
   - sentence
6. Create entity mention extraction for:
   - product names
   - domain terms
   - competitor mentions
7. Create daily or weekly aggregate marts for:
   - signal counts
   - subreddit activity
   - entity frequency
8. Implement false-positive review samples.
9. Tune rules against real pilot examples.
10. Document every rule category and its rationale.

### Important Constraints

- every signal must be reproducible from checked-in rules
- every signal row must point to evidence text
- confidence scores must be interpretable
- no LLM dependency is allowed in this phase

### Required Artifacts

- signal taxonomy config
- regex and dictionary config
- signal extraction implementation
- entity extraction implementation
- research marts in `data/marts/`
- QA samples showing true positives and false positives

### Verification Checks

- each signal type produces non-zero output on the pilot if expected
- each signal row links to source evidence
- at least 30 examples per major signal type are manually reviewed where available
- false positives are documented and rule adjustments are made
- marts can be rebuilt from clean data alone

### Success Metrics

- at least 3 core signal types produce useful results on the pilot
- manual precision for top signal types is acceptable for MVP use
- acceptable for MVP means humans reviewing samples agree the results are directionally useful for research, not merely technically matched
- every emitted signal includes source id, subreddit, timestamp, and evidence text

### Completion Rule

Phase 4 is complete when deterministic enrichment surfaces clearly useful research signals with traceable evidence and tolerable noise.

## 12. Phase 5: Research Workflows And Evidence Exports

### Objective

Turn the cleaned and enriched data into repeatable research workflows that answer real questions.

### Why This Phase Exists

Argus succeeds only if it helps answer research questions, not just if it stores rows correctly.

### Required Inputs

- cleaned pilot dataset
- initial signal layer
- selected target domain

### Tasks

1. Implement a checked-in SQL workflow for pain-point discovery.
2. Implement a checked-in SQL workflow for app-idea discovery.
3. Implement a checked-in SQL workflow for cross-community or competitor analysis.
4. Define export schemas for:
   - evidence CSV
   - evidence JSON
   - summary markdown
5. Build export scripts that collect:
   - representative rows
   - counts and trends
   - supporting metadata
   - direct thread or permalink references where available
6. Ensure every workflow can be run from config rather than notebook-only state.
7. Create research output examples under `docs/research/` or `data/exports/`.
8. Document how a human should interpret each workflow output.

### Minimum Required Workflows

The system must answer at least these three end-to-end:

1. What are the most repeated travel pain points in the pilot dataset?
2. What request-like conversations suggest app opportunities?
3. Which subreddits and entities are most associated with the chosen problem area?

### Required Artifacts

- checked-in SQL workflows
- export scripts or commands
- at least three example evidence bundles
- research interpretation notes

### Verification Checks

- workflows run end-to-end without notebook-only hidden steps
- outputs contain raw evidence, not just counts
- exports are reproducible from config and checked-in queries
- a reviewer can trace every summary claim back to source rows

### Success Metrics

- at least 3 research workflows produce outputs judged useful by human review
- 100 percent of exported findings include supporting evidence rows
- at least 1 output bundle is strong enough to inform an actual founder-style research memo

### Completion Rule

Phase 5 is complete when Argus can answer concrete research questions with repeatable queries and evidence-backed exports.

## 13. Phase 6: Operational Hardening And Repeatability

### Objective

Make the local pipeline dependable enough that future runs do not require constant manual supervision.

### Why This Phase Exists

Useful pipelines fail if only one person remembers the exact sequence of steps.

### Required Inputs

- functioning pilot workflows
- raw, clean, and mart layers
- run logs from earlier phases

### Tasks

1. Standardize CLI entrypoints for the major steps.
2. Ensure every major phase can be run independently.
3. Add dry-run support where practical.
4. Add clear exit codes and structured logs.
5. Add run summaries at the end of each job.
6. Add QA commands that verify:
   - manifests
   - raw layer
   - clean layer
   - marts
7. Add basic automated tests for:
   - manifest logic
   - transform logic
   - signal rule logic
8. Write runbooks for:
   - first setup
   - running the pilot
   - recovering from failed shards
   - rebuilding one layer from another
9. Ensure phase order is documented and scriptable.

### Required Artifacts

- stable CLI or script entrypoints
- structured logs
- automated checks
- runbooks
- test coverage for critical non-trivial logic

### Verification Checks

- a fresh agent can run the pilot from docs
- interrupted jobs can be resumed without manual file surgery
- validation commands catch at least one intentionally injected failure during testing
- logs contain enough context to debug a failed run

### Success Metrics

- pilot can be rerun end-to-end from docs on the same machine
- mean manual intervention during a normal pilot run is near zero
- critical pipeline steps have automated verification commands

### Completion Rule

Phase 6 is complete when Argus is not merely working once, but working repeatably.

## 14. Phase 7: Scale-Out Decision And Optional Infrastructure Upgrade

### Objective

Decide whether the current local-first architecture is sufficient or whether scale-up is justified.

### Why This Phase Exists

Premature infrastructure expansion is one of the main failure modes this project is trying to avoid.

### Required Inputs

- successful pilot workflows
- measured disk usage
- measured runtimes
- measured query latency
- human judgment on research usefulness

### Tasks

1. Collect actual metrics from the completed pilot:
   - ingest runtime
   - disk used by raw, clean, and marts
   - query runtimes for core workflows
   - human usefulness of outputs
2. Identify the current bottleneck:
   - disk
   - RAM
   - scan speed
   - developer time
   - concurrency
3. Decide whether the next step should be:
   - stay local and widen pilot slightly
   - move data to external SSD
   - use an on-demand VM
   - introduce ClickHouse
4. If scale-up is chosen, write an ADR documenting why.
5. Keep the pipeline storage contract portable.

### Decision Rules

Remain on DuckDB plus Parquet if:

- one user is the main operator
- batch research is acceptable
- pilot queries are fast enough
- local or external disk is still manageable

Consider VM only if:

- local disk becomes too tight
- runs are too long to tolerate locally
- the dataset must remain available while the laptop is off

Consider ClickHouse only if:

- query concurrency matters
- materialized serving tables become necessary
- local scans over Parquet are now the binding bottleneck

### Required Artifacts

- scale-up decision memo
- benchmark summary
- optional ADR for VM or warehouse adoption

### Verification Checks

- the decision is based on measured bottlenecks, not instinct
- cost impact is documented before any upgrade
- portability is preserved if infrastructure changes

### Success Metrics

- zero infrastructure upgrades justified only by preference
- every upgrade decision has a measurable bottleneck attached

### Completion Rule

Phase 7 is complete when the next scale step, or the decision to avoid scaling, is justified by real pilot evidence.

## 15. Cross-Phase Success Metrics

The whole Argus implementation should not be considered successful until all of the following are true.

### Technical Metrics

- the pilot ingest is resumable
- raw, clean, and mart layers are rebuildable
- critical pipeline logic is checked into the repo
- core workflows can run without hand-edited notebook state

### Research Metrics

- at least three research questions can be answered end-to-end
- exported outputs include direct supporting evidence
- signal noise is tolerated by human reviewers because results are still useful

### Operational Metrics

- disk usage is measured at each layer
- runtime for each major job is recorded
- failures are visible and recoverable

### Economic Metrics

- phase 0 through phase 5 run without paid SaaS requirements
- scaling decisions are tied to actual need
- the system can be paused without ongoing fixed monthly cost

## 16. Definition Of Done For MVP

Argus MVP is done when all of these are true.

1. The project has a stable repo structure and runbook.
2. A bounded Reddit pilot slice has been discovered and ingested locally.
3. Raw, clean, and mart layers exist and are reproducible.
4. Deterministic signals identify useful pain points and request-like language.
5. At least three research workflows run end-to-end from checked-in logic.
6. Outputs include source evidence that a human reviewer can inspect.
7. The local-first operating model still fits the project's actual needs.

## 17. What Agents Must Not Do

Agents working on Argus should avoid these mistakes.

- Do not expand scope to full-history ingestion before the pilot works.
- Do not introduce ClickHouse because it feels more serious.
- Do not hide important logic inside one notebook.
- Do not produce research summaries with no source evidence.
- Do not create untracked local scripts outside the agreed layout.
- Do not rely on paid APIs for functionality that deterministic logic can handle in MVP.
- Do not mix raw, clean, and derived data in the same directory.

## 18. Suggested First Execution Sequence

When implementation starts, agents should work in this order.

1. Complete phase 0 repository scaffolding.
2. Complete phase 1 archive discovery and freeze the pilot.
3. Build only enough phase 2 ingest logic to smoke test one tiny shard.
4. Finish phase 2 full pilot ingest.
5. Build phase 3 cleaning and quality checks.
6. Build phase 4 deterministic enrichment on the clean layer.
7. Build phase 5 workflows and evidence exports.
8. Harden phase 6 only after the workflows already prove value.

## 19. Final Guidance

Argus will win if execution stays disciplined.

That means:

- small pilot first
- measurable progress
- evidence-backed outputs
- portable local-first tooling
- no premature infrastructure

If a proposed change does not improve one of those five things, it probably belongs later.
