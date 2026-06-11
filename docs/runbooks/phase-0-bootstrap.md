# Phase 0 Bootstrap Runbook

This runbook describes the minimum local setup needed before Phase 2 implementation begins.

## Verified Local Tooling

Verified on 2026-06-11:

- `go version go1.25.3 darwin/arm64`
- `Python 3.9.6`
- `git version 2.39.5`

DuckDB CLI was not installed, but DuckDB Python package was installed successfully:

```bash
python3 -m pip install --user duckdb
```

## Recommended Local Bootstrap

1. Ensure Python and Go are on `PATH`.
2. Install DuckDB Python package if missing.
3. Confirm roughly 100 GB free disk before pilot ingest.
4. Work from the repo root.

## Validation Commands

```bash
python3 -m site --user-site
python3 -c "import duckdb; print(duckdb.__version__)"
go version
git status
df -h ~
```

## Important Note About DuckDB Extensions

DuckDB's `httpfs` extension must be available for Hugging Face remote Parquet access.

Validated locally with:

```python
import duckdb
con = duckdb.connect()
con.execute("INSTALL httpfs;")
con.execute("LOAD httpfs;")
```

If the local environment blocks extension caching under the normal home directory, set `HOME` to a writable project directory for the validation session.
