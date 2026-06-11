#!/usr/bin/env python3
import argparse
import json
import sys
from datetime import datetime, timezone
from pathlib import Path


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--input-url", action="append", required=True)
    parser.add_argument("--output-path", required=True)
    parser.add_argument("--record-type", required=True)
    parser.add_argument("--manifest-id", required=True)
    parser.add_argument("--source-path", required=True)
    parser.add_argument("--subreddits", required=True)
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

    output_path = Path(args.output_path)
    output_path.parent.mkdir(parents=True, exist_ok=True)

    con = duckdb.connect()
    con.execute("INSTALL httpfs;")
    con.execute("LOAD httpfs;")

    subreddit_sql = ",".join("'" + item.replace("'", "''") + "'" for item in subreddits)
    manifest_id_sql = args.manifest_id.replace("'", "''")
    output_sql = str(output_path).replace("'", "''")
    source_path_sql = args.source_path.replace("'", "''")
    input_urls_sql = ",".join("'" + item.replace("'", "''") + "'" for item in args.input_url)
    parquet_source = f"[{input_urls_sql}]"

    try:
        copy_query = f"""
            COPY (
                SELECT
                    * EXCLUDE (filename),
                    filename AS source_file,
                    TIMESTAMP '{datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M:%S")}' AS ingested_at,
                    '{manifest_id_sql}' AS manifest_id
                FROM read_parquet({parquet_source}, filename=true)
                WHERE lower(subreddit) IN ({subreddit_sql})
            )
            TO '{output_sql}'
            (FORMAT PARQUET, COMPRESSION ZSTD)
        """
        con.execute(copy_query)

        bytes_written = output_path.stat().st_size if output_path.exists() else 0
        if bytes_written == 0:
            print(
                json.dumps(
                    {
                        "status": "completed_zero_rows",
                        "rows_written": 0,
                        "bytes_written": 0,
                        "output_path": "",
                        "source_path": args.source_path,
                    }
                )
            )
            return 0

        rows_written = int(con.execute(f"SELECT count(*) FROM read_parquet('{output_sql}')").fetchone()[0])
        if rows_written == 0 and output_path.exists():
            output_path.unlink()
            bytes_written = 0

        print(
            json.dumps(
                {
                    "status": "completed" if rows_written > 0 else "completed_zero_rows",
                    "rows_written": rows_written,
                    "bytes_written": bytes_written,
                    "output_path": str(output_path) if rows_written > 0 else "",
                    "source_path": source_path_sql,
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
                "rows_written": 0,
                "bytes_written": 0,
                "output_path": "",
                "source_path": "",
                "error": message,
            }
        )
    )


if __name__ == "__main__":
    sys.exit(main())
