# Phase 3 Cleaning Rules

This document records the first explicit cleaning policy for Argus.

## Current Policy

### Author Retention

- preserve `author` as-is in the clean layer
- revisit hashing or alternate retention only after research requirements are clearer

### Comments

- preserve raw `body`
- write normalized `body_clean`
- flag `is_deleted` when trimmed comment text equals `[deleted]`
- flag `is_removed` when trimmed comment text equals `[removed]`
- flag `is_bot_like` when:
  - `author = 'AutoModerator'`
  - author name contains `bot`
  - body begins with common automation phrases such as `I am a bot` or `This action was performed automatically`

### Submissions

- preserve raw `title` and `selftext`
- write normalized `title_clean` and `selftext_clean`
- write `combined_text` as `title_clean`, `selftext_clean`, or both separated by a blank line
- flag `is_deleted` when title or selftext equals `[deleted]`
- flag `is_removed` when title or selftext equals `[removed]`
- flag `is_bot_like` using the same author and boilerplate heuristics as comments

### Shared Rules

- do not overwrite raw text
- collapse repeated whitespace in cleaned text
- keep provenance fields from raw data
- add `cleaned_at` and `clean_run_id`
- store `raw_id` for raw-to-clean lineage
- keep `raw_duplicate_count` so duplicate raw ids stay visible to downstream QA
- when duplicate raw ids exist, keep the newest `ingested_at` row and break ties by `source_file`
- compute `text_length` from the cleaned analytical text field
- leave `language` as `NULL` for now

## Notes

- these rules are intentionally simple and reversible
- suspicious content is annotated, not discarded
- later phases can tune bot heuristics against manual review samples
