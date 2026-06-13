# Argus Local Market Research Product Plan

## Status

Approved implementation direction.

Date: `2026-06-13`

## 1. Product Goal

Build a useful local research product that helps answer:

- What recurring problems are people describing?
- Which problems suggest viable SaaS or app opportunities?
- What tools and workarounds do people use today?
- Where are users dissatisfied with existing products?
- Which travel problems recur across communities?
- What evidence supports each finding?

The product must remain:

- local-first
- evidence-backed
- reproducible
- usable by one primary operator
- bounded to a final durable data footprint of `30 GB`
- able to use up to `70 GB` temporarily during ingestion

## 2. Architecture Decision

The target architecture is:

```text
Remote versioned Parquet archive
        |
        v
Monthly manifest and bounded shard batches
        |
        v
Remote high-recall candidate filter
        |
        v
Temporary raw candidate Parquet
        |
        v
Clean, score, validate, and enrich
        |
        v
Durable argus.duckdb
        |
        +--> search and exploration
        +--> themes and opportunity scoring
        +--> evidence-backed ask
        +--> local web product
```

Storage responsibilities:

- Remote Parquet is the recoverable upstream source.
- Temporary local Parquet is staging and may be deleted after validation.
- `data/argus.duckdb` is the durable local research corpus and query store.
- Manifests, checkpoints, validation reports, and database snapshots preserve reproducibility.

Subreddit is metadata and a relevance feature. It is not an ingest gate.

## 3. Scope

### Initial research domains

1. Travel
2. SaaS and app opportunity discovery

### Initial opportunity signals

- repeated pain points
- feature requests
- recommendation requests
- manual or hacky workarounds
- repeated comparisons
- dissatisfaction with existing products
- willingness to pay or evidence of spending
- fragmented workflows involving multiple tools

### Explicit non-goals for the first usable product

- complete Reddit history
- real-time ingestion
- multi-user serving
- automatic business-plan generation
- unreviewed LLM classification over every archive row
- a separate vector database
- ClickHouse or cloud infrastructure

## 4. Capacity Policy

### Durable storage ceiling

`30 GB` total for product data.

Recommended allocation:

| Area | Target | Hard ceiling |
| :-- | --: | --: |
| Documents and context | 12 GB | 15 GB |
| Relevance, signals, and entities | 4 GB | 5 GB |
| Embeddings and semantic metadata | 5 GB | 6 GB |
| Themes and opportunities | 2 GB | 3 GB |
| Saved research and run metadata | 1 GB | 1 GB |
| Free capacity for growth and compaction | 6 GB | 6 GB |

### Temporary working ceiling

`70 GB` total local use while processing.

Operational thresholds:

- warning at `55 GB`
- stop starting new batches at `62 GB`
- hard abort at `68 GB`
- leave at least `10 GB` of machine disk free outside Argus

### Durable database thresholds

- warning at `21 GB`
- stop widening scope at `24 GB`
- mandatory retention review at `27 GB`
- hard durable-data ceiling at `30 GB`

## 5. Data Retention Policy

### Keep permanently

- `argus.duckdb`
- pinned source manifests
- source revision and shard metadata
- batch checkpoints
- row-count reconciliation reports
- transform, rule, model, and schema versions
- rejected-row counts by reason
- quarantined rows needed for debugging
- database snapshots
- source IDs and Reddit backlinks

### Keep in the durable document table

- original source text
- cleaned analytical text
- source type and source ID
- submission/thread ID
- parent ID for comments
- subreddit
- timestamp
- score and selected engagement metadata
- direct Reddit URL
- remote source file and archive revision
- relevance scores and reasons
- quality flags

### Delete after a batch commits successfully

- downloaded raw candidate Parquet
- temporary clean Parquet
- temporary enrichment files
- DuckDB spill files
- transient model request and response payloads

### Conditions required before deleting raw staging

Every condition must pass:

1. The source archive revision is pinned.
2. Each source shard has stable identity metadata.
3. Candidate rows preserve original text and provenance.
4. `rows_seen = rows_rejected_early + rows_staged`.
5. `rows_staged = rows_retained + rows_rejected_late + rows_quarantined`.
6. Durable writes committed successfully.
7. A post-write read validates the committed batch.
8. The durable batch checksum and row counts are recorded.
9. The checkpoint is marked complete only after validation.
10. The batch can be regenerated from the manifest.

Until these controls exist, raw data must not be deleted automatically.

## 6. Durable DuckDB Schema

### `documents`

One row per retained submission or comment.

Core fields:

- `document_id`
- `source_type`
- `source_id`
- `raw_id`
- `thread_id`
- `parent_id`
- `subreddit`
- `author_hash`
- `created_at`
- `score`
- `title`
- `original_text`
- `clean_text`
- `text_length`
- `source_url`
- `archive_repo`
- `archive_revision`
- `source_file`
- `source_shard_size`
- `manifest_id`
- `ingest_batch_id`
- `clean_version`
- `is_deleted`
- `is_removed`
- `is_bot_like`
- `retained_at`

Primary uniqueness contract:

```text
(source_type, source_id)
```

### `document_relevance`

One row per document and relevance model version.

- `document_id`
- `domain`
- `relevance_score`
- `relevance_tier`
- `matched_terms`
- `matched_rules`
- `subreddit_prior`
- `signal_prior`
- `semantic_score`
- `classifier_score`
- `decision`
- `decision_reasons`
- `relevance_version`
- `scored_at`

Initial domains:

- `travel`
- `saas_opportunity`
- `app_opportunity`

### `signals`

- `signal_id`
- `document_id`
- `signal_type`
- `signal_score`
- `matched_pattern`
- `evidence_text`
- `signal_version`
- `created_at`

Signal types:

- `pain_point`
- `feature_request`
- `recommendation_request`
- `workaround`
- `comparison`
- `competitor_dissatisfaction`
- `willingness_to_pay`
- `workflow_fragmentation`

### `entities`

- `entity_mention_id`
- `document_id`
- `entity_type`
- `entity_text`
- `normalized_entity`
- `entity_version`

Entity types include:

- product
- company
- workflow
- travel concept
- location
- airline
- booking platform
- payment tool
- pricing term

### `document_embeddings`

- `document_id`
- `embedding_model`
- `embedding_dimension`
- `embedding`
- `embedded_at`

Only embed retained documents. Do not embed archive rows before relevance filtering.

### `themes`

- `theme_id`
- `domain`
- `theme_name`
- `theme_description`
- `theme_version`
- `document_count`
- `community_count`
- `first_seen_at`
- `last_seen_at`
- `created_at`

### `theme_documents`

- `theme_id`
- `document_id`
- `membership_score`
- `evidence_rank`
- `membership_reason`

### `opportunities`

- `opportunity_id`
- `domain`
- `title`
- `problem_statement`
- `target_user`
- `current_workarounds`
- `existing_products`
- `opportunity_score`
- `evidence_strength`
- `status`
- `generated_at`
- `opportunity_version`

### `opportunity_scores`

Store score components independently:

- recurrence
- severity
- community breadth
- workaround activity
- request intent
- willingness to pay
- competitor dissatisfaction
- recent momentum
- evidence diversity
- contradictory evidence penalty

### `opportunity_evidence`

- `opportunity_id`
- `document_id`
- `evidence_role`
- `evidence_rank`
- `reason`

Evidence roles:

- supports_problem
- supports_demand
- shows_workaround
- shows_spending
- shows_competitor_gap
- contradicts_opportunity

### Operational tables

- `source_manifests`
- `ingest_batches`
- `batch_reconciliation`
- `pipeline_versions`
- `saved_queries`
- `saved_research`
- `ask_runs`

## 7. Candidate Selection Pipeline

Candidate selection must optimize for high recall without retaining all Reddit rows.

### Stage A: Remote projection and cheap filters

Read only required columns from remote Parquet:

- IDs and relationship fields
- subreddit
- timestamps
- score
- title/body/selftext
- selected engagement metadata

Apply broad OR-based candidate rules:

- pain and complaint language
- feature and recommendation request language
- workaround language
- product and tool mentions
- travel vocabulary
- business workflow vocabulary
- pricing and payment language
- explicit app, software, spreadsheet, automation, or manual-process language

Subreddit membership adds recall but is not required.

### Stage B: Local normalization

- deduplicate by source ID
- normalize whitespace
- flag deleted, removed, and bot-like content
- preserve original text
- derive combined submission text
- create direct source URLs

### Stage C: Relevance scoring

Use a staged scorer:

1. deterministic rules
2. keyword and entity density
3. subreddit prior
4. semantic similarity
5. lightweight classifier on uncertain candidates

Initial decision tiers:

- `A`: score `>= 0.80`, retain with full enrichment
- `B`: score `0.60-0.79`, retain and review/sample
- `C`: score `0.40-0.59`, temporary evaluation pool
- `D`: score `< 0.40`, discard after metrics are recorded

Thresholds are provisional and must be calibrated from labelled data.

### Stage D: Context expansion

For retained comments:

- retain the linked submission when available
- retain the parent comment when available
- optionally retain one parent chain up to a bounded depth

Context rows may have a lower relevance score, but must record:

```text
retention_reason = context_for:<document_id>
```

### Stage E: Durable commit

Within one DuckDB transaction:

1. insert or update documents
2. insert relevance results
3. insert deterministic signals and entities
4. write batch reconciliation
5. mark the batch validated

Only after the transaction and post-write validation may staging files be removed.

## 8. Monthly Processing Workflow

Process one month at a time, but one bounded shard batch at a time.

Recommended defaults:

- source batch: `256-512 MB`
- DuckDB memory limit: `4-6 GB`
- threads: `4`
- one active ingest worker initially
- no more than `8 GB` of raw staging at once

Workflow:

```text
build pinned monthly manifest
        |
preflight sample and estimate candidate yield
        |
for each bounded shard batch:
    remote candidate scan
    write temporary candidate parquet
    clean and score
    fetch bounded context
    enrich retained candidates
    commit to argus.duckdb
    reconcile and validate
    delete staging
        |
month-end QA and snapshot
        |
approve or stop before next month
```

Every month-end report must include:

- source bytes scanned
- source rows scanned
- early candidate rows
- retained rows by relevance tier and domain
- rejected rows by reason
- context rows added
- candidate precision from manual review
- database growth
- bytes per retained document
- representative query latency
- projected remaining capacity

## 9. Product Workflows

### Ask

Natural-language research with:

- structured query planning
- keyword retrieval
- semantic retrieval
- theme retrieval
- evidence links
- explicit caveats

### Explore

Search and filter by:

- domain
- date
- subreddit
- relevance score
- signal type
- theme
- entity or product
- source type

### Themes

Show:

- concrete normalized problem
- recurrence
- community breadth
- trend over time
- representative evidence
- contradictory evidence
- related products and workarounds

### Opportunities

Show:

- problem statement
- target user
- why the problem matters
- current workarounds
- competitors and gaps
- evidence-backed score
- source links
- confidence and limitations

### Saved Research

Allow users to save:

- questions
- filters
- reports
- selected evidence
- candidate ideas
- notes and decisions

## 10. Implementation Phases

### Phase A: Foundation and source reproducibility

Deliver:

- ADR for durable DuckDB and temporary raw lifecycle
- pinned archive revision in manifests
- source identity fields such as revision, ETag or checksum where available
- database migrations and schema version table
- disk-budget checks

Definition of done:

- the same manifest always identifies the same source files
- the database can be recreated from manifests
- no raw deletion occurs without lifecycle validation

### Phase B: Durable local database

Deliver:

- `data/argus.duckdb`
- schema migration command
- repository layer for durable inserts and reads
- migration of the current V0 clean/mart data into the new schema
- database status and size command
- snapshot/export command

Definition of done:

- current V0 queries and `ask` work against DuckDB tables
- source links and lineage remain intact
- database rebuild and restore are tested

### Phase C: Broad remote candidate scanner

Deliver:

- pipeline config that does not require subreddit filtering
- broad candidate rule config for travel and product opportunities
- bounded remote scan command
- candidate staging output
- batch checkpoints and reconciliation

Definition of done:

- one archive shard can be scanned broadly
- retained candidates include relevant rows outside the current subreddit list
- retrying a completed batch creates no duplicates

### Phase D: Relevance scoring and retention

Deliver:

- deterministic relevance feature extraction
- domain-specific scores and reasons
- labelled evaluation fixture
- uncertain-candidate sampling command
- configurable retention thresholds

Definition of done:

- at least `300` candidates are manually labelled
- retained candidate precision is at least `70%`
- estimated recall is measured on the labelled sample
- every retention decision is explainable

### Phase E: Transactional batch lifecycle

Deliver:

- durable transactional batch writes
- row reconciliation
- automated staging cleanup
- forced-interruption recovery test
- month-end report

Definition of done:

- a batch can fail after staging and resume cleanly
- staging is deleted only after durable validation
- no source row is silently lost between counters

### Phase F: First broad monthly ingest

Recommended month:

```text
2021-02
```

Reason:

- current V0 data overlaps it for comparison
- published source size is known
- February is slightly smaller than January and March

Execution order:

1. scan a 1% shard sample
2. review candidate quality
3. scan 10% of the month
4. recalibrate thresholds
5. complete the month
6. publish capacity and quality report

Definition of done:

- full month completes under the temporary disk ceiling
- final retained corpus growth is measured
- product research queries find useful evidence outside curated subreddits
- the month is approved before another month begins

### Phase G: Semantic retrieval and theme mining

Deliver:

- embeddings for retained documents only
- similarity search inside DuckDB
- theme candidate generation
- LLM-assisted theme naming and description
- theme evidence and contradiction review

Definition of done:

- semantic retrieval improves a fixed evaluation set
- themes use concrete issue language rather than regex labels
- every theme links to multiple source documents where available

### Phase H: Opportunity engine

Deliver:

- opportunity scoring model
- opportunity generation from themes
- supporting and contradictory evidence
- travel-specific and general app-idea views
- evaluation rubric for idea usefulness

Definition of done:

- at least `20` generated opportunities are manually reviewed
- at least `5` are judged worthy of deeper founder research
- no opportunity appears without linked evidence

### Phase I: Local web application

Recommended stack:

- existing Go code for pipeline and API
- server-rendered HTML or a small React frontend only if interaction requires it
- local-only HTTP server
- DuckDB opened read-mostly by the app
- write operations serialized

Deliver:

- dashboard
- ask
- explore
- themes
- opportunities
- saved research
- ingestion status and storage budget

Definition of done:

- a user can move from question to evidence to saved opportunity without the terminal
- all displayed claims link back to source evidence

## 11. Quality and Evaluation

Create a fixed evaluation set containing:

- `50` travel questions
- `50` SaaS/app opportunity questions
- expected retrieval concepts
- expected relevant source rows where practical
- known false-positive traps

Track:

- candidate precision
- candidate recall estimate
- evidence precision at top 5 and top 10
- theme coherence
- unsupported-claim rate
- source-link coverage
- ask-answer usefulness
- query latency

Minimum launch gates:

- source-link coverage: `100%` for cited claims
- unsupported-claim rate: below `5%` in manual review
- top-10 evidence precision: at least `70%`
- median common query latency: below `3 seconds`
- ask answer latency excluding LLM: below `5 seconds`

## 12. Security and Privacy

- keep API keys only in ignored environment files
- hash author names in durable analytical tables by default
- store the author-hash salt outside the database in an ignored environment file
- retain original author only if a documented research need exists
- never expose local DuckDB over a public network
- treat source text as potentially sensitive user-generated content
- support deletion by source ID
- document archive licensing and permitted use before broad backfill

## 13. Backup and Recovery

At the end of each accepted month:

1. checkpoint and close active writers
2. run database integrity checks
3. export essential tables to versioned Parquet
4. create a compressed DuckDB snapshot
5. record database size and checksum
6. retain the latest two snapshots

Backups count toward machine storage but not the `30 GB` active database ceiling. They should preferably live on external storage.

## 14. Recommended Build Order

Do not begin the full monthly scan until steps 1-5 are complete:

1. source revision pinning
2. durable schema and migrations
3. storage budget monitor
4. candidate scanner
5. batch reconciliation and cleanup
6. labelled relevance evaluation
7. 1% broad sample
8. 10% broad sample
9. first complete month
10. semantic theme mining
11. opportunity scoring
12. local web application

## 15. Immediate Implementation Backlog

The first implementation sequence should be:

1. Add ADR-002 and database path configuration.
2. Add `cmd/db-migrate`.
3. Add `cmd/db-status`.
4. Extend manifests with archive revision and source identity.
5. Add broad candidate rule configuration.
6. Add `cmd/scan-candidates`.
7. Add staging and reconciliation schemas.
8. Add `cmd/commit-candidates`.
9. Add post-commit validation and cleanup.
10. Run a one-shard sample and publish the first yield report.

This sequence proves the storage lifecycle before spending time on the web product or a large ingest.
