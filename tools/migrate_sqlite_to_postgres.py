#!/usr/bin/env python3
"""
Migrate SQLite DB (simple_trader.db) to Postgres.

Usage:
    python tools/migrate_sqlite_to_postgres.py --sqlite simple_trader.db --pg "postgresql://user:pw@host:5432/dbname"

Notes:
- This is a one-off helper for staging/ops to move the SQLite data to Postgres.
- It tries to preserve id values so foreign key relationships in the new DB match the old DB.
- It reads the Postgres DDL from migrations/001_create_postgres_schema.sql and executes it first.
- Postgres should be reachable from the host running this script and the connecting user should have
  privilege to create schema/tables.
"""

from __future__ import annotations

import argparse
import json
import os
import sqlite3
import sys
from datetime import datetime, timezone
from typing import Any, Dict, Iterable, List, Optional, Tuple

# psycopg2 imports are optional at runtime; we raise a friendly error if missing.
try:
    import psycopg2
    import psycopg2.extras
except Exception as e:
    psycopg2 = None  # type: ignore
    psycopg2_extras = None  # type: ignore

# optional robust timestamp parsing
try:
    from dateutil import parser as dateutil_parser  # type: ignore
except Exception:
    dateutil_parser = None  # type: ignore


def parse_args():
    p = argparse.ArgumentParser(
        description="Migrate simple_trader SQLite DB to Postgres"
    )
    p.add_argument(
        "--sqlite",
        dest="sqlite_path",
        default=os.getenv("SQLITE_DB", "simple_trader.db"),
        help="SQLite DB path (default simple_trader.db)",
    )
    p.add_argument(
        "--pg",
        dest="pg_conn",
        default=os.getenv("PG_CONN"),
        help="Postgres connection string, e.g. postgresql://user:pw@host:5432/dbname (env PG_CONN)",
    )
    p.add_argument(
        "--migrations-sql",
        dest="migrations_sql",
        default=os.getenv(
            "MIGRATIONS_SQL", "migrations/001_create_postgres_schema.sql"
        ),
        help="SQL file with Postgres schema to create",
    )
    p.add_argument(
        "--batch-size",
        dest="batch_size",
        default=1000,
        type=int,
        help="Number of rows to insert per batched command (default 1000)",
    )
    return p.parse_args()


def parse_iso_to_dt(s: Optional[str]) -> Optional[datetime]:
    if s is None:
        return None
    if isinstance(s, (int, float)):
        return datetime.fromtimestamp(int(s), tz=timezone.utc)
    if isinstance(s, datetime):
        return s
    try:
        # Built-in iso parser
        return datetime.fromisoformat(s)
    except Exception:
        if dateutil_parser:
            try:
                return dateutil_parser.parse(s)
            except Exception:
                return None
        return None


def json_parse_or_none(value: Any):
    if value is None:
        return None
    if isinstance(value, (dict, list)):
        return value
    if isinstance(value, str):
        try:
            return json.loads(value)
        except Exception:
            # Not JSON - return the string (or null) - we rely on DB to accept TEXT if needed
            return None
    return None


def migrate_table_bulk(
    src_cur: sqlite3.Cursor,
    dst_cur: "psycopg2.extras.RealDictCursor",
    src_query: str,
    dst_table: str,
    columns: List[str],
    transform_fn=None,
    batch_size: int = 1000,
) -> int:
    """
    Copy rows from SQLite to Postgres using `psycopg2.extras.execute_values` in batches.
    - src_query returns rows in sqlite (cursor with `row_factory = sqlite3.Row`).
    - columns is order preserved list of destination columns to insert.
    - transform_fn(row_dict)->tuple : transforms sqlite row into values for insertion.

    Returns number of rows inserted.
    """
    inserted = 0
    src_cur.execute(src_query)
    rows = src_cur.fetchall()
    if not rows:
        return 0

    # Prepare insert SQL: `INSERT INTO dst_table (col1, col2) VALUES %s`
    cols_sql = ", ".join(columns)
    insert_sql = f"INSERT INTO {dst_table} ({cols_sql}) VALUES %s"
    vectors: List[Tuple] = []

    for r in rows:
        row = {k: r[k] for k in r.keys()}
        if transform_fn:
            vals = transform_fn(row)
        else:
            vals = tuple(row.get(c) for c in columns)
        vectors.append(tuple(vals))
        # flush by batch
        if len(vectors) >= batch_size:
            psycopg2.extras.execute_values(
                dst_cur, insert_sql, vectors, page_size=batch_size
            )
            inserted += len(vectors)
            vectors.clear()
    # final flush
    if vectors:
        psycopg2.extras.execute_values(
            dst_cur, insert_sql, vectors, page_size=batch_size
        )
        inserted += len(vectors)
        vectors.clear()
    return inserted


def main():
    args = parse_args()

    if psycopg2 is None:
        print(
            "Error: psycopg2-binary is required to run this migration. Please `pip install psycopg2-binary`.",
            file=sys.stderr,
        )
        sys.exit(2)

    if not args.pg_conn:
        print(
            "Error: Postgres connection string is required (--pg or PG_CONN env var)",
            file=sys.stderr,
        )
        sys.exit(2)

    if not os.path.exists(args.sqlite_path):
        print(f"Error: SQLite DB not found: {args.sqlite_path}", file=sys.stderr)
        sys.exit(2)

    cur_dir = os.getcwd()
    migrations_path = args.migrations_sql
    if not os.path.exists(migrations_path):
        print(f"Error: migrations SQL not found: {migrations_path}", file=sys.stderr)
        sys.exit(2)

    # Connect to SQLite
    src_conn = sqlite3.connect(args.sqlite_path)
    src_conn.row_factory = sqlite3.Row
    src_cur = src_conn.cursor()

    # Connect to Postgres
    pg_conn = psycopg2.connect(args.pg_conn)
    # Use named record cursors for convenience
    pg_cur = pg_conn.cursor()

    print("Applying Postgres schema...")
    with open(migrations_path, "r", encoding="utf-8") as fh:
        sql = fh.read()
    try:
        pg_cur.execute(sql)
        pg_conn.commit()
    except Exception as e:
        pg_conn.rollback()
        print("Error applying migration SQL:", e, file=sys.stderr)
        raise

    # Helper functions: transforms for each table
    def transform_news(row):
        return (
            row.get("id"),
            row.get("provider"),
            row.get("url"),
            row.get("title"),
            row.get("content"),
            parse_iso_to_dt(row.get("published_at")),
            row.get("asset"),
            row.get("hash"),
            parse_iso_to_dt(row.get("fetched_at")),
            bool(row.get("processed") or 0),
            json_parse_or_none(row.get("raw_json")),
            parse_iso_to_dt(row.get("created_at")),
        )

    def transform_analysis(row):
        return (
            row.get("id"),
            row.get("news_id"),
            row.get("provider"),
            json_parse_or_none(row.get("analysis_json")),
            row.get("confidence"),
            parse_iso_to_dt(row.get("created_at")),
        )

    def transform_market_data(row):
        return (
            row.get("id"),
            row.get("symbol"),
            row.get("timeframe"),
            row.get("start_ts"),
            row.get("open"),
            row.get("high"),
            row.get("low"),
            row.get("close"),
            row.get("volume"),
            parse_iso_to_dt(row.get("created_at")),
        )

    def transform_signals(row):
        return (
            row.get("id"),
            row.get("news_id"),
            row.get("symbol"),
            row.get("side"),
            row.get("entry_price"),
            row.get("stop_loss"),
            row.get("take_profit"),
            row.get("leverage"),
            row.get("position_size"),
            row.get("risk_amount"),
            row.get("rr"),
            row.get("timeframe_hours"),
            row.get("status"),
            json_parse_or_none(row.get("analysis_ids")),
            parse_iso_to_dt(row.get("created_at")),
            parse_iso_to_dt(row.get("expires_at")),
            parse_iso_to_dt(row.get("closed_at")),
            row.get("close_reason"),
            row.get("outcome"),
            row.get("pnl"),
        )

    def transform_trades(row):
        return (
            row.get("id"),
            row.get("signal_id"),
            parse_iso_to_dt(row.get("executed_at")),
            row.get("executed_price"),
            parse_iso_to_dt(row.get("exit_at")),
            row.get("exit_price"),
            row.get("pnl"),
            row.get("outcome"),
            row.get("notes"),
            parse_iso_to_dt(row.get("created_at")),
        )

    def transform_tuning_stats(row):
        return (
            row.get("id"),
            row.get("pattern_name"),
            row.get("symbol"),
            row.get("wins"),
            row.get("losses"),
            row.get("avg_rr"),
            row.get("avg_hold_time_seconds"),
            parse_iso_to_dt(row.get("last_updated")),
        )

    def transform_llm_usage(row):
        return (
            row.get("id"),
            row.get("provider"),
            row.get("request_ts"),
            row.get("request_size"),
            row.get("response_tokens"),
            row.get("estimated_cost"),
            parse_iso_to_dt(row.get("created_at")),
        )

    def transform_runtime_params(row):
        return (
            row.get("key"),
            row.get("value"),
            row.get("description"),
            parse_iso_to_dt(row.get("last_updated")),
        )

    def transform_tuning_history(row):
        return (
            row.get("id"),
            row.get("pattern_name"),
            row.get("symbol"),
            row.get("change"),
            row.get("old_value"),
            row.get("new_value"),
            row.get("notes"),
            row.get("applied_by"),
            parse_iso_to_dt(row.get("created_at")),
        )

    # Table migration order: create parents first
    migration_plan = [
        (
            "news",
            "SELECT * FROM news",
            [
                "id",
                "provider",
                "url",
                "title",
                "content",
                "published_at",
                "asset",
                "hash",
                "fetched_at",
                "processed",
                "raw_json",
                "created_at",
            ],
            transform_news,
        ),
        (
            "analysis",
            "SELECT * FROM analysis",
            ["id", "news_id", "provider", "analysis_json", "confidence", "created_at"],
            transform_analysis,
        ),
        (
            "market_data",
            "SELECT * FROM market_data",
            [
                "id",
                "symbol",
                "timeframe",
                "start_ts",
                "open",
                "high",
                "low",
                "close",
                "volume",
                "created_at",
            ],
            transform_market_data,
        ),
        (
            "signals",
            "SELECT * FROM signals",
            [
                "id",
                "news_id",
                "symbol",
                "side",
                "entry_price",
                "stop_loss",
                "take_profit",
                "leverage",
                "position_size",
                "risk_amount",
                "rr",
                "timeframe_hours",
                "status",
                "analysis_ids",
                "created_at",
                "expires_at",
                "closed_at",
                "close_reason",
                "outcome",
                "pnl",
            ],
            transform_signals,
        ),
        (
            "trades",
            "SELECT * FROM trades",
            [
                "id",
                "signal_id",
                "executed_at",
                "executed_price",
                "exit_at",
                "exit_price",
                "pnl",
                "outcome",
                "notes",
                "created_at",
            ],
            transform_trades,
        ),
        (
            "tuning_stats",
            "SELECT * FROM tuning_stats",
            [
                "id",
                "pattern_name",
                "symbol",
                "wins",
                "losses",
                "avg_rr",
                "avg_hold_time_seconds",
                "last_updated",
            ],
            transform_tuning_stats,
        ),
        (
            "llm_usage",
            "SELECT * FROM llm_usage",
            [
                "id",
                "provider",
                "request_ts",
                "request_size",
                "response_tokens",
                "estimated_cost",
                "created_at",
            ],
            transform_llm_usage,
        ),
        (
            "runtime_params",
            "SELECT * FROM runtime_params",
            ["key", "value", "description", "last_updated"],
            transform_runtime_params,
        ),
        (
            "tuning_history",
            "SELECT * FROM tuning_history",
            [
                "id",
                "pattern_name",
                "symbol",
                "change",
                "old_value",
                "new_value",
                "notes",
                "applied_by",
                "created_at",
            ],
            transform_tuning_history,
        ),
    ]

    # Run the migration
    try:
        for table_name, src_query, cols, transform in migration_plan:
            print(f"Migrating table: {table_name}")
            count = migrate_table_bulk(
                src_cur,
                pg_cur,
                src_query,
                table_name,
                cols,
                transform_fn=transform,
                batch_size=args.batch_size,
            )
            pg_conn.commit()
            print(f"Inserted {count} rows into {table_name}")
    except Exception as e:
        print("Migration failed; rolling back:", e, file=sys.stderr)
        pg_conn.rollback()
        raise
    finally:
        # Close connections
        src_conn.close()

    # Reset sequences for Postgres serial columns
    try:
        reset_seq_tables = [t[0] for t in migration_plan if "id" in t[2]]
        for t in reset_seq_tables:
            try:
                # attempt to set the sequence using pg_get_serial_sequence
                pg_cur.execute(
                    "SELECT setval(pg_get_serial_sequence(%s, 'id'), (SELECT COALESCE(MAX(id), 1) FROM "
                    + t
                    + "), true)",
                    (t,),
                )
            except Exception:
                # fallback if table has no id sequence or naming differs - we skip silently
                pass
        pg_conn.commit()
    except Exception as e:
        print("Error while updating sequences:", e, file=sys.stderr)
        pg_conn.rollback()

    print("Migration completed successfully.")
    pg_cur.close()
    pg_conn.close()


if __name__ == "__main__":
    main()
