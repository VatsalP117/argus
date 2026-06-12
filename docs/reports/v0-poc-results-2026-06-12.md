# V0 POC Results: Travel Jan-Feb 2021

Date: `2026-06-12`

Status: V0 POC complete on the frozen Jan-Feb 2021 travel slice.

## Scope

- domain: `travel`
- months:
  - `2021-01`
  - `2021-02`
- record types:
  - comments
  - submissions
- selected subreddits:
  - `travel`
  - `solotravel`
  - `shoestring`
  - `onebag`
  - `digitalnomad`
  - `travelhacks`

## Data Footprint

Raw rows:

- comments `2021-01`: `14,795`
- comments `2021-02`: `38,343`
- submissions `2021-01`: `6,136`
- submissions `2021-02`: `6,938`

Clean rows:

- comments `2021-01`: `14,795`
- comments `2021-02`: `38,343`
- submissions `2021-01`: `6,019`
- submissions `2021-02`: `6,938`

Derived marts:

- research signals: `200`
- entity mentions: `5,932`
- subreddit daily metrics rows: `354`

Signal mix:

- comparison: `100`
- pain_point: `73`
- recommendation_request: `21`
- feature_request: `5`
- workaround: `1`

Entity mix:

- domain_term: `3,967`
- booking_platform: `966`
- airline: `536`
- payment_tool: `254`
- product: `209`

## Research Questions Answered

### 1. What pain points repeat most often?

Top examples from [sql/marts/v0-pain-point-discovery.sql](/Users/vatsalpatel/Desktop/Projects/argus/sql/marts/v0-pain-point-discovery.sql):

- `solotravel | hostel | annoying | 5`
- `travel | visa | difficult to | 5`
- `onebag | airport | annoying | 4`
- `digitalnomad | visa | difficult to | 3`
- `onebag | jetblue | annoying | 3`

Representative evidence:

> "Visas aren't anything special, difficult to obtain..."

### 2. What request-like conversations suggest app opportunities?

Top examples from [sql/marts/v0-app-idea-discovery.sql](/Users/vatsalpatel/Desktop/Projects/argus/sql/marts/v0-app-idea-discovery.sql):

- `travel | recommendation_request | looking for recommendations | 5`
- `digitalnomad | recommendation_request | how do you manage | 4`
- `onebag | feature_request | i wish there was | 3`
- `onebag | recommendation_request | looking for recommendations | 3`
- `solotravel | recommendation_request | looking for recommendations | 3`

Representative evidence:

> "Five days Split Croatia -> Athens ... I'm looking for recommendations ..."

### 3. Which entities, products, or workarounds recur most often?

Top examples from [sql/marts/v0-entity-workaround-discovery.sql](/Users/vatsalpatel/Desktop/Projects/argus/sql/marts/v0-entity-workaround-discovery.sql):

- `hostel | domain_term | 931`
- `visa | domain_term | 931`
- `airbnb | booking_platform | 733`
- `airport | domain_term | 660`
- `booking | domain_term | 401`
- `united | airline | 234`
- `wise | payment_tool | 166`

Observed workaround coverage:

- `i ended up using | 1`

## Validation Status

The current frozen-slice run passed [sql/checks/phase-4-signal-validation.sql](/Users/vatsalpatel/Desktop/Projects/argus/sql/checks/phase-4-signal-validation.sql):

- major signal types are non-zero
- no missing `source_id`, `subreddit`, `evidence_text`, or `signal_run_id`
- entity mentions are non-zero across all configured entity classes
- daily metrics rows exist for both months

## One-Command Repro

Run:

```bash
scripts/dev/run_v0_poc.sh
```

This rebuilds the current marts, runs validation, executes the three research SQL workflows, and writes a timestamped output bundle under `data/exports/poc-run-*/`.

Verified bundle from this closeout pass:

- [poc-run-20260612T132314Z](</Users/vatsalpatel/Desktop/Projects/argus/data/exports/poc-run-20260612T132314Z>)
- [phase5-export-20260612T132355.025789000Z](</Users/vatsalpatel/Desktop/Projects/argus/data/exports/phase5-export-20260612T132355.025789000Z>)
- [phase5-export-20260612T132355.160289000Z](</Users/vatsalpatel/Desktop/Projects/argus/data/exports/phase5-export-20260612T132355.160289000Z>)

## Important Caveats

- this is a local-first analyst POC, not a polished application
- enrichment is deterministic and phrase-based, not model-based NLP
- the dataset is intentionally frozen to Jan-Feb 2021, not full Q1
- some residual false positives still exist, especially in generic pain-point phrasing
- workaround coverage is still thin compared with pain-point and recommendation coverage
