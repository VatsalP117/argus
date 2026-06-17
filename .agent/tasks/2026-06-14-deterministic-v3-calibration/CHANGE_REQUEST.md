# Change Request

## Summary

The approved bounded calibration work produced a reproducible `deterministic_v3`
candidate, focused tests, and a calibration runner, but the tracked v3 config
still fails the `comments-2021-01-001` recall gate.

Current stopping point:

- `comments-2021-01-000`: pass
- `comments-2021-01-001`: fail
  - exact retained precision: `79.4%`
  - regenerated full-label recall estimate: `27.6%`

## Why I Stopped

Further improvement now appears to require one of these non-trivial changes:

1. More expressive scorer logic than additive term boosts and penalties.
2. A domain-specific retain threshold scheme.
3. Broader candidate/scanner or source-text context changes.

Continuing with more config-only term tweaking risks:

- threshold collapse
- fixture-specific memorization
- obscuring the actual need for a more expressive deterministic rule model

## Recommended Follow-Up

Have the planner review one of these explicit directions before more executor
work:

1. Add a constrained scorer capability for conjunction evidence, such as
   requiring a product/tool anchor plus a failure/workaround context within the
   same domain score.
2. Add phrase-window or proximity-aware matching for travel/app ambiguity rather
   than more global term penalties.
3. Reassess whether the observed training labels include travel/app boundary
   cases that should remain `evaluate` rather than forcing them into `retain`.

## Constraints Preserved

- No access to frozen shards `comments-2021-01-002` or `comments-2021-01-003`
- No CLI default change from v2 to v3
- No durable DuckDB mutation or commit
- No full-month run
- No dependency additions
- No candidate-scanner changes
