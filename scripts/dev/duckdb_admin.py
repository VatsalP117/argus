#!/usr/bin/env python3
import argparse
import hashlib
import json
import re
import sys
from pathlib import Path


MIGRATION_PATTERN = re.compile(r"^(?P<version>\d{3,})_(?P<name>.+)\.sql$")


def parse_args():
    parser = argparse.ArgumentParser()
    subparsers = parser.add_subparsers(dest="command", required=True)

    migrate = subparsers.add_parser("migrate")
    migrate.add_argument("--database-path", required=True)
    migrate.add_argument("--migrations-dir", required=True)

    status = subparsers.add_parser("status")
    status.add_argument("--database-path", required=True)

    return parser.parse_args()


def discover_migrations(migrations_dir: Path):
    migrations = []
    for path in sorted(migrations_dir.glob("*.sql")):
        match = MIGRATION_PATTERN.match(path.name)
        if not match:
            continue
        sql = path.read_text()
        migrations.append(
            {
                "version": int(match.group("version")),
                "name": match.group("name"),
                "path": path,
                "sql": sql,
                "checksum": hashlib.sha256(sql.encode("utf-8")).hexdigest(),
            }
        )

    versions = [migration["version"] for migration in migrations]
    if len(versions) != len(set(versions)):
        raise ValueError("migration versions must be unique")
    return migrations


def migrate(database_path: Path, migrations_dir: Path):
    try:
        import duckdb
    except ModuleNotFoundError as exc:
        raise RuntimeError(f"duckdb python package is not installed: {exc}") from exc

    if not migrations_dir.is_dir():
        raise ValueError(f"migrations directory does not exist: {migrations_dir}")

    database_path.parent.mkdir(parents=True, exist_ok=True)
    migrations = discover_migrations(migrations_dir)
    con = duckdb.connect(str(database_path))
    try:
        con.execute(
            """
            CREATE TABLE IF NOT EXISTS schema_migrations (
                version INTEGER PRIMARY KEY,
                name VARCHAR NOT NULL,
                checksum VARCHAR NOT NULL,
                applied_at TIMESTAMPTZ NOT NULL DEFAULT current_timestamp
            )
            """
        )
        applied = {
            row[0]: row[1]
            for row in con.execute(
                "SELECT version, checksum FROM schema_migrations"
            ).fetchall()
        }
        applied_now = []

        for migration in migrations:
            version = migration["version"]
            if version in applied:
                if applied[version] != migration["checksum"]:
                    raise ValueError(
                        f"migration {version:03d} checksum differs from the applied migration"
                    )
                continue

            con.execute("BEGIN TRANSACTION")
            try:
                con.execute(migration["sql"])
                con.execute(
                    """
                    INSERT INTO schema_migrations (version, name, checksum)
                    VALUES (?, ?, ?)
                    """,
                    [version, migration["name"], migration["checksum"]],
                )
                con.execute("COMMIT")
            except Exception:
                con.execute("ROLLBACK")
                raise
            applied_now.append(version)

        schema_version = con.execute(
            "SELECT coalesce(max(version), 0) FROM schema_migrations"
        ).fetchone()[0]
    finally:
        con.close()

    return {
        "status": "completed",
        "database_path": str(database_path),
        "schema_version": schema_version,
        "applied_migrations": applied_now,
    }


def status(database_path: Path):
    database_exists = database_path.is_file()
    database_size_bytes = database_path.stat().st_size if database_exists else 0
    schema_version = 0

    disk_path = database_path.parent
    while not disk_path.exists() and disk_path != disk_path.parent:
        disk_path = disk_path.parent

    import os

    filesystem = os.statvfs(disk_path)
    free_disk_bytes = filesystem.f_bavail * filesystem.f_frsize

    if database_exists:
        try:
            import duckdb
        except ModuleNotFoundError as exc:
            raise RuntimeError(
                f"duckdb python package is not installed: {exc}"
            ) from exc

        con = duckdb.connect(str(database_path), read_only=True)
        try:
            has_migrations = con.execute(
                """
                SELECT count(*)
                FROM information_schema.tables
                WHERE table_schema = 'main'
                  AND table_name = 'schema_migrations'
                """
            ).fetchone()[0]
            if has_migrations:
                schema_version = con.execute(
                    "SELECT coalesce(max(version), 0) FROM schema_migrations"
                ).fetchone()[0]
        finally:
            con.close()

    return {
        "status": "completed",
        "database_path": str(database_path),
        "database_exists": database_exists,
        "database_size_bytes": database_size_bytes,
        "schema_version": schema_version,
        "free_disk_bytes": free_disk_bytes,
    }


def main():
    args = parse_args()
    try:
        if args.command == "migrate":
            result = migrate(Path(args.database_path), Path(args.migrations_dir))
        elif args.command == "status":
            result = status(Path(args.database_path))
        else:  # pragma: no cover
            raise ValueError(f"unsupported command: {args.command}")
        print(json.dumps(result))
        return 0
    except Exception as exc:
        print(json.dumps({"status": "error", "error": str(exc)}))
        return 1


if __name__ == "__main__":
    sys.exit(main())
