# Relevance Calibration Report

Date: `2026-06-14`

## Scope

This report compares `deterministic_v1` and `deterministic_v2` on the same bounded `comments-2021-01-000` shard.

- source rows: `500,000`
- broad candidates: `10,909`
- labelled candidates: `339`
- labels: `48` travel, `12` SaaS opportunity, `40` app opportunity
- v2-retained rows included in labels: `94 of 94`
- reviewer: Codex agent using `market-research-relevance-v1`

The fixture stores 1,000-character evidence excerpts and source backlinks. It is an engineering calibration set, not independent human ground truth.

## Results

| Candidate metric | v1 | v2 |
| :-- | --: | --: |
| Retained predictions | 73 | 94 |
| True-positive retained | 20 | 80 |
| False-positive retained | 53 | 14 |
| Retained precision | 27.4% | 85.1% |
| Retained recall | 20.0% | 80.0% |
| F1 | 23.1% | 82.5% |

V2 domain results:

| Domain | Precision | Recall |
| :-- | --: | --: |
| Travel | 82.0% | 85.4% |
| SaaS opportunity | 75.0% | 75.0% |
| App opportunity | 88.2% | 75.0% |

Known retained trap leakage fell from `44` categorized v1 rows to `12` v2 rows. Payment-brand `Visa` and promotional/bot traps fell to zero in the labelled set.

## V2 Changes

- contextual boosts reward concrete visa, border, itinerary, workflow, bug, sync, and workaround language
- contextual penalties suppress payment-card `Visa`, adult promotion, game-map `customs`, and political immigration ambiguity
- SaaS and app domains require product or workflow evidence rather than generic pain alone
- every score retains matched rules, context reasons, and penalty reasons

## Lifecycle Validation

A fresh temporary schema-v3 DuckDB completed the full v2 lifecycle:

- retained documents: `94`
- relevance rows: `282`
- signals: `57`
- entities: `173`
- source and staging equations: valid
- commit retry: `skipped_existing`
- staging cleanup: `4,767,743` bytes across two files
- cleanup retry: `skipped_existing`
- retained bot rows: `0`
- missing source URLs: `0`
- durable checksum: `568b9628aea6d2b12ffdf3989c0e31b173f675968a4d4423a524417781cccf7c`

The main `data/argus.duckdb` was intentionally not mutated because it contains the historical v1 calibration batch and the lifecycle correctly blocks duplicate source ingestion.

## Decision

Promote `deterministic_v2` as the default scorer. Proceed with one adjacent bounded shard, not a full month. Before monthly widening, review the new-shard yield and independently spot-check a small sample of the agent-reviewed labels.
