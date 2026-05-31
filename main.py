# Simple-Trader\main.py
# -*- coding: utf-8 -*-
"""
CLI entry point and scheduling for fetching news, generating signals,
and sending via Telegram. This script provides an operational wrapper for:

- Fetching news via RSS / NewsAPI / configured providers.
- Analyzing news via multiple LLM providers (groq, cloudflare, google, mock).
- Combining LLM output with 2h candlestick pattern detection, generating signals
  and computing entry/stop/target/leverage/position sizing.
- Storing news, analyses, signals, and trades into the local SQLite DB.
- Sending actionable signals via Telegram (if a Telegram bot token and chat id are configured).
- Closing expired signals and running lightweight strategy tuning based on recorded trades.

This CLI can be run once or scheduled to run continuously with a configurable interval.

Example:
    python main.py run --interval 300
    python main.py fetch
    python main.py process --max-news 200
    python main.py status
"""

from __future__ import annotations

import argparse
import logging
import os
import signal
import sys
import threading
import time
from datetime import datetime, timezone
from typing import Any, Dict, Optional

# Load environment variables from `.env` file if present (supports python-dotenv).
# We prefer to load .env here before importing `CONFIG` so environment-driven configuration is applied.
try:
    from dotenv import load_dotenv

    load_dotenv()
except Exception:
    # If python-dotenv is not installed or a .env file is missing, proceed silently.
    pass

# Import package-level config and modules
from simple_trader import CONFIG, get_logger, setup_logging
from simple_trader.db import get_default_db
from simple_trader.llm_pool import LLMPool
from simple_trader.market_data import MarketDataClient
from simple_trader.metrics import start_metrics_server
from simple_trader.news_fetcher import FetchResult, NewsFetcher
from simple_trader.paper_execution import PaperExecutor
from simple_trader.signal_manager import SignalManager
from simple_trader.telegram import TelegramNotifier

LOG = get_logger(__name__)


def _parse_time_to_epoch(value: Optional[str]) -> int:
    """
    Parse a timestamp string to an epoch integer (seconds).

    Accepts:
    - ISO-8601 strings like '2023-08-21T12:34:56Z' or with explicit offsets
    - Numeric epoch strings like '1692612496'
    - None -> returns current time

    The function returns an integer epoch seconds and falls back to the current time when parsing fails.
    """
    if value is None:
        return int(time.time())
    try:
        s = str(value).strip()
        # numeric epoch
        if s.isdigit():
            return int(s)
        # Handle Z timezone by converting to +00:00 for Python's fromisoformat
        if s.endswith("Z"):
            s = s[:-1] + "+00:00"
        dt = datetime.fromisoformat(s)
        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=timezone.utc)
        return int(dt.timestamp())
    except Exception:
        LOG.warning("Failed to parse timestamp [%s], using now", value)
        return int(time.time())


class Orchestrator:
    """
    Orchestrates the top-level flow for the Simple-Trader project.
    """

    def __init__(self, config=None):
        self.config = config or CONFIG
        # Setup logging depending on config
        setup_logging(
            level=getattr(logging, self.config.log_level.upper(), logging.INFO)
        )
        # Initialize DB and components
        self.db = get_default_db(self.config.database_path)

        # Seed runtime parameters with safe default tuning values in the database.
        # This ensures a fresh deployment will initialize the runtime tuning parameters
        # in a deterministic and auditable way. If the parameter already exists, we leave it untouched.
        try:
            self._seed_runtime_params()
        except Exception:
            # We don't want the orchestrator startup to crash due to seeding issues.
            LOG.exception("Failed to seed runtime parameters")

        # Start Prometheus metrics server if telemetry is enabled in config
        if getattr(self.config, "enable_telemetry", False):
            try:
                port = int(os.getenv("PROMETHEUS_PORT", "9000"))
            except Exception:
                port = 9000
            try:
                start_metrics_server(port=port)
                LOG.info("Prometheus metrics server started on port %s", port)
            except Exception:
                # Avoid crash on startup if metrics can't be started; telemetry must not block service.
                LOG.exception("Failed to start Prometheus metrics server")

        self.news_fetcher = NewsFetcher(self.config, self.db)
        self.llm_pool = LLMPool(self.config, self.db)
        self.market_client = MarketDataClient(self.config, self.db)
        self.signal_manager = SignalManager(
            config=self.config,
            db=self.db,
            news_fetcher=self.news_fetcher,
            llm_pool=self.llm_pool,
            market_client=self.market_client,
            telegram_notifier=TelegramNotifier(config=self.config, db=self.db),
        )
        self._stop_event = threading.Event()

    def _seed_runtime_params(self):
        """
        Insert safe default runtime parameters (if not present) into the DB for dynamic tuning.
        These are conservative defaults intended for production usage and automatic tuning.
        """
        defaults = {
            # LLM and pattern confidence thresholds
            "llm_min_confidence": str(self.config.llm_min_confidence),
            "min_pattern_confidence": str(self.config.min_pattern_confidence),
            # risk & exposure controls
            "min_risk_reward_ratio": str(self.config.min_risk_reward_ratio),
            "risk_per_trade_pct": str(self.config.risk_per_trade_pct),
            # leverage caps
            "max_leverage_crypto": str(self.config.max_leverage_crypto),
            "max_leverage_forex": str(self.config.max_leverage_forex),
            # operational limits
            "open_positions_limit": str(self.config.open_positions_limit),
            # scorer
            "signal_score_threshold": str(0.6),
            # autotuner
            "runtime_auto_apply_tuner": "true",
            # safe defaults for scorer training window (in seconds)
            "scorer_train_lookback_seconds": str(3600 * 24 * 30),  # 30 days
        }

        # Persist these defaults if not already set in the DB.
        for key, value in defaults.items():
            try:
                if self.db.get_runtime_param(key) is None:
                    # Add the runtime param with a short description for auditability.
                    self.db.set_runtime_param(
                        key, value, description="seeded default runtime parameter"
                    )
            except Exception:
                LOG.exception("Failed to set runtime parameter %s in DB", key)

    def fetch_news_once(self) -> Dict[str, int]:
        """
        Fetch the news using the configured provider(s) and return stats about inserted/ignored.
        """
        LOG.info("Starting one-off fetch of news (RSS/API configured)")
        try:
            results = self.news_fetcher.fetch_all()
            # results contains FetchResult objects (provider, news_item, inserted_id)
            inserted = len(results)
            LOG.info("Fetch completed - inserted/deduped items=%d", inserted)
            return {"inserted": inserted}
        except Exception:
            LOG.exception("Failed to fetch news")
            return {"inserted": 0, "error": 1}

    def process_news_once(self, max_news: int = 100) -> Dict[str, int]:
        """
        Process unprocessed news items to create signals.
        Returns stats about created signals.
        """
        LOG.info("Processing up to %d unprocessed news items", max_news)
        try:
            created_signal_ids = self.signal_manager.process_unprocessed_news(
                limit=max_news
            )
            LOG.info(
                "Processing completed - signals created=%d", len(created_signal_ids)
            )
            return {"created_signals": len(created_signal_ids)}
        except Exception:
            LOG.exception("Failed to process news and generate signals")
            return {"created_signals": 0, "error": 1}

    def close_expired_once(self) -> Dict[str, int]:
        """
        Close expired signals. Returns number closed.
        """
        LOG.info("Closing expired signals (if any)")
        try:
            closed = self.signal_manager.close_expired_signals()
            LOG.info("Closed expired signals count=%d", len(closed))
            return {"closed_signals": len(closed)}
        except Exception:
            LOG.exception("Failed to close expired signals")
            return {"closed_signals": 0, "error": 1}

    def monitor_and_tune_once(self, since_seconds: int = 3600 * 24) -> Dict[str, int]:
        """
        Update strategy tuner and analyze the most recent trades to adapt.
        """
        LOG.info("Running monitor and tuner updates (since %s seconds)", since_seconds)
        try:
            self.signal_manager.monitor_and_tune(since_seconds=since_seconds)
            LOG.info("Monitor & tune completed")
            return {"tuned": 1}
        except Exception:
            LOG.exception("Failed to run monitor/tune")
            return {"tuned": 0, "error": 1}

    def status(self) -> Dict[str, int]:
        """
        Return quick status counters to help with monitoring & debugging.
        """
        try:
            stats = {}
            stats["unprocessed_news"] = (
                int(
                    self.db.execute_custom(
                        "SELECT COUNT(*) AS c FROM news WHERE processed = 0"
                    )[0]["c"]
                )
                if self.db.execute_custom(
                    "SELECT COUNT(*) AS c FROM news WHERE processed = 0"
                )
                else 0
            )
            stats["open_signals"] = (
                int(
                    self.db.execute_custom(
                        "SELECT COUNT(*) AS c FROM signals WHERE status = 'open'"
                    )[0]["c"]
                )
                if self.db.execute_custom(
                    "SELECT COUNT(*) AS c FROM signals WHERE status = 'open'"
                )
                else 0
            )
            stats["total_signals"] = (
                int(self.db.execute_custom("SELECT COUNT(*) AS c FROM signals")[0]["c"])
                if self.db.execute_custom("SELECT COUNT(*) AS c FROM signals")
                else 0
            )
            stats["trades_count"] = (
                int(self.db.execute_custom("SELECT COUNT(*) AS c FROM trades")[0]["c"])
                if self.db.execute_custom("SELECT COUNT(*) AS c FROM trades")
                else 0
            )
            stats["recent_llm_requests"] = len(
                self.db.get_llm_usage(provider=None, since_ts=None)
            )
            LOG.info(
                "Status: news=%s open_signals=%s trades=%s",
                stats["unprocessed_news"],
                stats["open_signals"],
                stats["trades_count"],
            )
            return stats
        except Exception:
            LOG.exception("Failed to query status from DB")
            return {}

    def run_periodic(self, interval_seconds: int = 300, max_news: int = 100):
        """
        Run the orchestration flow in a loop with an interval in seconds.
        Steps:
          1. Fetch news
          2. Process news -> create signals
          3. Close expired signals
          4. Monitor & tune
        """
        LOG.info("Starting periodic runner (interval=%s seconds)", interval_seconds)

        # Hook to handle graceful shutdown
        def _sig_handler(signum, frame):
            LOG.info("Received signal %s: shutting down the runner", signum)
            self._stop_event.set()

        signal.signal(signal.SIGINT, _sig_handler)
        signal.signal(signal.SIGTERM, _sig_handler)

        last_run = 0
        while not self._stop_event.is_set():
            try:
                tstart = time.time()
                # Dynamically adapt LLM provider weights based on recent LLM usage
                try:
                    self.llm_pool.adapt_weights()
                except Exception:
                    LOG.exception("Failed to adapt LLM provider weights")
                # Run the steps sequentially
                self.fetch_news_once()
                self.process_news_once(max_news)
                self.close_expired_once()
                # run tuner asynchronously occasionally (we run every loop)
                self.monitor_and_tune_once()
                # sleep until next interval while being responsive to signals
                elapsed = time.time() - tstart
                # We choose to log only if the cycle took longer than a fraction of interval to avoid spam
                if elapsed > interval_seconds * 0.8:
                    LOG.info("Cycle took %.2f seconds (near interval limit)", elapsed)
                # Wait up to `interval_seconds` but exit sooner if requested to stop
                to_wait = max(0, interval_seconds - elapsed)
                # wait in short increments to be responsive to stop event
                wait_step = 0.5
                waited = 0
                while waited < to_wait and not self._stop_event.is_set():
                    time.sleep(min(wait_step, to_wait - waited))
                    waited += wait_step
            except Exception:
                LOG.exception("Unexpected error during periodic run cycle")
                # Sleep a bit before retrying to avoid hot loops on consistent errors
                time.sleep(min(60, interval_seconds))
        LOG.info("Periodic runner stopped")

    def stop(self):
        self._stop_event.set()
        try:
            if getattr(self, "llm_pool", None) is not None:
                self.llm_pool.shutdown(wait=True)
        except Exception:
            LOG.exception("Failed to shutdown LLM pool")


def parse_cli_args(argv) -> argparse.Namespace:
    ap = argparse.ArgumentParser(
        prog="simple-trader", description="Simple Trader CLI runner"
    )
    sub = ap.add_subparsers(dest="command", help="Command to run")

    # top-level commands
    # fetch news
    fetch_cmd = sub.add_parser(
        "fetch", help="Fetch news from configured providers (RSS, NewsAPI)"
    )
    fetch_cmd.add_argument(
        "--list", action="store_true", help="Only list configured RSS feeds and exit"
    )

    # process news -> generate signals
    process_cmd = sub.add_parser(
        "process", help="Process unprocessed news to generate signals"
    )
    process_cmd.add_argument(
        "--max-news",
        default=100,
        type=int,
        help="Max number of unprocessed news items to process in a run",
    )

    # close expired
    close_cmd = sub.add_parser("close", help="Close expired signals")

    # run loop
    run_cmd = sub.add_parser(
        "run", help="Run the fetch/process/close/tune loop periodically"
    )
    run_cmd.add_argument(
        "--interval",
        type=int,
        default=300,
        help="Interval in seconds between cycles (default 300s)",
    )
    run_cmd.add_argument(
        "--max-news", default=100, type=int, help="Max news to process per cycle"
    )

    # monitor/tune
    monitor_cmd = sub.add_parser(
        "monitor",
        help="Run monitor/tune cycle (update tuner stats based on recent trades)",
    )
    monitor_cmd.add_argument(
        "--since-seconds",
        default=3600 * 24,
        type=int,
        help="How far back to consider trades for tuning (seconds)",
    )

    # Paper-execution simulator (simulate fills and record paper trades)
    paper_exec_cmd = sub.add_parser(
        "paper-exec",
        help="Run paper-execution simulation for open signals and record results",
    )
    paper_exec_cmd.add_argument(
        "--lookback-hours",
        default=None,
        type=int,
        help="How many hours of historical market data to consider for simulation (defaults to a conservative window)",
    )
    paper_exec_cmd.add_argument(
        "--slippage-pct",
        default=None,
        type=float,
        help="Optional slippage fraction to simulate for entries/exits (e.g., 0.001 = 0.1%)",
    )

    # Suggest adjustments from the tuner
    suggest_cmd = sub.add_parser(
        "suggest", help="Show suggested adjustments from the strategy tuner"
    )
    suggest_cmd.add_argument(
        "--min-win-rate",
        default=0.4,
        type=float,
        help="Minimum win rate to consider a pattern acceptable (default 0.4)",
    )
    suggest_cmd.add_argument(
        "--min-avg-rr",
        default=3.0,
        type=float,
        help="Minimum average RR to consider acceptable (default 3.0)",
    )
    suggest_cmd.add_argument(
        "--min-sample-size",
        default=10,
        type=int,
        help="Minimum number of sample trades required to make suggestions (default 10)",
    )
    suggest_cmd.add_argument(
        "--apply",
        action="store_true",
        help="Apply suggested tuning adjustments automatically (DANGEROUS: enable only when you trust the auto-tuner). Default: false",
    )

    # single-shot: run everything once
    all_cmd = sub.add_parser(
        "all", help="Fetch, process and close expired signals once"
    )
    all_cmd.add_argument(
        "--max-news",
        default=100,
        type=int,
        help="Max news to process in the single run",
    )

    # status
    status_cmd = sub.add_parser("status", help="Show status & counters from DB")

    # record-trade: record a result for a previously generated signal
    record_cmd = sub.add_parser(
        "record-trade", help="Record result of a trade for a signal id"
    )
    record_cmd.add_argument(
        "--signal-id",
        required=True,
        type=int,
        dest="signal_id",
        help="Signal ID from the signals table",
    )
    record_cmd.add_argument(
        "--executed-at",
        required=False,
        default=None,
        type=str,
        dest="executed_at",
        help="Executed at time (ISO or epoch seconds). Defaults to now.",
    )
    record_cmd.add_argument(
        "--executed-price",
        required=True,
        type=float,
        dest="executed_price",
        help="Executed entry price (numeric)",
    )
    record_cmd.add_argument(
        "--exit-at",
        required=False,
        default=None,
        type=str,
        dest="exit_at",
        help="Exit time (ISO or epoch seconds). Defaults to now.",
    )
    record_cmd.add_argument(
        "--exit-price",
        required=True,
        type=float,
        dest="exit_price",
        help="Exit price (numeric)",
    )
    record_cmd.add_argument(
        "--pnl",
        required=True,
        type=float,
        dest="pnl",
        help="Profit/Loss in USD (float)",
    )
    record_cmd.add_argument(
        "--outcome",
        required=True,
        choices=["win", "loss", "timeout", "cancelled"],
        default="loss",
        dest="outcome",
        help="Outcome: win / loss / timeout / cancelled",
    )
    record_cmd.add_argument(
        "--notes",
        required=False,
        default=None,
        dest="notes",
        type=str,
        help="Optional notes",
    )

    # debug
    version_cmd = sub.add_parser("version", help="Print Simple-Trader package version")

    ap.add_argument(
        "--debug", action="store_true", help="Enable debug logging (overrides config)"
    )
    ap.add_argument(
        "--cfg-file",
        default=None,
        help="Optional config file path (not yet implemented option)",
    )
    return ap.parse_args(argv)


def main(argv=None):
    args = parse_cli_args(argv or sys.argv[1:])
    # Activate logging using config and debug flag
    cfg = CONFIG
    if args.debug:
        setup_logging(level=logging.DEBUG)
    else:
        setup_logging(level=getattr(logging, cfg.log_level.upper(), logging.INFO))
    LOG.info("Starting Simple-Trader CLI: command=%s", args.command)

    orchestrator = Orchestrator(config=cfg)

    if args.command == "fetch":
        if getattr(args, "list", False):
            LOG.info("RSS feeds configured:")
            for f in cfg.news_rss_feeds:
                LOG.info("- %s", f)
            return 0
        res = orchestrator.fetch_news_once()
        LOG.info("Fetch: result=%s", res)
        return 0

    if args.command == "process":
        res = orchestrator.process_news_once(max_news=getattr(args, "max_news", 100))
        LOG.info("Process: result=%s", res)
        return 0

    if args.command == "close":
        res = orchestrator.close_expired_once()
        LOG.info("Close: result=%s", res)
        return 0

    if args.command == "monitor":
        res = orchestrator.monitor_and_tune_once(
            getattr(args, "since_seconds", 3600 * 24)
        )
        LOG.info("Monitor: result=%s", res)
        return 0

    if args.command == "suggest":
        # Gather CLI args and call tuner.suggest_adjustments to get recommendations
        min_win_rate = getattr(args, "min_win_rate", 0.4)
        min_avg_rr = getattr(args, "min_avg_rr", 3.0)
        min_sample_size = getattr(args, "min_sample_size", 10)
        apply_changes = getattr(args, "apply", False)
        try:
            suggestions = orchestrator.signal_manager.tuner.suggest_adjustments(
                min_win_rate=min_win_rate,
                min_avg_rr=min_avg_rr,
                min_sample_size=min_sample_size,
            )
            LOG.info("Tuner suggestions found: %s", len(suggestions))
            for s in suggestions:
                # Log them; the user may redirect logs to files or monitoring
                LOG.info("%s", s)
            # Apply if the user explicitly provided --apply; otherwise only display or log suggestions
            if apply_changes:
                try:
                    applied = orchestrator.signal_manager.tuner.apply_adjustments(
                        suggestions, auto_apply=True
                    )
                    LOG.info("Auto-applied %s tuning adjustment(s)", len(applied))
                except Exception:
                    LOG.exception("Failed to auto-apply the suggestions")
            return 0
        except Exception:
            LOG.exception("Failed to compute tuner suggestions")
            return 1

    if args.command == "all":
        orchestrator.fetch_news_once()
        orchestrator.process_news_once(max_news=getattr(args, "max_news", 100))
        orchestrator.close_expired_once()
        orchestrator.monitor_and_tune_once()
        LOG.info("All steps executed.")
        return 0

    if args.command == "status":
        status = orchestrator.status()
        LOG.info("Status: %s", status)
        return 0

    if args.command == "paper-exec":
        LOG.info("Running paper-execution simulation (paper-exec)")
        lookback = getattr(args, "lookback_hours", None)
        slippage = getattr(args, "slippage_pct", None)
        try:
            pe = PaperExecutor(
                config=cfg,
                db=orchestrator.db,
                market_client=orchestrator.market_client,
                signal_manager=orchestrator.signal_manager,
                simulate_slippage_pct=slippage,
            )
            created = pe.simulate_open_signals_once(lookback_hours=lookback)
            LOG.info("Paper execution recorded %s simulated trades", len(created))
            return 0
        except Exception:
            LOG.exception("Paper execution simulation failed")
            return 1

    if args.command == "record-trade":
        # Normalize timestamps into epoch seconds
        executed_at_ts = _parse_time_to_epoch(getattr(args, "executed_at", None))
        exit_at_ts = _parse_time_to_epoch(getattr(args, "exit_at", None))

        LOG.info(
            "Recording trade for signal_id=%s executed_at=%s exit_at=%s executed_price=%s exit_price=%s pnl=%s outcome=%s",
            args.signal_id,
            executed_at_ts,
            exit_at_ts,
            args.executed_price,
            args.exit_price,
            args.pnl,
            args.outcome,
        )
        try:
            trade_id = orchestrator.signal_manager.record_trade_result(
                signal_id=int(args.signal_id),
                executed_at_ts=executed_at_ts,
                executed_price=float(args.executed_price),
                exit_at_ts=exit_at_ts,
                exit_price=float(args.exit_price),
                pnl=float(args.pnl),
                outcome=str(args.outcome),
                notes=str(args.notes) if getattr(args, "notes", None) else None,
            )
            LOG.info("Recorded trade; id=%s", trade_id)
            return 0
        except Exception:
            LOG.exception("Failed to record trade for signal_id=%s", args.signal_id)
            return 1

    if args.command == "version":
        try:
            from simple_trader import version  # type: ignore
        except Exception:
            version = "unknown"
        LOG.info("Simple-Trader version: %s", version)
        return 0

    if args.command == "run":
        interval = getattr(args, "interval", 300)
        max_news = getattr(args, "max_news", 100)
        try:
            orchestrator.run_periodic(interval_seconds=interval, max_news=max_news)
        except KeyboardInterrupt:
            LOG.info("Interrupted by user")
        return 0

    # If no command or unknown command provided, print help
    LOG.info("No command given. Use --help for usage instructions.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
