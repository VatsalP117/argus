# Technical Specification: Project Argus

> Working Title: Argus
> Document Version: 2.1.0
> Last Updated: 2026-06-11
> Primary Objective: Build a reusable Reddit research platform that makes future market research fast, evidence-backed, and cheap.
> Companion Execution Guide: `IMPLEMENTATION_GUIDE.md`

## 1. Executive Summary

Argus is not just a large-scale Reddit ingestion project. Its actual goal is to make questions like:

- "What app ideas are people repeatedly asking for?"
- "What pain points do travelers complain about?"
- "What workarounds are users stitching together today?"
- "Which communities discuss a problem most intensely, and when?"

easy to answer from historical Reddit data.

The system will ingest public Reddit submissions and comments from the `open-index/arctic` archive into a low-cost research store, normalize and enrich the data, and expose research-friendly query patterns. The long-term value is not the raw dataset itself, but a durable analytics layer that lets us run repeated research workflows without re-scraping, re-cleaning, or re-modeling the data each time.

This is feasible, but only if we treat it as a staged research platform:

1. Validate the archive and schema remotely.
2. Ingest a narrow but useful pilot slice first.
3. Prove research workflows on that slice.
4. Scale ingestion only after the research outputs are genuinely useful.

Trying to fully ingest 12B+ rows before proving research value is technically possible, but strategically risky.

## 2. Product Goal

### Core Outcome

A researcher or founder should be able to ask for:

- pain points in a domain
- repeated requests for tools or products
- complaints about existing products
- unmet needs within a user segment
- changes in discussion volume over time
- cross-subreddit community overlap around a problem

and get back evidence-backed outputs quickly.

### Example Research Jobs To Be Done

- Find startup ideas from repeated user frustrations.
- Discover common travel pain points across travel-related subreddits.
- Compare how users describe the same problem in different communities.
- Identify whether a pain point is persistent, seasonal, or hype-driven.
- Pull representative posts and comments as raw evidence for a thesis.

## 3. Feasibility Assessment

### Short Answer

Yes, this is feasible.

### Why It Is Feasible

- The referenced archive exists and is already exposed as Parquet on Hugging Face.
- DuckDB is a strong fit for remote exploration, pilot-scale storage, and low-cost analysis over Parquet.
- ClickHouse is a strong fit later if query concurrency, materialized views, or broad historical coverage justify the extra operational footprint.
- A staged ingestion pipeline in Go is realistic for a self-hosted setup.

### What Is Not Automatically Solved

Raw OLAP alone does not fully answer research questions like "best app ideas" or "pain points while traveling." Those require a second layer:

- text cleaning
- thread-aware context
- heuristics or classifiers for signals like complaint, request, recommendation, workaround
- optional LLM-based summarization or labeling later

So the original spec was directionally right, but incomplete. A pure SQL pipeline is necessary, but not sufficient.

### Main Risks

- Full-history ingestion may be expensive in time, storage, and reprocessing effort.
- Reddit text is noisy and full of deleted content, bots, memes, sarcasm, and low-signal chatter.
- Research quality depends heavily on filtering, enrichment, and query design, not just storage.
- Historical archives are strong for backtesting and discovery, but weaker for near-real-time monitoring unless a separate freshness pipeline is added later.

## 4. Scope

### In Scope

- Historical Reddit submissions and comments from the archive
- Remote exploration of archive layout and schema
- Incremental ingestion into local Parquet and optional analytical storage later
- Text cleaning and normalization
- Derived research tables for repeated analyses
- Research query layer for themes like pain points, requests, product mentions, and trends
- Internal API or notebook-first access pattern

### Out of Scope For Initial Versions

- Real-time Reddit streaming
- Full UI product before the data model is proven
- Generic web scraping of arbitrary sites
- Full RAG chatbot as the first milestone
- Deep model training or custom foundation models

## 5. Success Criteria

The project is successful when all of the following are true:

1. We can ingest and query a targeted Reddit slice reliably.
2. We can answer at least three concrete research workflows end-to-end.
3. Results include direct evidence, not just summaries.
4. Adding a new domain such as travel, fintech, or productivity requires configuration, not a redesign.
5. Scaling from pilot scope to larger historical coverage is operationally straightforward.

## 6. Primary Research Workflows

These workflows should drive the architecture.

### Workflow A: Pain Point Discovery

Input:

- topic or domain such as travel
- subreddit set and time window

Output:

- repeated complaints
- frequency over time
- representative evidence
- related subreddits and user overlap

### Workflow B: App Idea Discovery

Input:

- domain keywords such as itinerary, visa, airport, booking, expense tracking

Output:

- "someone should build this" style requests
- workaround-heavy conversations
- recurring unmet needs
- comparison against existing products mentioned in-thread

### Workflow C: Market Map

Input:

- product or problem space

Output:

- top communities discussing it
- dominant themes and co-occurring terms
- trend shifts by month or quarter
- likely adjacent niches

### Workflow D: Evidence Export

Input:

- saved query or research thesis

Output:

- raw posts/comments
- metadata
- thread links
- CSV/JSON export for further review

## 7. External Data Assumptions

The current external assumption is the Hugging Face dataset `open-index/arctic`, which is visible as a large Reddit archive with:

- comments subset: 9.52B rows
- submissions subset: 2.33B rows
- Parquet-backed access

This assumption must be revalidated before implementation begins, including:

- exact file layout
- available columns
- partitioning strategy
- licensing and acceptable-use implications
- rate and reliability constraints for large remote reads

The archive should be treated as a public historical source, not as a guarantee of perfect completeness or freshness.

## 8. System Overview

Argus should be designed as a layered pipeline.

### Layer 1: Discovery

Purpose:

- inspect remote schema
- estimate data volume
- identify useful columns
- build target shard manifests

Technology:

- DuckDB with `httpfs`

Outputs:

- validated schemas
- list of candidate parquet paths
- pilot-scope manifest
- rough volume estimates by year, subreddit, and record type

### Layer 2: Raw Ingestion

Purpose:

- move selected archive slices into analytical storage

Technology:

- Go worker-based ingestion service
- Parquet-first landing zone on local or attached SSD
- optional ClickHouse bulk inserts later

Outputs:

- raw submissions dataset
- raw comments dataset
- ingestion logs and checkpoints

### Layer 3: Cleaning And Normalization

Purpose:

- make noisy Reddit text queryable and comparable

Tasks:

- normalize timestamps
- standardize missing values
- flag deleted/removed content
- detect obvious bot or automoderator content
- compute text length and basic quality features
- preserve source identifiers for traceability

Outputs:

- cleaned datasets or fact tables
- quality flags

### Layer 4: Research Enrichment

Purpose:

- convert generic social text into reusable research signals

Tasks:

- phrase and n-gram extraction
- complaint/request/recommendation/workaround classification
- product and domain mention extraction
- thread reconstruction where useful
- optional topic labeling or clustering

Outputs:

- derived marts
- materialized views if a warehouse is later introduced

### Layer 5: Access Layer

Purpose:

- make research repeatable for humans and downstream tools

Possible surfaces:

- saved SQL templates
- thin internal API
- notebook workflows
- later, a small app or agent interface

## 9. Proposed Technology Stack

| Component | Preferred low-cost choice | Role |
| :-- | :-- | :-- |
| Archive exploration | DuckDB | Remote schema inspection, manifest creation, sample analysis |
| Raw and cleaned storage | Parquet on local SSD or external SSD | Cheap, portable storage for pilot and early research |
| Ingestion engine | Go | High-concurrency ETL with checkpointing |
| Query engine | DuckDB | Local analytics, aggregation, and evidence extraction |
| Orchestration | Simple CLI scripts or Docker Compose | Local-first execution without always-on infra |
| Optional enrichment | Go or Python sidecar | Classifiers, topic extraction, offline batch jobs |
| Access layer | SQL templates plus notebooks or lightweight API | Research consumption |
| Optional scale-up store | ClickHouse | Only if historical breadth or concurrent queries justify it |

## 10. Data Model

The original draft schema is too thin for market research. We need both submissions and comments, plus enough metadata to reconstruct useful context.

### 10.1 Core Tables

#### `reddit_submissions_raw`

Suggested columns:

- `id String`
- `subreddit LowCardinality(String)`
- `author String`
- `title String`
- `selftext String`
- `url String`
- `domain LowCardinality(String)`
- `score Int32`
- `num_comments Int32`
- `created_utc DateTime`
- `is_self UInt8`
- `over_18 UInt8`
- `permalink String`
- `retrieved_at Nullable(DateTime)`
- `ingested_at DateTime DEFAULT now()`
- `source_file String`

#### `reddit_comments_raw`

Suggested columns:

- `id String`
- `subreddit LowCardinality(String)`
- `author String`
- `body String`
- `score Int32`
- `created_utc DateTime`
- `link_id String`
- `parent_id String`
- `distinguished Nullable(LowCardinality(String))`
- `author_flair_text Nullable(String)`
- `permalink Nullable(String)`
- `retrieved_at Nullable(DateTime)`
- `ingested_at DateTime DEFAULT now()`
- `source_file String`

### 10.2 Cleaned Tables

#### `reddit_submissions_clean`

Additional derived columns:

- `title_clean String`
- `selftext_clean String`
- `combined_text String`
- `is_deleted UInt8`
- `is_removed UInt8`
- `is_bot_like UInt8`
- `text_length UInt32`
- `language Nullable(LowCardinality(String))`

#### `reddit_comments_clean`

Additional derived columns:

- `body_clean String`
- `is_deleted UInt8`
- `is_removed UInt8`
- `is_bot_like UInt8`
- `text_length UInt32`
- `language Nullable(LowCardinality(String))`

### 10.3 Research Tables

#### `research_signals`

One row per classified text unit.

Suggested fields:

- `source_type Enum('submission','comment')`
- `source_id String`
- `subreddit LowCardinality(String)`
- `created_utc DateTime`
- `signal_type LowCardinality(String)`
- `signal_score Float32`
- `evidence_text String`
- `topic_hint Nullable(String)`

Examples of `signal_type`:

- `pain_point`
- `feature_request`
- `recommendation_request`
- `workaround`
- `comparison`
- `willingness_to_pay`

#### `entity_mentions`

Suggested fields:

- `source_type Enum('submission','comment')`
- `source_id String`
- `subreddit LowCardinality(String)`
- `created_utc DateTime`
- `entity_text String`
- `entity_type LowCardinality(String)`
- `normalized_entity String`

#### `subreddit_metrics_daily`

Suggested fields:

- `day Date`
- `subreddit LowCardinality(String)`
- `submission_count UInt64`
- `comment_count UInt64`
- `unique_authors UInt64`
- `median_score Float32`
- `pain_point_count UInt64`
- `feature_request_count UInt64`

## 11. Storage And Table Design

### Parquet Layout Recommendation

- partition by record type, subreddit, and year or month where practical
- keep shard manifests so ingestion and reprocessing are resumable
- store raw and cleaned layers separately

### If ClickHouse Is Added Later

For comments:

- `ORDER BY (subreddit, created_utc, id)`

For submissions:

- `ORDER BY (subreddit, created_utc, id)`

### Notes

- Keep text columns, but avoid indexing fantasies. Most performance will come from pruning, batching, and derived marts.
- For the cheap path, favor portable Parquet datasets over warehouse-specific optimizations.
- Consider ClickHouse projections or materialized views only after real query patterns are observed.
- Budget more disk than the raw archive slice suggests because cleaned and derived layers add overhead.

## 12. Ingestion Strategy

The full-history ingest should not be the first milestone.

### Stage 0: Discovery

- inspect archive columns and row counts
- identify partition naming conventions
- estimate high-value subreddits and years
- define the pilot slice

### Stage 1: Pilot Ingest

Recommended pilot:

- one or two domains only
- a bounded year range
- both submissions and comments

Example pilot domains:

- travel
- startup or side-project communities

Purpose:

- validate schema choices
- measure ingest throughput
- measure local disk footprint
- test research workflows

### Stage 2: Domain Expansion

- expand to additional related subreddits
- add more years
- materialize research tables
- keep running on DuckDB if performance remains acceptable

### Stage 3: Optional Warehouse Cutover

- only if DuckDB plus Parquet becomes a bottleneck
- introduce ClickHouse or similar OLAP store
- migrate the highest-value tables first

### Stage 4: Broad Historical Backfill

- only after workflow usefulness is proven
- checkpointed and resumable
- metrics-driven scaling

## 13. Data Cleaning Rules

At minimum, the pipeline should:

- preserve raw text exactly in raw tables
- write cleaned text separately
- flag `[deleted]` and `[removed]` rather than simply dropping them
- maintain author fields, but allow later policy decisions about retention or hashing
- filter obvious automoderator or boilerplate spam with explicit rules
- normalize timestamps to `DateTime`
- keep record provenance via source file and ingest batch metadata

## 14. Research Enrichment Strategy

This is the missing middle layer in the original spec.

### Phase 1 Enrichment: Deterministic

- keyword dictionaries
- regex patterns
- n-grams
- co-occurrence analysis
- heuristics for request and complaint phrases

Example patterns:

- "I wish there was"
- "someone should build"
- "why is there no"
- "what do you use for"
- "I hate when"
- "the worst part about"

### Phase 2 Enrichment: Lightweight Models

- zero-shot or small classifier labeling
- sentence-level theme tagging
- entity normalization for product names

### Phase 3 Enrichment: LLM-Assisted

- cluster summarization
- evidence-backed synthesis
- optional report generation

Important:

LLMs should sit on top of curated evidence, not replace the core data platform.

## 15. Query Surface

The platform should support reusable query templates such as:

### Pain Point Query

- filter by subreddits, time range, and keywords
- return top complaint phrases, trend lines, and example threads

### App Idea Query

- filter for request-like language
- group by repeated noun phrases or entity gaps
- return supporting examples and velocity over time

### Competitor Query

- extract product mentions
- compare sentiment-adjacent complaint/request signals around each product

### Cross-Community Query

- find subreddits discussing similar entities or themes
- measure author overlap and theme overlap

## 16. Operational Requirements

### Deployment

- start local-first
- prefer direct CLI execution or Docker Compose on one machine
- do not require an always-on VM for planning, discovery, or pilot work
- store manifests, checkpoints, and configs in local mounted volumes

### Monitoring

Track at least:

- rows ingested per minute
- failed shard count
- retry counts
- insert latency
- disk growth
- query latency for representative research queries

### Reliability

The ingestion service must be:

- resumable
- idempotent where possible
- checkpointed by source shard and ingest batch
- able to quarantine bad shards without stalling the full run

## 17. Cost And Capacity Planning

The project should be designed so the first meaningful version is cheap to run.

### 17.1 Baseline Principle

Do not optimize for full-history ingestion on day one.

Optimize for:

- low fixed monthly cost
- minimal always-on infrastructure
- ability to pause work without still paying for idle compute
- portable storage that can move from laptop to VM later

### 17.2 What Is Required For The Cheapest Useful Version

Required resources:

- one development machine
- DuckDB
- local SSD space or an external SSD
- the archive source itself

Not required for the MVP:

- managed database subscription
- always-on VM
- Kubernetes
- hosted observability stack
- paid LLM API budget

### 17.3 Recommended Resource Profiles

#### Profile A: Cheapest MVP

Use this for discovery, pilot ingest, and first research workflows.

- compute: existing laptop or desktop
- memory: 16 GB RAM is workable, 32 GB is more comfortable
- storage: 100 GB to 300 GB free SSD space for a bounded pilot, preferably on fast local or external SSD
- services: none required beyond the public dataset source
- software: DuckDB, Go, optional Python

This profile is enough to:

- inspect remote Parquet
- ingest a bounded slice
- build cleaned datasets
- run pain-point and app-idea queries
- export evidence bundles

#### Profile B: Low-Cost Scale-Up

Use this when the pilot works but local disk or runtime becomes annoying.

- one VM, only when actively needed
- suggested starting shape: 8 vCPU, 32 GB RAM
- attached SSD: 500 GB to 1 TB to start
- object storage: optional for archive shards, manifests, and backups

This profile is enough to:

- backfill a larger subreddit set
- run longer ETL jobs without tying up the local machine
- keep a shared research dataset online for occasional use

#### Profile C: Broad Historical Coverage

Use this only after the research outputs are proven valuable.

- 16 to 32 vCPU
- 64 GB RAM or more
- 2 TB or more fast SSD, likely plus backups
- likely ClickHouse or another dedicated OLAP layer
- optional object storage for raw shard retention

This is the first profile where infrastructure cost meaningfully increases.

### 17.4 SaaS And Subscription Guidance

Default answer: none are required.

Optional services only if justified later:

- object storage for cheaper backup and archive retention
- hosted VM provider for longer backfills
- LLM API credits for summarization or labeling
- lightweight monitoring SaaS only if the system becomes multi-user or always-on

### 17.5 Storage Strategy For A Tight Budget

The key cost control is to avoid downloading or materializing more data than the current phase needs.

- remote exploration should happen against the archive directly
- pilot ingests should target selected subreddits and time windows
- raw, cleaned, and derived layers should be separable so old intermediates can be deleted and rebuilt
- if local disk is tight, buy an external SSD before paying for a larger always-on VM

### 17.6 VM Decision Rule

Get a VM only when one of these becomes true:

- the local machine does not have enough disk even for the bounded pilot
- ETL runs are too long to tolerate on the local machine
- the system needs to stay online while the local machine is off
- multiple people or tools need shared access to the same dataset

Until then, local-first is the right default.

## 18. Security, Privacy, And Compliance

Even though this uses public Reddit data, the spec should explicitly account for:

- archive license review
- terms-of-use review for downstream usage
- policy for storing usernames versus hashed identifiers
- deletion and retention strategy for derived exports
- avoiding accidental exposure of raw dumps if a hosted interface is added later

## 19. MVP Recommendation

The best MVP is not "ingest everything."

The best MVP is:

1. Prove one domain-specific research loop.
2. Use a bounded subset of Reddit history.
3. Build enough enrichment to surface pain points and requests.
4. Keep infrastructure local-first and cheap.
5. Export evidence-backed results.

Recommended MVP thesis:

"Can we reliably extract travel pain points and app opportunities from a targeted set of travel-related subreddits over a bounded historical period?"

If that works, the architecture generalizes.

## 20. Implementation Phases

The strategic phase summary below is expanded into an execution-grade playbook in `IMPLEMENTATION_GUIDE.md`.

### Phase 0: Planning And Validation

- validate archive structure and licensing assumptions
- define target research workflows
- define pilot subreddit list and year range
- finalize schema and checkpoint model

### Phase 1: Archive Discovery

- use DuckDB to inspect remote Parquet
- build manifest generator
- sample records and confirm column mapping

### Phase 2: Pilot Ingestion

- ingest pilot submissions and comments into Parquet
- verify throughput, disk usage, and query latency in DuckDB

### Phase 3: Cleaning And Derived Tables

- build cleaned tables
- materialize daily metrics
- create first-pass research signal extraction

### Phase 4: Research Workflows

- implement pain point discovery queries
- implement app idea discovery queries
- produce exportable evidence bundles

### Phase 5: Optional Infrastructure Upgrade

- decide whether DuckDB is still sufficient
- add ClickHouse only if query volume or scale truly requires it
- move workloads to a VM only if local execution is no longer practical

### Phase 6: Scale-Out

- widen subreddit coverage
- widen time range
- optimize storage and ingest concurrency

### Phase 7: Optional AI Layer

- report generation
- semantic clustering
- evidence-backed summaries

## 21. Acceptance Criteria Before Broad Backfill

Do not start full-history ingestion until all of these are true:

1. Pilot ingestion runs reliably without manual babysitting.
2. At least three research queries produce useful outputs.
3. Cleaned and derived schemas feel stable.
4. Storage growth is measured, not guessed.
5. We know which enrichments are worth the cost.
6. We have evidence that a warehouse or always-on VM is actually needed.

## 22. Open Questions To Resolve Before Execution

1. What is the first target domain: travel, startup ideas, or something else?
2. Do we want notebook-first access, API-first access, or both?
3. How much author identity should we retain in the analytical layer?
4. What exact freshness requirement do we have, if any?
5. Are we optimizing for founder research, analyst workflows, or future agentic querying?
6. Which outputs matter most: raw evidence exports, dashboards, saved queries, or natural-language reports?
7. What is the acceptable role of LLMs: optional summarization, core labeling, or neither in v1?

## 23. Bottom Line

This project is feasible and strategically strong if framed correctly.

The winning version of Argus is:

- a Reddit research platform
- powered by a cheap local-first data stack first
- strengthened by enrichment
- validated on a narrow domain first
- scaled only after the research outputs are clearly valuable

The risky version is:

- a giant ingestion project
- without a sharp research surface
- that assumes storage alone creates insight

Argus should optimize for reusable research workflows, not just row count.
