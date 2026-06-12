#!/usr/bin/env python3
import argparse
import json
import sys
import time
from pathlib import Path


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--input-url", action="append", required=True)
    parser.add_argument("--record-type", required=True)
    parser.add_argument("--subreddits", required=True)
    parser.add_argument("--duckdb-memory-limit", default="4GB")
    parser.add_argument("--duckdb-threads", type=int, default=4)
    parser.add_argument("--duckdb-temp-dir", default=".duckdb/tmp")
    return parser.parse_args()


def main():
    args = parse_args()

    try:
        import duckdb
    except ModuleNotFoundError as exc:
        emit_error(f"duckdb python package is not installed: {exc}")
        return 1

    subreddits = [item.strip().lower() for item in args.subreddits.split(",") if item.strip()]
    if not subreddits:
        emit_error("no subreddits provided")
        return 1

    con = duckdb.connect()
    con.execute("INSTALL httpfs;")
    con.execute("LOAD httpfs;")
    con.execute(f"SET memory_limit = '{args.duckdb_memory_limit}'")
    con.execute(f"SET threads = {max(args.duckdb_threads, 1)}")
    temp_dir = Path(args.duckdb_temp_dir)
    temp_dir.mkdir(parents=True, exist_ok=True)
    temp_dir_sql = str(temp_dir).replace("'", "''")
    con.execute(f"SET temp_directory = '{temp_dir_sql}'")

    subreddit_sql = ",".join("'" + item.replace("'", "''") + "'" for item in subreddits)
    input_urls_sql = ",".join("'" + item.replace("'", "''") + "'" for item in args.input_url)
    parquet_source = f"[{input_urls_sql}]"

    try:
        started = time.time()
        query = f"""
            SELECT count(*) AS matched_rows
            FROM read_parquet({parquet_source})
            WHERE lower(subreddit) IN ({subreddit_sql})
        """
        matched_rows = int(con.execute(query).fetchone()[0])
        print(
            json.dumps(
                {
                    "status": "completed",
                    "record_type": args.record_type,
                    "matched_rows": matched_rows,
                    "elapsed_seconds": round(time.time() - started, 3),
                    "source_count": len(args.input_url),
                }
            )
        )
        return 0
    except Exception as exc:  # pragma: no cover
        emit_error(str(exc))
        return 1


def emit_error(message: str):
    print(
        json.dumps(
            {
                "status": "error",
                "matched_rows": 0,
                "elapsed_seconds": 0,
                "error": message,
            }
        )
    )


if __name__ == "__main__":
    sys.exit(main())
