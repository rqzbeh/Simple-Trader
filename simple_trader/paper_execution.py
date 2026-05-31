# Simple-Trader/simple_trader/paper_execution.py
# Paper-execution simulator to run signals in 'paper' (simulated) mode and record results.
#
# This module is intentionally conservative and best-effort:
# - It treats signals as executed at their requested entry price (optionally adjusted for slippage).
# - It simulates hitting TP/SL using OHLC series from MarketDataClient (1H resolution by default).
# - If both TP and SL are touched in the same candle, it uses distance-from-entry heuristics
#   to decide which one was hit first (approximate).
# - When a simulated exit occurs, it calls SignalManager.record_trade_result(...) to close the
#   signal and record a trade outcome, so the rest of the system can learn from it.
#
# Usage:
#   from simple_trader.paper_execution import PaperExecutor
#   pe = PaperExecutor(config=CONFIG)
#   pe.simulate_open_signals_once()
#
# NOTE:
# - This tool is designed for conservative simulation/paper runs (e.g., on a 'paper' account).
# - It is not a full order-book accurate engine; consider it a higher-resolution 'backtest-like'
#   simulation that uses OHLC data to detect likely TP/SL hits and to produce trade records for learning.
#

from __future__ import annotations

import logging
from dataclasses import dataclass
from datetime import datetime, timezone
from math import isfinite
from typing import Any, Dict, List, Optional, Tuple

from .config import CONFIG, Config
from .db import Database, get_default_db, now_ts
from .market_data import MarketDataClient
from .signal_manager import SignalManager

logger = logging.getLogger("simple_trader.paper_execution")
logger.addHandler(logging.NullHandler())


def _parse_iso_to_ts(iso_ts: Optional[str]) -> Optional[int]:
    """Parse an ISO-format timestamp into an integer epoch seconds (UTC)."""
    if not iso_ts:
        return None
    try:
        dt = datetime.fromisoformat(iso_ts)
        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=timezone.utc)
        return int(dt.timestamp())
    except Exception:
        try:
            # fallback: try to parse as a float epoch string
            return int(float(iso_ts))
        except Exception:
            return None


@dataclass
class PaperExecutor:
    """
    Simulates the execution of open signals using available market OHLC data.

    Key configuration options:
    - config: Optional custom Config. Defaults to `CONFIG`.
    - db: Optional Database instance.
    - market_client: Optional MarketDataClient; will be created if None.
    - signal_manager: Optional SignalManager - used to record trade results (preferred for consistency).
    - simulate_slippage_pct: optional override for slippage used on entries & exits (defaults to config.stop_loss_slippage_pct).
    """

    config: Config | None = None
    db: Database | None = None
    market_client: MarketDataClient | None = None
    signal_manager: SignalManager | None = None
    simulate_slippage_pct: Optional[float] = None

    def __post_init__(self) -> None:
        self.config = self.config or CONFIG
        self.db = self.db or get_default_db(
            self.config.database_path, tenant_id=self.config.tenant_id
        )
        self.market_client = self.market_client or MarketDataClient(
            self.config, self.db
        )
        self.signal_manager = self.signal_manager or SignalManager(
            config=self.config, db=self.db, market_client=self.market_client
        )
        if self.simulate_slippage_pct is None:
            # default tolerance for slippage if not provided explicitly
            self.simulate_slippage_pct = float(
                getattr(self.config, "stop_loss_slippage_pct", 0.001)
            )

    def simulate_open_signals_once(
        self, limit: int = 500, lookback_hours: Optional[int] = None
    ) -> List[int]:
        """
        Simulate fills for currently open signals and record outcomes.
        Returns list of trade id's (or created trade record ids) for successfully recorded results.

        Args:
            limit: number of open signals to consider (pass `limit=0` or a large number to consider all).
            lookback_hours: optional override for how many hours of OHLC to fetch for simulation; when
                None, a conservative default window is used derived from config.max_trade_duration_hours.
        """
        created_trade_ids: List[int] = []
        try:
            open_signals = self.db.get_open_signals(limit=limit)
        except Exception:
            logger.exception("Failed to load open signals from DB")
            return created_trade_ids

        for row in open_signals:
            try:
                tid = self._simulate_single_signal(row, lookback_hours=lookback_hours)
                if tid:
                    created_trade_ids.append(tid)
            except Exception:
                logger.exception(
                    "Simulation for signal %s failed unexpectedly", row.get("id")
                )
        return created_trade_ids

    def _simulate_single_signal(
        self, signal_row: Dict[str, Any], lookback_hours: Optional[int] = None
    ) -> Optional[int]:
        """
        Simulate one signal. Detect TP/SL hits from 1H OHLC data after the signal `created_at`.
        If neither is hit within the signal's expiry window, it records a 'timeout'.
        """
        sig_id = int(signal_row["id"])
        symbol = str(signal_row["symbol"]).strip().upper()
        side = str(signal_row["side"]).lower()
        try:
            entry_price = float(signal_row["entry_price"])
            stop_loss = float(signal_row["stop_loss"])
            take_profit = float(signal_row["take_profit"])
        except Exception:
            logger.debug("Signal %s has invalid price data; skipping", sig_id)
            return None

        created_at_iso = signal_row.get("created_at")
        created_ts = _parse_iso_to_ts(created_at_iso) or now_ts()
        expires_iso = signal_row.get("expires_at")
        expired_ts = _parse_iso_to_ts(expires_iso)

        position_size = signal_row.get("position_size")
        if position_size is None:
            # Compute a position size consistent with the original sizing logic:
            try:
                risk_amount = float(signal_row.get("risk_amount") or 0.0)
                if not risk_amount or not isfinite(risk_amount) or risk_amount <= 0:
                    # fallback to using config risk settings
                    risk_amount = float(
                        self.config.account_balance_usd * self.config.risk_per_trade_pct
                    )
                price_diff = abs(entry_price - stop_loss)
                if price_diff <= 0:
                    logger.debug(
                        "Signal %s has invalid entry/stop distance; skip sizing", sig_id
                    )
                    return None
                position_size = risk_amount / price_diff
            except Exception:
                logger.exception(
                    "Failed to compute position size for signal %s", sig_id
                )
                return None
        else:
            position_size = float(position_size)

        # Apply entry slippage: for longs we assume buying slightly above the requested entry,
        # for shorts slightly below the requested entry.
        slippage = float(self.simulate_slippage_pct or 0.0)
        if side == "long":
            executed_price = entry_price * (1.0 + slippage)
        elif side == "short":
            executed_price = entry_price * (1.0 - slippage)
        else:
            logger.debug(
                "Unsupported signal side=%s for signal=%s; skipping", side, sig_id
            )
            return None

        executed_ts = created_ts

        # Determine an OHLC lookback window: ensure we fetch enough history to check TP/SL.
        # Respect an override passed in the method call, but enforce a conservative minimum to
        # avoid spurious early exits on small lookbacks.
        if lookback_hours is None:
            lookback_hours = max(24, int(self.config.max_trade_duration_hours) + 12)
        else:
            # Ensure a sensible lower bound (24 hours) even when caller passes a smaller value.
            try:
                lookback_hours = max(24, int(lookback_hours))
            except Exception:
                lookback_hours = max(24, int(self.config.max_trade_duration_hours) + 12)

        # Fetch 1H candles (we use 1H as a compromise between granularity and data volume)
        try:
            df = self.market_client.get_ohlc(
                symbol, timeframe_hours=1, lookback_hours=lookback_hours
            )
        except Exception:
            logger.exception(
                "Failed to fetch OHLC data for symbol=%s in simulation for signal=%s",
                symbol,
                sig_id,
            )
            return None

        if df is None or df.empty:
            logger.debug(
                "No OHLC data for %s - skipping simulation for signal %s",
                symbol,
                sig_id,
            )
            return None

        # Filter to candles at or after the execution time so we simulate forward ticks
        try:
            ts_dt = datetime.fromtimestamp(executed_ts, tz=timezone.utc)
            df_forward = df[df.index >= ts_dt]
            if df_forward.empty:
                # Not enough data after signal; consider defaulting to now and skip
                logger.debug(
                    "No forward OHLC rows after signal creation for symbol=%s signal=%s",
                    symbol,
                    sig_id,
                )
                # If the signal is already expired, register as timeout
                if expired_ts is not None and expired_ts < now_ts():
                    return self._record_timeout_for_signal(
                        sig_id, executed_ts, executed_price, signal_row
                    )
                return None
        except Exception:
            logger.exception(
                "Failed to filter OHLC by creation time for signal %s", sig_id
            )
            return None

        # Iterate and detect TP/SL
        has_exit = False
        exit_price = None
        exit_ts = None
        outcome = "timeout"
        for candle_ts, row in df_forward.iterrows():
            # row is a pandas Series keyed by 'open', 'high', 'low', 'close', 'volume'
            try:
                high = float(row["high"]) if row["high"] is not None else None
                low = float(row["low"]) if row["low"] is not None else None
            except Exception:
                logger.debug("Malformed OHLC values for %s row %s", symbol, candle_ts)
                continue

            # Evaluate candidate hits based on side
            # Note: if both high >= TP and low <= SL in the same candle, choose the more likely/close one.
            tp_hit = False
            sl_hit = False
            if side == "long":
                if high is not None and take_profit is not None and high >= take_profit:
                    tp_hit = True
                if low is not None and stop_loss is not None and low <= stop_loss:
                    sl_hit = True
            elif side == "short":
                if low is not None and take_profit is not None and low <= take_profit:
                    tp_hit = True
                if high is not None and stop_loss is not None and high >= stop_loss:
                    sl_hit = True

            chosen = None
            if tp_hit and sl_hit:
                # Rough heuristic: which distance is closer from entry? that one is considered hit first.
                distance_to_tp = (
                    abs(take_profit - executed_price)
                    if take_profit is not None
                    else float("inf")
                )
                distance_to_sl = (
                    abs(executed_price - stop_loss)
                    if stop_loss is not None
                    else float("inf")
                )
                if distance_to_tp <= distance_to_sl:
                    chosen = "tp"
                else:
                    chosen = "sl"
            elif tp_hit:
                chosen = "tp"
            elif sl_hit:
                chosen = "sl"

            if chosen is not None:
                # decide exit price with conservative slippage assumptions:
                if chosen == "tp":
                    # assume we can exit at the requested TP or at the available high
                    exit_price_raw = (
                        min(take_profit, high)
                        if high is not None and take_profit is not None
                        else high or take_profit
                    )
                    # When closing a long, we might get a slightly worse fill; apply a small negative slippage to wins
                    if side == "long":
                        exit_price = exit_price_raw * (1.0 - slippage * 0.2)
                    else:
                        exit_price = exit_price_raw * (1.0 + slippage * 0.2)
                    outcome = "win"
                else:
                    # SL chosen
                    exit_price_raw = (
                        max(stop_loss, low)
                        if low is not None and stop_loss is not None
                        else low or stop_loss
                    )
                    # When hitting stop loss, we may suffer slippage that worsens the outcome
                    if side == "long":
                        exit_price = exit_price_raw * (1.0 - slippage * 0.5)
                    else:
                        exit_price = exit_price_raw * (1.0 + slippage * 0.5)
                    outcome = "loss"

                # Build timestamp from candle's index
                exit_ts = int(candle_ts.timestamp())
                has_exit = True
                break

            # If we reach here and the signal is expired, record a timeout
            if expired_ts and int(candle_ts.timestamp()) > expired_ts:
                # Timeout: no PnL as signal never hit a TP or SL in its life
                return self._record_timeout_for_signal(
                    sig_id, executed_ts, executed_price, signal_row
                )

        # If no TP/SL was found in the available forward series but the signal has expired, close as timeout
        if not has_exit:
            # check if expired
            now_t = now_ts()
            if expired_ts is not None and expired_ts <= now_t:
                return self._record_timeout_for_signal(
                    sig_id, executed_ts, executed_price, signal_row
                )
            # Otherwise no decision yet (still open) -> nothing to do
            return None

        # Compute pnl
        # For long: pnl = position_size * (exit - entry)
        # For short: pnl = position_size * (entry - exit)
        try:
            if exit_price is None:
                # Unexpected null
                logger.warning(
                    "Exit price unexpectedly None for signal %s (sid:%s)",
                    symbol,
                    sig_id,
                )
                return None
            if side == "long":
                pnl = position_size * (float(exit_price) - float(executed_price))
            else:
                pnl = position_size * (float(executed_price) - float(exit_price))
        except Exception:
            logger.exception("Failed to compute pnl for signal %s", sig_id)
            pnl = 0.0

        # Round and finalize outputs
        try:
            # Use the signal_manager to record trade result; it will close the signal and persist the trade
            trade_id = self.signal_manager.record_trade_result(
                signal_id=sig_id,
                executed_at_ts=int(executed_ts),
                executed_price=float(executed_price),
                exit_at_ts=int(exit_ts) if exit_ts else now_ts(),
                exit_price=float(exit_price),
                pnl=float(pnl),
                outcome=outcome,
                notes="paper-execution simulation",
            )
            return trade_id
        except Exception:
            logger.exception("Failed to record simulated trade for signal %s", sig_id)
            return None

    def _record_timeout_for_signal(
        self,
        sig_id: int,
        executed_ts: int,
        executed_price: float,
        signal_row: Dict[str, Any],
    ) -> Optional[int]:
        """
        Convenience helper to record a timeout close and a trade (zero pnl) if the
        signal expired without TP/SL being hit.
        """
        try:
            now_ts_val = now_ts()
            # Close signal & add a timeout trade record with estimated entry price
            trade_id = self.signal_manager.record_trade_result(
                signal_id=sig_id,
                executed_at_ts=int(executed_ts),
                executed_price=float(executed_price),
                exit_at_ts=int(now_ts_val),
                exit_price=None,
                pnl=0.0,
                outcome="timeout",
                notes="paper-execution timeout",
            )
            return trade_id
        except Exception:
            logger.exception("Failed to record timeout trade for signal %s", sig_id)
            return None
