# -*- coding: utf-8 -*-
"""
SQLite DB wrapper and schema for Simple-Trader.
This module provides a small, production-feasible database abstraction around SQLite
that stores news, LLM analyses, generated signals, recorded trades, and tuning stats.

Design goals:
- Keep it dependency-free (standard library only) for maximum portability.
- Thread-safe-ish: use a simple lock around write operations (SQLite supports multiple readers).
- Use JSON text fields for flexible structured data (analysis JSON, LLM usage, etc).
- Make it easy to query recent events and update analytics/tuning data.
"""

from __future__ import annotations

import json
import logging
import sqlite3
import threading
from contextlib import contextmanager
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from typing import Any, Dict, Iterable, List, Optional, Tuple, Union

logger = logging.getLogger("simple_trader.db")
logger.addHandler(logging.NullHandler())
DEFAULT_TENANT_ID = "default"


def now_ts() -> int:
    return int(datetime.now(timezone.utc).timestamp())


def ensure_iso(ts: Optional[int]) -> Optional[str]:
    if ts is None:
        return None
    return datetime.fromtimestamp(ts, tz=timezone.utc).isoformat()


def normalize_tenant_id(tenant_id: Optional[str]) -> str:
    candidate = (tenant_id or DEFAULT_TENANT_ID).strip()
    if not candidate:
        raise DatabaseError("tenant_id must not be empty")
    if len(candidate) > 64:
        raise DatabaseError("tenant_id length must be <= 64")
    if not all(c.isalnum() or c in ("-", "_") for c in candidate):
        raise DatabaseError(
            "tenant_id must contain only letters, numbers, '-' or '_'"
        )
    return candidate


@dataclass
class NewsItem:
    provider: str
    url: str
    title: Optional[str] = None
    content: Optional[str] = None
    published_at: Optional[int] = None
    asset: Optional[str] = None  # e.g., "BTC", "EURUSD"
    raw_json: Optional[Dict[str, Any]] = None
    fetched_at: Optional[int] = None
    hash: Optional[str] = None


@dataclass
class AnalysisItem:
    news_id: int
    provider: str
    analysis_json: Dict[str, Any]
    confidence: Optional[float] = None
    created_at: Optional[int] = None


@dataclass
class Signal:
    news_id: Optional[int]
    symbol: str
    side: str  # 'long' or 'short'
    entry_price: float
    stop_loss: float
    take_profit: float
    leverage: int
    position_size: Optional[float] = None
    risk_amount: Optional[float] = None
    rr: Optional[float] = None
    timeframe_hours: Optional[int] = None
    created_at: Optional[int] = None
    expires_at: Optional[int] = None
    status: str = "open"  # 'open', 'closed', 'cancelled'
    analysis_ids: Optional[List[int]] = None
    # Full system fields
    asset_class: Optional[str] = None
    bucket: Optional[str] = None
    hedge_of: Optional[int] = None
    decision_audit: Optional[str] = None


@dataclass
class TradeRecord:
    signal_id: int
    executed_at: int
    executed_price: float
    exit_at: Optional[int] = None
    exit_price: Optional[float] = None
    pnl: Optional[float] = None
    outcome: Optional[str] = None  # 'win', 'loss', 'timeout'
    notes: Optional[str] = None


@dataclass
class TuningStat:
    pattern_name: str
    symbol: str
    wins: int = 0
    losses: int = 0
    avg_rr: float = 0.0
    avg_hold_time_seconds: float = 0.0
    last_updated: Optional[int] = None


class DatabaseError(RuntimeError):
    pass


class Database:
    """
    Manage a SQLite DB connection and provide high-level methods for the Simple-Trader app.
    The class ensures the schema exists on creation.
    """

    def __init__(
        self, db_path: str = "simple_trader.db", tenant_id: str = DEFAULT_TENANT_ID
    ):
        self.db_path = db_path
        self.tenant_id = normalize_tenant_id(tenant_id)
        self._lock = threading.RLock()
        # permit shared cache and allow connections to be used across threads
        self.conn = sqlite3.connect(self.db_path, check_same_thread=False, timeout=30)
        self.conn.row_factory = sqlite3.Row
        with self._lock:
            self._configure()
            self._create_schema()
            self._migrate_schema_for_tenant_support()
        logger.info("Database initialized: %s", self.db_path)

    def close(self) -> None:
        with self._lock:
            try:
                self.conn.close()
            except Exception:
                logger.exception("Error closing DB connection")

    def _configure(self) -> None:
        # SQLite pragmatic settings for a trading app
        cur = self.conn.cursor()
        try:
            cur.execute("PRAGMA journal_mode=WAL;")
            cur.execute("PRAGMA synchronous = NORMAL;")
            cur.execute("PRAGMA foreign_keys = ON;")
            cur.execute("PRAGMA busy_timeout = 30000;")
            self.conn.commit()
        finally:
            cur.close()

    def _create_schema(self) -> None:
        """
        Create all tables if they don't exist yet.
        Keep columns fairly generic so evolving the schema remains easy.
        """
        cur = self.conn.cursor()
        try:
            cur.executescript(
                """
                BEGIN;

                CREATE TABLE IF NOT EXISTS news (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    tenant_id TEXT NOT NULL DEFAULT 'default',
                    provider TEXT NOT NULL,
                    url TEXT,
                    title TEXT,
                    content TEXT,
                    published_at TEXT,
                    asset TEXT,
                    hash TEXT,
                    fetched_at TEXT,
                    processed INTEGER DEFAULT 0,
                    raw_json TEXT,
                    created_at TEXT DEFAULT (datetime('now')),
                    UNIQUE(tenant_id, hash)
                );

                CREATE INDEX IF NOT EXISTS idx_news_tenant_published ON news (tenant_id, published_at);
                CREATE INDEX IF NOT EXISTS idx_news_tenant_asset ON news(tenant_id, asset);
                CREATE INDEX IF NOT EXISTS idx_news_tenant_processed ON news(tenant_id, processed);

                CREATE TABLE IF NOT EXISTS analysis (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    tenant_id TEXT NOT NULL DEFAULT 'default',
                    news_id INTEGER NOT NULL REFERENCES news(id) ON DELETE CASCADE,
                    provider TEXT NOT NULL,
                    analysis_json TEXT,
                    confidence REAL,
                    created_at TEXT DEFAULT (datetime('now'))
                );

                CREATE INDEX IF NOT EXISTS idx_analysis_tenant_news_id ON analysis(tenant_id, news_id);

                CREATE TABLE IF NOT EXISTS market_data (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    tenant_id TEXT NOT NULL DEFAULT 'default',
                    symbol TEXT NOT NULL,
                    timeframe TEXT NOT NULL,
                    start_ts INTEGER NOT NULL,
                    open REAL, high REAL, low REAL, close REAL, volume REAL,
                    created_at TEXT DEFAULT (datetime('now')),
                    UNIQUE(tenant_id, symbol, timeframe, start_ts)
                );

                CREATE INDEX IF NOT EXISTS idx_market_data_tenant_symbol_time ON market_data(tenant_id, symbol, timeframe, start_ts);

                CREATE TABLE IF NOT EXISTS signals (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    tenant_id TEXT NOT NULL DEFAULT 'default',
                    news_id INTEGER REFERENCES news(id) ON DELETE SET NULL,
                    symbol TEXT NOT NULL,
                    side TEXT NOT NULL,
                    entry_price REAL NOT NULL,
                    stop_loss REAL NOT NULL,
                    take_profit REAL NOT NULL,
                    leverage INTEGER,
                    position_size REAL,
                    risk_amount REAL,
                    rr REAL,
                    timeframe_hours INTEGER,
                    status TEXT DEFAULT 'open',
                    analysis_ids TEXT,
                    created_at TEXT DEFAULT (datetime('now')),
                    expires_at TEXT,
                    closed_at TEXT,
                    close_reason TEXT,
                    outcome TEXT,
                    pnl REAL,
                    -- Full system columns (added for company secret formula)
                    asset_class TEXT,
                    bucket TEXT,           -- CORE or ALPHA
                    hedge_of INTEGER,      -- signal id this hedge protects
                    decision_audit TEXT    -- JSON of why we sized/hedged/rejected
                );

                CREATE INDEX IF NOT EXISTS idx_signals_tenant_status ON signals(tenant_id, status);
                CREATE INDEX IF NOT EXISTS idx_signals_tenant_symbol ON signals(tenant_id, symbol);
                CREATE INDEX IF NOT EXISTS idx_signals_bucket ON signals(tenant_id, bucket);

                CREATE TABLE IF NOT EXISTS trades (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    tenant_id TEXT NOT NULL DEFAULT 'default',
                    signal_id INTEGER REFERENCES signals(id) ON DELETE CASCADE,
                    executed_at TEXT NOT NULL,
                    executed_price REAL NOT NULL,
                    exit_at TEXT,
                    exit_price REAL,
                    pnl REAL,
                    outcome TEXT,
                    notes TEXT,
                    created_at TEXT DEFAULT (datetime('now'))
                );

                CREATE INDEX IF NOT EXISTS idx_trades_tenant_signal_id ON trades(tenant_id, signal_id);

                CREATE TABLE IF NOT EXISTS tuning_stats (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    tenant_id TEXT NOT NULL DEFAULT 'default',
                    pattern_name TEXT NOT NULL,
                    symbol TEXT,
                    wins INTEGER DEFAULT 0,
                    losses INTEGER DEFAULT 0,
                    avg_rr REAL DEFAULT 0.0,
                    avg_hold_time_seconds REAL DEFAULT 0.0,
                    last_updated TEXT DEFAULT (datetime('now')),
                    UNIQUE(tenant_id, pattern_name, symbol)
                );

                CREATE INDEX IF NOT EXISTS idx_tuning_stats_tenant_pattern ON tuning_stats(tenant_id, pattern_name);
                CREATE INDEX IF NOT EXISTS idx_tuning_stats_tenant_symbol ON tuning_stats(tenant_id, symbol);

                CREATE TABLE IF NOT EXISTS llm_usage (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    tenant_id TEXT NOT NULL DEFAULT 'default',
                    provider TEXT,
                    request_ts INTEGER,
                    request_size INTEGER,
                    response_tokens INTEGER,
                    estimated_cost REAL DEFAULT 0.0,
                    created_at TEXT DEFAULT (datetime('now'))
                );

                CREATE TABLE IF NOT EXISTS runtime_params (
                    tenant_id TEXT NOT NULL DEFAULT 'default',
                    key TEXT NOT NULL,
                    value TEXT,
                    description TEXT,
                    last_updated TEXT DEFAULT (datetime('now')),
                    PRIMARY KEY(tenant_id, key)
                );

                CREATE TABLE IF NOT EXISTS tuning_history (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    tenant_id TEXT NOT NULL DEFAULT 'default',
                    pattern_name TEXT,
                    symbol TEXT,
                    change TEXT,
                    old_value TEXT,
                    new_value TEXT,
                    notes TEXT,
                    applied_by TEXT,
                    created_at TEXT DEFAULT (datetime('now'))
                );

                CREATE INDEX IF NOT EXISTS idx_tuning_history_pattern ON tuning_history(pattern_name);
                CREATE INDEX IF NOT EXISTS idx_tuning_history_symbol ON tuning_history(symbol);

                -- Regret table for post-mortem analysis (Buffett/Simons style: learn from every mistake explicitly)
                CREATE TABLE IF NOT EXISTS regret_table (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    tenant_id TEXT NOT NULL DEFAULT 'default',
                    signal_id INTEGER,
                    trade_id INTEGER,
                    symbol TEXT,
                    outcome TEXT,
                    pnl REAL,
                    causes TEXT,  -- JSON list of attributed causes
                    tags TEXT,    -- user judgment tags from review (e.g. "ignored_hedge,low_conf")
                    lesson TEXT,  -- human or auto lesson
                    created_at TEXT DEFAULT (datetime('now'))
                );

                CREATE INDEX IF NOT EXISTS idx_regret_tenant ON regret_table(tenant_id, symbol);

                -- Small cause -> weight mapping persisted for ML (Simons: factors from labeled outcomes; Buffett: avoid repeating margin-of-safety violations)
                CREATE TABLE IF NOT EXISTS cause_weights (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    tenant_id TEXT NOT NULL DEFAULT 'default',
                    cause TEXT NOT NULL,
                    weight REAL NOT NULL DEFAULT 0.0,
                    count_bad INTEGER DEFAULT 0,
                    last_updated TEXT DEFAULT (datetime('now')),
                    UNIQUE(tenant_id, cause)
                );

                CREATE INDEX IF NOT EXISTS idx_cause_weights_tenant ON cause_weights(tenant_id, cause);

                COMMIT;
                """
            )
            self.conn.commit()
        finally:
            cur.close()

    def _migrate_schema_for_tenant_support(self) -> None:
        tables_to_patch = (
            "news",
            "analysis",
            "market_data",
            "signals",
            "trades",
            "tuning_stats",
            "llm_usage",
            "runtime_params",
            "tuning_history",
        )
        allowed_tables = set(tables_to_patch)
        for table_name in tables_to_patch:
            if table_name not in allowed_tables:
                raise DatabaseError(f"Unexpected table in tenant migration: {table_name}")
            info_rows = self._execute(f"PRAGMA table_info({table_name})").fetchall()
            columns = {r["name"] for r in info_rows}
            if "tenant_id" not in columns:
                self._execute(
                    f"ALTER TABLE {table_name} ADD COLUMN tenant_id TEXT NOT NULL DEFAULT '{DEFAULT_TENANT_ID}'"
                )
        for table_name in tables_to_patch:
            self._execute(
                f"UPDATE {table_name} SET tenant_id = ? WHERE tenant_id IS NULL OR tenant_id = ''",
                (DEFAULT_TENANT_ID,),
            )
        self.conn.commit()

        # New full-system columns for asset classification, buckets, hedging, audit
        self._migrate_full_system_columns()

    def _migrate_full_system_columns(self) -> None:
        """Add asset_class, bucket, hedge_of, decision_audit to signals if missing."""
        try:
            info = self._execute("PRAGMA table_info(signals)").fetchall()
            cols = {r["name"] for r in info}
            if "asset_class" not in cols:
                self._execute("ALTER TABLE signals ADD COLUMN asset_class TEXT")
            if "bucket" not in cols:
                self._execute("ALTER TABLE signals ADD COLUMN bucket TEXT")
            if "hedge_of" not in cols:
                self._execute("ALTER TABLE signals ADD COLUMN hedge_of INTEGER")
            if "decision_audit" not in cols:
                self._execute("ALTER TABLE signals ADD COLUMN decision_audit TEXT")
            self.conn.commit()
        except Exception:
            logger.exception("Full system column migration had issues (non-fatal)")

    def _execute(
        self, query: str, params: Optional[Iterable[Any]] = None
    ) -> sqlite3.Cursor:
        with self._lock:
            cur = self.conn.cursor()
            try:
                if params:
                    cur.execute(query, tuple(params))
                else:
                    cur.execute(query)
                return cur
            except Exception:
                logger.exception("DB execute error: %s; params=%s", query, params)
                cur.close()
                raise

    def _executemany(self, query: str, params: Iterable[Iterable[Any]]) -> None:
        with self._lock:
            cur = self.conn.cursor()
            try:
                cur.executemany(query, params)
                self.conn.commit()
            finally:
                cur.close()

    def _tenant_scoped_hash(self, raw_hash: str) -> str:
        value = (raw_hash or "").strip()
        if not value:
            raise DatabaseError("news hash must not be empty")
        scoped_prefix = f"{self.tenant_id}:"
        if value.startswith(scoped_prefix):
            return value
        return f"{scoped_prefix}{value}"

    def _runtime_key(self, key: str) -> str:
        return f"{self.tenant_id}:{key}"

    # ----------------------
    # News methods
    # ----------------------
    def insert_news(self, news: NewsItem) -> int:
        """
        Insert a news item if it doesn't exist (unique on hash). Returns the id.
        If the hash exists in DB, return the existing id and ignore the input params.
        """
        if not news.hash:
            # fallback to computing a simple hash from url+title+published_at
            base = (
                (news.url or "")
                + "|"
                + (news.title or "")
                + "|"
                + str(news.published_at)
            )
            import hashlib

            news.hash = hashlib.sha256(base.encode("utf-8")).hexdigest()
        scoped_hash = self._tenant_scoped_hash(news.hash)

        query = """
            INSERT INTO news (tenant_id, provider, url, title, content, published_at, asset, hash, fetched_at, raw_json)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        """
        params = (
            self.tenant_id,
            news.provider,
            news.url,
            news.title,
            news.content,
            ensure_iso(news.published_at) if news.published_at else None,
            news.asset,
            scoped_hash,
            ensure_iso(news.fetched_at or now_ts()),
            json.dumps(news.raw_json) if news.raw_json is not None else None,
        )
        try:
            cur = self._execute(query, params)
            new_id = cur.lastrowid
            self.conn.commit()
            return new_id
        except sqlite3.IntegrityError:
            # Duplicate; return existing ID
            row = self._execute(
                "SELECT id FROM news WHERE tenant_id = ? AND hash = ? LIMIT 1",
                (self.tenant_id, scoped_hash),
            ).fetchone()
            if row:
                return int(row["id"])
            raise DatabaseError("Failed to insert or lookup duplicate news")

    def get_news_by_hash(self, hash: str) -> Optional[sqlite3.Row]:
        scoped_hash = self._tenant_scoped_hash(hash)
        # Keep compatibility with records written before tenant scoping was introduced.
        cur = self._execute(
            "SELECT * FROM news WHERE tenant_id = ? AND hash IN (?, ?) LIMIT 1",
            (self.tenant_id, scoped_hash, hash),
        )
        row = cur.fetchone()
        return row

    def mark_news_processed(self, news_id: int, processed: bool = True) -> None:
        self._execute(
            "UPDATE news SET processed = ?, fetched_at = ? WHERE tenant_id = ? AND id = ?",
            (1 if processed else 0, ensure_iso(now_ts()), self.tenant_id, news_id),
        )
        self.conn.commit()

    def get_unprocessed_news(self, limit: int = 100) -> List[sqlite3.Row]:
        cur = self._execute(
            "SELECT * FROM news WHERE tenant_id = ? AND processed = 0 ORDER BY published_at DESC LIMIT ? ",
            (self.tenant_id, limit),
        )
        rows = cur.fetchall()
        return list(rows)

    def get_news(self, offset: int = 0, limit: int = 100) -> List[sqlite3.Row]:
        cur = self._execute(
            "SELECT * FROM news WHERE tenant_id = ? ORDER BY published_at DESC LIMIT ? OFFSET ?",
            (self.tenant_id, limit, offset),
        )
        return list(cur.fetchall())

    # ----------------------
    # Analysis methods
    # ----------------------
    def insert_analysis(self, analysis: AnalysisItem) -> int:
        query = """
            INSERT INTO analysis (tenant_id, news_id, provider, analysis_json, confidence)
            VALUES (?, ?, ?, ?, ?)
        """
        params = (
            self.tenant_id,
            analysis.news_id,
            analysis.provider,
            json.dumps(analysis.analysis_json),
            analysis.confidence,
        )
        cur = self._execute(query, params)
        self.conn.commit()
        return int(cur.lastrowid)

    def get_analysis_for_news(self, news_id: int) -> List[sqlite3.Row]:
        cur = self._execute(
            "SELECT * FROM analysis WHERE tenant_id = ? AND news_id = ? ORDER BY id ASC",
            (self.tenant_id, news_id),
        )
        return list(cur.fetchall())

    def get_latest_analysis(self, news_id: int) -> Optional[sqlite3.Row]:
        cur = self._execute(
            "SELECT * FROM analysis WHERE tenant_id = ? AND news_id = ? ORDER BY created_at DESC LIMIT 1",
            (self.tenant_id, news_id),
        )
        return cur.fetchone()

    # ----------------------
    # Market data caching
    # ----------------------
    def upsert_market_data_row(
        self,
        symbol: str,
        timeframe: str,
        start_ts: int,
        open_: float,
        high: float,
        low: float,
        close: float,
        volume: Optional[float] = None,
    ) -> int:
        """
        Insert or replace a market data candle row.
        """
        query = """
            INSERT OR REPLACE INTO market_data (tenant_id, symbol, timeframe, start_ts, open, high, low, close, volume)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
        """
        params = (
            self.tenant_id,
            symbol,
            timeframe,
            start_ts,
            open_,
            high,
            low,
            close,
            volume,
        )
        cur = self._execute(query, params)
        self.conn.commit()
        return int(cur.lastrowid)

    def get_market_data(
        self, symbol: str, timeframe: str, start_ts_min: int = 0, limit: int = 500
    ) -> List[sqlite3.Row]:
        cur = self._execute(
            "SELECT * FROM market_data WHERE tenant_id = ? AND symbol = ? AND timeframe = ? AND start_ts >= ? ORDER BY start_ts ASC LIMIT ?",
            (self.tenant_id, symbol, timeframe, start_ts_min, limit),
        )
        return list(cur.fetchall())

    # ----------------------
    # Signal methods
    # ----------------------
    def create_signal(self, signal: Signal) -> int:
        query = """
            INSERT INTO signals (
                tenant_id, news_id, symbol, side, entry_price, stop_loss, take_profit,
                leverage, position_size, risk_amount, rr, timeframe_hours, status,
                analysis_ids, created_at, expires_at,
                asset_class, bucket, hedge_of, decision_audit
            )
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        """
        params = (
            self.tenant_id,
            signal.news_id,
            signal.symbol,
            signal.side,
            signal.entry_price,
            signal.stop_loss,
            signal.take_profit,
            signal.leverage,
            signal.position_size,
            signal.risk_amount,
            signal.rr,
            signal.timeframe_hours,
            signal.status,
            json.dumps(signal.analysis_ids) if signal.analysis_ids is not None else None,
            ensure_iso(signal.created_at or now_ts()),
            ensure_iso(signal.expires_at) if signal.expires_at else None,
            signal.asset_class,
            signal.bucket,
            signal.hedge_of,
            signal.decision_audit,
        )
        cur = self._execute(query, params)
        self.conn.commit()
        return int(cur.lastrowid)

    def get_open_signals(self, limit: int = 200) -> List[sqlite3.Row]:
        cur = self._execute(
            "SELECT * FROM signals WHERE tenant_id = ? AND status = 'open' ORDER BY created_at ASC LIMIT ?",
            (self.tenant_id, limit),
        )
        return list(cur.fetchall())

    def get_signals_by_news(self, news_id: int) -> List[sqlite3.Row]:
        cur = self._execute(
            "SELECT * FROM signals WHERE tenant_id = ? AND news_id = ? ORDER BY created_at DESC",
            (self.tenant_id, news_id),
        )
        return list(cur.fetchall())

    def close_signal(
        self,
        signal_id: int,
        close_price: Optional[float] = None,
        outcome: Optional[str] = None,
        reason: Optional[str] = None,
        exit_time: Optional[int] = None,
        pnl: Optional[float] = None,
    ) -> None:
        """
        Mark a signal as closed and optionally set pnl/outcome/close reason. This will
        not create a trade record by itself (use record_trade for that), but it will set the signal as closed.
        """
        exit_time_iso = ensure_iso(exit_time or now_ts())
        cur = self._execute(
            "UPDATE signals SET status = 'closed', closed_at = ?, close_reason = ?, outcome = ?, pnl = ? WHERE tenant_id = ? AND id = ?",
            (exit_time_iso, reason, outcome, pnl, self.tenant_id, signal_id),
        )
        self.conn.commit()

    def cancel_signal(self, signal_id: int, reason: Optional[str] = None) -> None:
        self._execute(
            "UPDATE signals SET status = 'cancelled', closed_at = ?, close_reason = ? WHERE tenant_id = ? AND id = ?",
            (ensure_iso(now_ts()), reason, self.tenant_id, signal_id),
        )
        self.conn.commit()

    def find_duplicate_signals(
        self, symbol: str, side: str, within_seconds: int = 3600
    ) -> List[sqlite3.Row]:
        """
        Find recent signals created for the same symbol and side to reduce duplicate alerts.
        """
        cutoff = int(now_ts()) - within_seconds
        cutoff_iso = ensure_iso(cutoff)
        cur = self._execute(
            "SELECT * FROM signals WHERE tenant_id = ? AND symbol = ? AND side = ? AND created_at >= ? AND status = 'open'",
            (self.tenant_id, symbol, side, cutoff_iso),
        )
        return list(cur.fetchall())

    # ----------------------
    # Trade records
    # ----------------------
    def record_trade(self, record: TradeRecord) -> int:
        query = """
            INSERT INTO trades (tenant_id, signal_id, executed_at, executed_price, exit_at, exit_price, pnl, outcome, notes)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
        """
        params = (
            self.tenant_id,
            record.signal_id,
            ensure_iso(record.executed_at),
            record.executed_price,
            ensure_iso(record.exit_at) if record.exit_at else None,
            record.exit_price,
            record.pnl,
            record.outcome,
            record.notes,
        )
        cur = self._execute(query, params)
        self.conn.commit()
        return int(cur.lastrowid)

    def get_trades_for_signal(self, signal_id: int) -> List[sqlite3.Row]:
        cur = self._execute(
            "SELECT * FROM trades WHERE tenant_id = ? AND signal_id = ? ORDER BY created_at ASC",
            (self.tenant_id, signal_id),
        )
        return list(cur.fetchall())

    # ----------------------
    # Tuning stats methods
    # ----------------------
    def get_tuning_stat(
        self, pattern_name: str, symbol: Optional[str] = None
    ) -> Optional[sqlite3.Row]:
        if symbol:
            cur = self._execute(
                "SELECT * FROM tuning_stats WHERE tenant_id = ? AND pattern_name = ? AND symbol = ? LIMIT 1",
                (self.tenant_id, pattern_name, symbol),
            )
        else:
            cur = self._execute(
                "SELECT * FROM tuning_stats WHERE tenant_id = ? AND pattern_name = ? AND (symbol IS NULL OR symbol = '') LIMIT 1",
                (self.tenant_id, pattern_name),
            )
        return cur.fetchone()

    def update_tuning_stat(
        self,
        pattern_name: str,
        symbol: Optional[str],
        win: bool,
        rr: Optional[float],
        hold_seconds: Optional[float],
    ) -> None:
        """
        Incrementally update tuning stats. If the entry doesn't exist, it will be created.
        The stats are kept as aggregated counters + EWMA-like avg values.
        """
        now_iso = ensure_iso(now_ts())
        existing = self.get_tuning_stat(pattern_name, symbol)
        if existing is None:
            wins = 1 if win else 0
            losses = 0 if win else 1
            avg_rr = float(rr) if rr else 0.0
            avg_hold = float(hold_seconds) if hold_seconds else 0.0
            self._execute(
                "INSERT INTO tuning_stats (tenant_id, pattern_name, symbol, wins, losses, avg_rr, avg_hold_time_seconds, last_updated) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                (
                    self.tenant_id,
                    pattern_name,
                    symbol,
                    wins,
                    losses,
                    avg_rr,
                    avg_hold,
                    now_iso,
                ),
            )
            self.conn.commit()
            return

        # compute updated aggregate
        wins = int(existing["wins"]) + (1 if win else 0)
        losses = int(existing["losses"]) + (0 if win else 1)
        prev_avg_rr = float(existing["avg_rr"] or 0.0)
        prev_avg_hold = float(existing["avg_hold_time_seconds"] or 0.0)
        total = wins + losses
        # incremental average: new_avg = (prev_avg * (n-1) + new_val) / n
        new_avg_rr = prev_avg_rr
        new_avg_hold = prev_avg_hold
        if rr:
            new_avg_rr = ((prev_avg_rr * (total - 1)) + rr) / total
        if hold_seconds:
            new_avg_hold = ((prev_avg_hold * (total - 1)) + hold_seconds) / total
        self._execute(
            "UPDATE tuning_stats SET wins = ?, losses = ?, avg_rr = ?, avg_hold_time_seconds = ?, last_updated = ? WHERE tenant_id = ? AND id = ?",
            (wins, losses, new_avg_rr, new_avg_hold, now_iso, self.tenant_id, existing["id"]),
        )
        self.conn.commit()

    # Tuning history auditing
    # ----------------------
    def add_tuning_history_entry(
        self,
        pattern_name: str,
        symbol: Optional[str],
        change: str,
        old_value: Optional[str],
        new_value: Optional[str],
        notes: Optional[str] = None,
        applied_by: Optional[str] = "auto_tuner",
    ) -> int:
        """
        Record a single tuning change applied by the automatic tuner or manually, for auditing.
        """
        cur = self._execute(
            "INSERT INTO tuning_history (tenant_id, pattern_name, symbol, change, old_value, new_value, notes, applied_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
            (
                self.tenant_id,
                pattern_name,
                symbol,
                change,
                old_value,
                new_value,
                notes,
                applied_by,
            ),
        )
        self.conn.commit()
        return int(cur.lastrowid)

    def get_tuning_history(
        self,
        pattern_name: Optional[str] = None,
        symbol: Optional[str] = None,
        limit: int = 200,
    ) -> List[sqlite3.Row]:
        """
        Query tuning history rows, optionally filtered by pattern or symbol.
        """
        if pattern_name and symbol:
            cur = self._execute(
                "SELECT * FROM tuning_history WHERE tenant_id = ? AND pattern_name = ? AND symbol = ? ORDER BY created_at DESC LIMIT ?",
                (self.tenant_id, pattern_name, symbol, limit),
            )
        elif pattern_name:
            cur = self._execute(
                "SELECT * FROM tuning_history WHERE tenant_id = ? AND pattern_name = ? ORDER BY created_at DESC LIMIT ?",
                (self.tenant_id, pattern_name, limit),
            )
        elif symbol:
            cur = self._execute(
                "SELECT * FROM tuning_history WHERE tenant_id = ? AND symbol = ? ORDER BY created_at DESC LIMIT ?",
                (self.tenant_id, symbol, limit),
            )
        else:
            cur = self._execute(
                "SELECT * FROM tuning_history WHERE tenant_id = ? ORDER BY created_at DESC LIMIT ?",
                (self.tenant_id, limit),
            )

        rows = cur.fetchall()
        return list(rows)

    # ----------------------
    # Regret table for ML post-mortems (explicit learning from mistakes)
    # ----------------------
    def log_regret(self, signal_id: int, trade_id: int, symbol: str, outcome: str, pnl: float, causes: list, tags: list = None, lesson: str = None):
        """Log a regret entry for analysis and feedback. Also updates cause_weights delta (negative for losses)."""
        causes_json = json.dumps(causes) if causes else "[]"
        tags_json = json.dumps(tags) if tags else "[]"
        self._execute(
            "INSERT INTO regret_table (tenant_id, signal_id, trade_id, symbol, outcome, pnl, causes, tags, lesson) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
            (self.tenant_id, signal_id, trade_id, symbol, outcome, pnl, causes_json, tags_json, lesson)
        )
        self.conn.commit()
        # Immediately persist cause->weight mapping (Simons online update)
        is_bad = (pnl or 0) < 0 or outcome in ("loss", "timeout")
        delta = -0.12 if is_bad else 0.03  # stronger penalty for mistakes
        for c in (causes or []):
            try:
                self.update_cause_weight(c, delta, is_bad=is_bad)
            except Exception:
                pass

    def get_regrets(self, symbol: str = None, limit: int = 50) -> List[Dict]:
        """Query regrets for review and ML feedback."""
        if symbol:
            cur = self._execute("SELECT * FROM regret_table WHERE tenant_id = ? AND symbol = ? ORDER BY created_at DESC LIMIT ?", (self.tenant_id, symbol, limit))
        else:
            cur = self._execute("SELECT * FROM regret_table WHERE tenant_id = ? ORDER BY created_at DESC LIMIT ?", (self.tenant_id, limit))
        return [dict(r) for r in cur.fetchall()]

    # ----------------------
    # Cause weights table (persist small cause -> weight for ML feedback loop)
    # ----------------------
    def update_cause_weight(self, cause: str, delta: float, is_bad: bool = True):
        """Update or insert cause weight. Delta typically negative for bad outcomes (e.g. -0.1)."""
        cur = self._execute(
            "INSERT INTO cause_weights (tenant_id, cause, weight, count_bad, last_updated) "
            "VALUES (?, ?, ?, ?, datetime('now')) "
            "ON CONFLICT(tenant_id, cause) DO UPDATE SET "
            "weight = weight + ?, count_bad = count_bad + ?, last_updated = datetime('now')",
            (self.tenant_id, cause, delta, 1 if is_bad else 0, delta, 1 if is_bad else 0)
        )
        self.conn.commit()
        return True

    def get_cause_weights(self) -> Dict[str, float]:
        """Return dict cause -> current weight (for scorer/tuner bias)."""
        cur = self._execute(
            "SELECT cause, weight, count_bad FROM cause_weights WHERE tenant_id = ?",
            (self.tenant_id,)
        )
        res = {}
        for r in cur.fetchall():
            res[r["cause"]] = float(r["weight"])
        return res

    def get_cause_weight(self, cause: str) -> float:
        w = self.get_cause_weights().get(cause, 0.0)
        return w

    # ----------------------
    # LLM usage logging
    # ----------------------
    def log_llm_usage(
        self,
        provider: str,
        request_ts: int,
        request_size: int,
        response_tokens: int,
        estimated_cost: float = 0.0,
    ) -> int:
        cur = self._execute(
            "INSERT INTO llm_usage (tenant_id, provider, request_ts, request_size, response_tokens, estimated_cost) VALUES (?, ?, ?, ?, ?, ?)",
            (
                self.tenant_id,
                provider,
                request_ts,
                request_size,
                response_tokens,
                estimated_cost,
            ),
        )
        self.conn.commit()
        return int(cur.lastrowid)

    def get_llm_usage(
        self, provider: Optional[str] = None, since_ts: Optional[int] = None
    ) -> List[sqlite3.Row]:
        if provider and since_ts:
            cur = self._execute(
                "SELECT * FROM llm_usage WHERE tenant_id = ? AND provider = ? AND request_ts >= ? ORDER BY request_ts DESC",
                (self.tenant_id, provider, since_ts),
            )
        elif provider:
            cur = self._execute(
                "SELECT * FROM llm_usage WHERE tenant_id = ? AND provider = ? ORDER BY request_ts DESC",
                (self.tenant_id, provider),
            )
        elif since_ts:
            cur = self._execute(
                "SELECT * FROM llm_usage WHERE tenant_id = ? AND request_ts >= ? ORDER BY request_ts DESC",
                (self.tenant_id, since_ts),
            )
        else:
            cur = self._execute(
                "SELECT * FROM llm_usage WHERE tenant_id = ? ORDER BY request_ts DESC",
                (self.tenant_id,),
            )
        return list(cur.fetchall())

    # ----------------------
    # Runtime parameters (CRUD)
    # ----------------------
    def set_runtime_param(
        self, key: str, value: str, description: Optional[str] = None
    ) -> None:
        """
        Create or update a runtime parameter that can be used to tweak system behavior
        without needing to modify environment variables or redeploy. The table stores
        the last updated timestamp automatically.
        """
        scoped_key = self._runtime_key(key)
        self._execute(
            "INSERT OR REPLACE INTO runtime_params (tenant_id, key, value, description, last_updated) VALUES (?, ?, ?, ?, datetime('now'))",
            (self.tenant_id, scoped_key, value, description),
        )
        self.conn.commit()

    def get_runtime_param(self, key: str) -> Optional[str]:
        """
        Read a runtime parameter value. Returns None if not found.
        """
        scoped_key = self._runtime_key(key)
        cur = self._execute(
            "SELECT value FROM runtime_params WHERE tenant_id = ? AND key = ? LIMIT 1",
            (self.tenant_id, scoped_key),
        )
        row = cur.fetchone()
        return row["value"] if row else None

    def get_runtime_param_dict(self, prefix: Optional[str] = None) -> Dict[str, str]:
        """
        Return a dictionary of runtime parameters. If prefix is provided, only return keys starting with that prefix.
        """
        scoped_prefix = self._runtime_key(prefix) if prefix else None
        if prefix:
            rows = self._execute(
                "SELECT key, value FROM runtime_params WHERE tenant_id = ? AND key LIKE ?;",
                (self.tenant_id, f"{scoped_prefix}%"),
            ).fetchall()
        else:
            rows = self._execute(
                "SELECT key, value FROM runtime_params WHERE tenant_id = ?;",
                (self.tenant_id,),
            ).fetchall()
        results: Dict[str, str] = {}
        key_prefix = f"{self.tenant_id}:"
        for r in rows:
            db_key = r["key"]
            if isinstance(db_key, str) and db_key.startswith(key_prefix):
                db_key = db_key[len(key_prefix) :]
            results[db_key] = r["value"]
        return results

    def delete_runtime_param(self, key: str) -> None:
        """
        Remove a runtime parameter.
        """
        scoped_key = self._runtime_key(key)
        self._execute(
            "DELETE FROM runtime_params WHERE tenant_id = ? AND key = ?;",
            (self.tenant_id, scoped_key),
        )
        self.conn.commit()

    def get_runtime_param_as_bool(self, key: str, default: bool = False) -> bool:
        """
        Convenience method for boolean runtime params stored as 'true'/'false' (case-insensitive) or '1'/'0'.
        """
        val = self.get_runtime_param(key)
        if val is None:
            return default
        v = str(val).strip().lower()
        if v in ("1", "true", "yes", "y", "on"):
            return True
        if v in ("0", "false", "no", "n", "off"):
            return False
        return default

    def get_runtime_param_as_int(self, key: str, default: int = 0) -> int:
        val = self.get_runtime_param(key)
        if val is None:
            return default
        try:
            return int(val)
        except Exception:
            return default

    def get_runtime_param_as_float(self, key: str, default: float = 0.0) -> float:
        val = self.get_runtime_param(key)
        if val is None:
            return default
        try:
            return float(val)
        except Exception:
            return default

    # ----------------------
    # Misc helpers
    # ----------------------
    def prune_market_data_older_than(self, seconds: int) -> None:
        cutoff = ensure_iso(int(now_ts()) - seconds)
        self._execute(
            "DELETE FROM market_data WHERE tenant_id = ? AND created_at < ?",
            (self.tenant_id, cutoff),
        )
        self.conn.commit()

    def vacuum(self) -> None:
        self._execute("VACUUM;")
        self.conn.commit()

    def execute_custom(
        self, sql: str, params: Optional[Iterable[Any]] = None
    ) -> List[sqlite3.Row]:
        cur = self._execute(sql, params)
        try:
            rows = cur.fetchall()
            return list(rows)
        finally:
            cur.close()

    # Context manager for a transaction
    @contextmanager
    def transaction(self):
        with self._lock:
            cur = self.conn.cursor()
            try:
                cur.execute("BEGIN;")
                yield cur
                self.conn.commit()
            except Exception:
                self.conn.rollback()
                raise
            finally:
                cur.close()


# Instantiate a default Database object helper used by scripts; users can create multiple DB instances as needed
_default_db: Optional[Database] = None


def get_default_db(
    db_path: Optional[str] = None, tenant_id: str = DEFAULT_TENANT_ID
) -> Database:
    global _default_db
    resolved_path = db_path or "simple_trader.db"
    resolved_tenant = normalize_tenant_id(tenant_id)
    if _default_db is None:
        _default_db = Database(resolved_path, tenant_id=resolved_tenant)
    elif resolved_path != _default_db.db_path:
        raise DatabaseError(
            f"Default DB already initialized with path '{_default_db.db_path}', "
            f"but got a request for '{resolved_path}'."
        )
    elif resolved_tenant != _default_db.tenant_id:
        raise DatabaseError(
            f"Default DB already initialized with tenant '{_default_db.tenant_id}', "
            f"but got a request for '{resolved_tenant}'."
        )
    return _default_db


# For demonstration / quick tests when running as script
if __name__ == "__main__":
    logging.basicConfig(level=logging.DEBUG)
    db = get_default_db(":memory:")
    n = NewsItem(
        provider="rss",
        url="https://example.com/article1",
        title="Test news",
        content="This is a test",
        published_at=now_ts(),
        asset="BTC",
        raw_json={"sample": True},
    )
    news_id = db.insert_news(n)
    logger.info("Inserted news_id=%s", news_id)
    a = AnalysisItem(
        news_id=news_id,
        provider="mock",
        analysis_json={"sentiment": "positive", "impact": 0.7},
        confidence=0.7,
    )
    aid = db.insert_analysis(a)
    logger.info("Inserted analysis id=%s for news %s", aid, news_id)
    s = Signal(
        news_id=news_id,
        symbol="BTCUSDT",
        side="long",
        entry_price=50000.0,
        stop_loss=49000.0,
        take_profit=53000.0,
        leverage=5,
        rr=3.0,
        timeframe_hours=2,
        analysis_ids=[aid],
    )
    sid = db.create_signal(s)
    logger.info("Inserted signal id=%s", sid)
    sigs = db.get_open_signals()
    logger.info("Open signals: %s", len(sigs))
