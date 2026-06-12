#!/usr/bin/env python3
import argparse
from pathlib import Path


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--sql-file", required=True)
    return parser.parse_args()


def parse_statements(text: str):
    lines = []
    for raw_line in text.splitlines():
        line = raw_line.strip()
        if not line or line.startswith("--"):
            continue
        lines.append(raw_line)

    cleaned = "\n".join(lines)
    return [stmt.strip() for stmt in cleaned.split(";") if stmt.strip()]


def main():
    args = parse_args()

    try:
        import duckdb
    except ModuleNotFoundError as exc:
        raise SystemExit(f"duckdb python package is not installed: {exc}")

    sql_text = Path(args.sql_file).read_text()
    statements = parse_statements(sql_text)
    con = duckdb.connect()

    for idx, stmt in enumerate(statements, 1):
        print(f"-- statement {idx}")
        rows = con.execute(stmt).fetchall()
        for row in rows:
            print(row)


if __name__ == "__main__":
    main()
