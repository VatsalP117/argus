# Pilot Definition: Travel Q1 2021

## Decision

The first Argus pilot will target the travel domain using:

- record types:
  - comments
  - submissions
- target months:
  - 2021-01
  - 2021-02
  - 2021-03
- smoke month:
  - 2021-01 only
- selected subreddits:
  - `travel`
  - `solotravel`
  - `shoestring`
  - `onebag`
  - `digitalnomad`
  - `travelhacks`

## Why This Pilot

This slice is small enough to stay local-first while still being rich enough for:

- pain-point discovery
- request-like language detection
- workaround discovery
- evidence export

It is also aligned with the first recommended domain in the spec: travel.

## Why Q1 2021

Q1 2021 was chosen because:

- comments and submissions are both clearly available in the validated archive structure
- monthly stats are available and consistent
- it is recent enough to be useful but old enough to avoid archive uncertainty around newer comment paths
- it lets the next agent begin with a single smoke month before widening to the full quarter

## Pilot Execution Strategy

The first execution should not ingest the whole quarter at once.

Recommended order:

1. smoke ingest `2021-01`
2. validate row counts, disk growth, and queryability
3. widen to `2021-02`
4. widen to `2021-03`

## Published Archive Scale For These Months

Across all Reddit, not yet filtered to the travel pilot:

- `2021-01`
  - comments: `210,496,207` rows, `17,386,136,706` bytes parquet
  - submissions: `32,704,571` rows, `3,324,589,156` bytes parquet
- `2021-02`
  - comments: `193,510,365` rows, `16,040,145,136` bytes parquet
  - submissions: `31,147,947` rows, `3,196,374,118` bytes parquet
- `2021-03`
  - comments: `207,454,415` rows, `17,479,835,440` bytes parquet
  - submissions: `33,006,103` rows, `3,439,770,881` bytes parquet

These are full-Reddit month totals. The actual travel-filtered pilot should be much smaller, but the next agent should still enforce safety caps during Phase 2.

## Safety Limits For Phase 2

Until measured data proves otherwise, the Phase 2 agent should enforce these caps:

- abort or pause if raw local pilot output exceeds `20 GB`
- abort or pause if a single smoke run exceeds `8 GB`
- do not ingest all three months in one run
- checkpoint after every source shard or month-part batch

## Research Questions This Pilot Must Support

1. What are the most repeated travel pain points in the pilot dataset?
2. What request-like travel conversations imply app opportunities?
3. Which travel-related entities, products, or workarounds recur most often?

## Definition Of Success

This pilot is successful if:

- the smoke month ingests cleanly
- the quarter can be expanded incrementally
- research workflows produce evidence-backed outputs
- the local-first stack remains comfortable on the current machine
