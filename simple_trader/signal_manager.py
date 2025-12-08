# -*- coding: utf-8 -*-
"""
Signal manager to combine LLM analysis and candlestick patterns into actionable trade signals.

This module orchestrates:
- taking unprocessed news from the DB
- asking LLM pool(s) to analyze them
- checking 2h candlestick patterns for the asset(s)
- combining LLM sentiment / direction and pattern-based set-ups
- computing entry, SL, TP, leverage, position size and verifying RR >= config.min_risk_reward_ratio
- writing signals to the DB and optionally sending them to Telegram
- offering a lightweight StrategyTuner that can update tuning stats from closed trades

Key classes:
- SignalManager: main orchestrator
- StrategyTuner: small helper to update stats and recommend parameter adjustments
"""

from __future__ import annotations

import logging
import math
import time
from dataclasses import asdict
from datetime import datetime, timedelta, timezone
from typing import Any, Dict, List, Optional, Tuple

from .config import CONFIG, Config
from .db import (
    Database,
    NewsItem,
    TradeRecord,
    get_default_db,
    now_ts,
)
from .db import (
    Signal as SignalDataclass,
)
from .llm_pool import LLMPool, LLMSummary
from .market_data import MarketDataClient, _symbol_is_forex
from .metrics import (
    record_news_processed,
    record_signal_closed,
    record_signal_created,
    record_trade_recorded,
    set_open_signals,
)
from .news_fetcher import NewsFetcher
from .pattern_detector import (
    PatternDetector,
    PatternMatch,
    detect_patterns_and_suggest_trades,
)
from .scorer import SignalScorer
from .telegram import TelegramNotifier

logger = logging.getLogger("simple_trader.signal_manager")
logger.addHandler(logging.NullHandler())


class StrategyTuner:
    """
    Very lightweight tuner that updates pattern/symbol success stats in DB
    and offers a small set of heuristic suggestions (not automatic parameter changes).
    """

    def __init__(self, db: Optional[Database] = None, config: Optional[Config] = None):
        self.config = config or CONFIG
        self.db = db or get_default_db(self.config.database_path)

    def update_from_trade(
        self,
        pattern_name: str,
        symbol: str,
        win: bool,
        rr: Optional[float],
        hold_seconds: Optional[float],
    ):
        """
        Update defaut tuning stats recorded in DB for this pattern+symbol.
        """
        try:
            self.db.update_tuning_stat(pattern_name, symbol, win, rr, hold_seconds)
        except Exception:
            logger.exception(
                "Strategy tuner failed to update stats for %s %s", pattern_name, symbol
            )

    def get_summary_for_pattern(self, pattern_name: str, symbol: Optional[str] = None):
        row = self.db.get_tuning_stat(pattern_name, symbol)
        return dict(row) if row else None

    def suggest_adjustments(
        self,
        min_win_rate: float = 0.4,
        min_avg_rr: float = 3.0,
        min_sample_size: int = 10,
    ) -> List[Dict[str, Any]]:
        """
        Generate operational suggestions based on aggregated `tuning_stats` stored in DB.

        This method inspects the `tuning_stats` table and returns a list of suggestions,
        each suggesting a conservative, actionable change when one or more heuristics
        identify that a pattern or pattern+symbol combination is underperforming.

        Heuristics:
        - 'insufficient_samples': If wins+losses < min_sample_size, mark as insufficient data.
        - 'low_win_rate': If win rate < min_win_rate, indicate the pattern might need de-prioritization.
        - 'low_avg_rr': If average RR < min_avg_rr, indicate the pattern is not meeting RR expectations.
        - 'long_holds': If average hold time (seconds) > configured max trade duration, it indicates
                        the pattern habitually leads to longer holds than allowed by policy.
        - 'unstable': If win-rate is near threshold with few samples, mark as marginal/unstable.

        Returns:
            A list of dicts like:
                {
                    "pattern_name": str,
                    "symbol": Optional[str],
                    "issue": str,
                    "wins": int,
                    "losses": int,
                    "win_rate": Optional[float],
                    "avg_rr": Optional[float],
                    "avg_hold_seconds": Optional[float],
                    "message": str
                }
        """
        suggestions: List[Dict[str, Any]] = []
        try:
            rows = self.db.execute_custom(
                "SELECT * FROM tuning_stats ORDER BY last_updated DESC"
            )
            for r in rows:
                wins = int(r.get("wins") or 0)
                losses = int(r.get("losses") or 0)
                n = wins + losses
                avg_rr = float(r.get("avg_rr") or 0.0)
                avg_hold = float(r.get("avg_hold_time_seconds") or 0.0)
                symbol = r.get("symbol")
                pattern_name = r.get("pattern_name")

                # Insufficient samples: this is not a negative signal, just informational
                if n < min_sample_size:
                    suggestions.append(
                        {
                            "pattern_name": pattern_name,
                            "symbol": symbol,
                            "issue": "insufficient_samples",
                            "wins": wins,
                            "losses": losses,
                            "win_rate": None if n == 0 else wins / n,
                            "avg_rr": avg_rr,
                            "avg_hold_seconds": avg_hold,
                            "message": f"Only {n} sampled trades for '{pattern_name}' on '{symbol}'; accumulate more data before relying on it.",
                        }
                    )
                    # skip additional checks for very low-sample patterns
                    continue

                # Win rate based suggestion
                win_rate: Optional[float] = None
                if n > 0:
                    win_rate = float(wins) / float(n)
                    if win_rate < min_win_rate:
                        suggestions.append(
                            {
                                "pattern_name": pattern_name,
                                "symbol": symbol,
                                "issue": "low_win_rate",
                                "wins": wins,
                                "losses": losses,
                                "win_rate": win_rate,
                                "avg_rr": avg_rr,
                                "avg_hold_seconds": avg_hold,
                                "message": f"Win rate for '{pattern_name}' on '{symbol}' is {win_rate:.2f} which is below the threshold {min_win_rate:.2f}. Consider increasing required confidence for signals, limiting position sizes, or pausing this pattern/symbol.",
                            }
                        )

                # Avg RR
                if avg_rr and avg_rr < min_avg_rr:
                    suggestions.append(
                        {
                            "pattern_name": pattern_name,
                            "symbol": symbol,
                            "issue": "low_avg_rr",
                            "wins": wins,
                            "losses": losses,
                            "win_rate": win_rate,
                            "avg_rr": avg_rr,
                            "avg_hold_seconds": avg_hold,
                            "message": f"Average RR for '{pattern_name}' on '{symbol}' is {avg_rr:.2f} (below {min_avg_rr}). Consider increasing TP targets, adjusting SL logic, or decreasing leverage.",
                        }
                    )

                # Avg hold time longer than configured max trade duration
                if avg_hold and avg_hold > (
                    self.config.max_trade_duration_hours * 3600
                ):
                    suggestions.append(
                        {
                            "pattern_name": pattern_name,
                            "symbol": symbol,
                            "issue": "long_holds",
                            "wins": wins,
                            "losses": losses,
                            "win_rate": win_rate,
                            "avg_rr": avg_rr,
                            "avg_hold_seconds": avg_hold,
                            "message": f"Average hold time for '{pattern_name}' on '{symbol}' is {avg_hold:.1f}s which exceeds the max trade duration ({self.config.max_trade_duration_hours}h). Consider enforcing earlier closes or tighter TP/SL rules.",
                        }
                    )

                # Mixed indicators near thresholds -> "unstable"
                if (
                    win_rate is not None
                    and min_win_rate <= win_rate <= min_win_rate + 0.05
                    and n < (min_sample_size * 3)
                ):
                    suggestions.append(
                        {
                            "pattern_name": pattern_name,
                            "symbol": symbol,
                            "issue": "unstable",
                            "wins": wins,
                            "losses": losses,
                            "win_rate": win_rate,
                            "avg_rr": avg_rr,
                            "avg_hold_seconds": avg_hold,
                            "message": f"'{pattern_name}' on '{symbol}' looks marginal with win rate {win_rate:.2f} across {n} trades. Consider temporarily increasing its confidence requirement or monitoring more closely.",
                        }
                    )
        except Exception:
            logger.exception("Error while computing suggestions from tuning_stats")
        return suggestions

    def apply_adjustments(
        self,
        suggestions: List[Dict[str, Any]],
        auto_apply: bool = True,
        pattern_conf_step: float = 0.05,
        leverage_reduce_pct: float = 0.9,
        disable_threshold: float = 0.15,
        disable_min_samples: int = 50,
    ) -> List[Dict[str, Any]]:
        """
        Apply suggested adjustments to runtime parameters and log them in tuning_history.
        Returns a list of applied adjustments for auditing.

        This function modifies runtime parameters using `db.set_runtime_param`, writes
        historical records to tuning_history via `db.add_tuning_history_entry`, and returns
        a list describing what changes were applied.

        Safety measures:
        - Only modify a small step at a time (pattern_conf_step).
        - For extremely poor performance (win_rate < disable_threshold and sample size large),
          the function may disable a pattern entirely.
        - Reduced leverage is applied by a small multiplicative factor (leverage_reduce_pct).
        - Suggestions classified as 'insufficient_samples' are not auto-applied.
        """
        applied_changes: List[Dict[str, Any]] = []
        if not suggestions:
            return applied_changes

        # Guard: require DB and config to exist; return no-op if missing
        if not self.db:
            logger.warning("No DB instance present; cannot apply adjustments")
            return applied_changes

        for s in suggestions:
            try:
                pattern = s.get("pattern_name")
                symbol = s.get("symbol")
                issue = s.get("issue")
                wins = int(s.get("wins") or 0)
                losses = int(s.get("losses") or 0)
                n = wins + losses
                win_rate = s.get("win_rate")
                avg_rr_val = s.get("avg_rr")

                # Only auto-apply if explicitly enabled or passed in
                if not auto_apply:
                    continue

                # 1) low_win_rate -> increase per-pattern min_confidence, or disable if very poor
                if issue == "low_win_rate" and pattern:
                    key_conf = f"pattern:{pattern}:min_confidence"
                    curr_min_conf = self.db.get_runtime_param_as_float(
                        key_conf, self.config.min_pattern_confidence
                    )
                    if (
                        win_rate is not None
                        and win_rate < disable_threshold
                        and n >= disable_min_samples
                    ):
                        # Disable the pattern in runtime params
                        self.db.set_runtime_param(
                            f"pattern:{pattern}:enabled",
                            "false",
                            f"Auto-disabled due to low win rate ({win_rate:.2f})",
                        )
                        self.db.add_tuning_history_entry(
                            pattern,
                            symbol,
                            "disable_pattern",
                            str(curr_min_conf),
                            "false",
                            notes="Auto-disabled by StrategyTuner",
                            applied_by="StrategyTuner",
                        )
                        applied_changes.append(
                            {
                                "pattern": pattern,
                                "action": "disable_pattern",
                                "reason": "low_win_rate",
                                "win_rate": win_rate,
                                "samples": n,
                            }
                        )
                    else:
                        new_min_conf = min(0.99, curr_min_conf + pattern_conf_step)
                        self.db.set_runtime_param(
                            key_conf,
                            str(new_min_conf),
                            f"Auto-increased min_confidence due to low win rate ({win_rate})",
                        )
                        self.db.add_tuning_history_entry(
                            pattern,
                            symbol,
                            "adjust_pattern_min_confidence",
                            str(curr_min_conf),
                            str(new_min_conf),
                            notes="Auto-adjusted by StrategyTuner",
                            applied_by="StrategyTuner",
                        )
                        applied_changes.append(
                            {
                                "pattern": pattern,
                                "action": "increase_min_confidence",
                                "old": curr_min_conf,
                                "new": new_min_conf,
                                "reason": "low_win_rate",
                            }
                        )

                # 2) low_avg_rr -> reduce leverage globally (conservative measure)
                if issue == "low_avg_rr":
                    curr_leverage_crypto = self.db.get_runtime_param_as_int(
                        "max_leverage_crypto", self.config.max_leverage_crypto
                    )
                    curr_leverage_forex = self.db.get_runtime_param_as_int(
                        "max_leverage_forex", self.config.max_leverage_forex
                    )
                    new_leverage_crypto = max(
                        1, int(math.floor(curr_leverage_crypto * leverage_reduce_pct))
                    )
                    new_leverage_forex = max(
                        1, int(math.floor(curr_leverage_forex * leverage_reduce_pct))
                    )
                    if new_leverage_crypto != curr_leverage_crypto:
                        self.db.set_runtime_param(
                            "max_leverage_crypto",
                            str(new_leverage_crypto),
                            "Auto-adjusted due to low avg RR patterns",
                        )
                        self.db.add_tuning_history_entry(
                            pattern,
                            symbol,
                            "adjust_max_leverage_crypto",
                            str(curr_leverage_crypto),
                            str(new_leverage_crypto),
                            "Auto-adjusted by StrategyTuner, low_avg_rr",
                            applied_by="StrategyTuner",
                        )
                        applied_changes.append(
                            {
                                "pattern": pattern,
                                "action": "reduce_max_leverage_crypto",
                                "old": curr_leverage_crypto,
                                "new": new_leverage_crypto,
                                "reason": "low_avg_rr",
                            }
                        )
                    if new_leverage_forex != curr_leverage_forex:
                        self.db.set_runtime_param(
                            "max_leverage_forex",
                            str(new_leverage_forex),
                            "Auto-adjusted due to low avg RR patterns",
                        )
                        self.db.add_tuning_history_entry(
                            pattern,
                            symbol,
                            "adjust_max_leverage_forex",
                            str(curr_leverage_forex),
                            str(new_leverage_forex),
                            "Auto-adjusted by StrategyTuner, low_avg_rr",
                            applied_by="StrategyTuner",
                        )
                        applied_changes.append(
                            {
                                "pattern": pattern,
                                "action": "reduce_max_leverage_forex",
                                "old": curr_leverage_forex,
                                "new": new_leverage_forex,
                                "reason": "low_avg_rr",
                            }
                        )

                # 3) long_holds -> reduce pattern-specific max trade duration
                if issue == "long_holds" and pattern:
                    key_duration = f"pattern:{pattern}:max_trade_duration_hours"
                    curr_duration = self.db.get_runtime_param_as_int(
                        key_duration, self.config.max_trade_duration_hours
                    )
                    new_duration = max(
                        1, curr_duration - 2
                    )  # reduce 2 hours conservatively
                    if new_duration != curr_duration:
                        self.db.set_runtime_param(
                            key_duration,
                            str(new_duration),
                            "Auto-adjusted due to long average holds",
                        )
                        self.db.add_tuning_history_entry(
                            pattern,
                            symbol,
                            "adjust_pattern_max_duration",
                            str(curr_duration),
                            str(new_duration),
                            "Auto-adjusted by StrategyTuner",
                            applied_by="StrategyTuner",
                        )
                        applied_changes.append(
                            {
                                "pattern": pattern,
                                "action": "reduce_max_duration",
                                "old": curr_duration,
                                "new": new_duration,
                                "reason": "long_holds",
                            }
                        )

                # 4) unstable -> small increase in pattern min_confidence
                if issue == "unstable" and pattern:
                    key_conf = f"pattern:{pattern}:min_confidence"
                    curr_min_conf = self.db.get_runtime_param_as_float(
                        key_conf, self.config.min_pattern_confidence
                    )
                    new_min_conf = min(0.99, curr_min_conf + (pattern_conf_step / 2.0))
                    self.db.set_runtime_param(
                        key_conf,
                        str(new_min_conf),
                        "Auto-adjusted small for unstable patterns",
                    )
                    self.db.add_tuning_history_entry(
                        pattern,
                        symbol,
                        "adjust_pattern_min_confidence_small",
                        str(curr_min_conf),
                        str(new_min_conf),
                        "Auto-adjusted by StrategyTuner",
                        applied_by="StrategyTuner",
                    )
                    applied_changes.append(
                        {
                            "pattern": pattern,
                            "action": "small_adjust_min_confidence",
                            "old": curr_min_conf,
                            "new": new_min_conf,
                            "reason": "unstable",
                        }
                    )

            except Exception:
                logger.exception(
                    "Failed to apply adjustment for suggestion: %s", (s or {})
                )

        return applied_changes


class SignalManager:
    """
    Orchestrates the entire signal generation pipeline.

    Usage:
       sm = SignalManager(config=CONFIG)
       sm.process_unprocessed_news()
    """

    def __init__(
        self,
        config: Optional[Config] = None,
        db: Optional[Database] = None,
        news_fetcher: Optional[NewsFetcher] = None,
        llm_pool: Optional[LLMPool] = None,
        market_client: Optional[MarketDataClient] = None,
        telegram_notifier: Optional[TelegramNotifier] = None,
    ):
        self.config = config or CONFIG
        self.db = db or get_default_db(self.config.database_path)
        self.news_fetcher = news_fetcher or NewsFetcher(self.config, self.db)
        self.llm_pool = llm_pool or LLMPool(self.config, self.db)
        self.market = market_client or MarketDataClient(self.config, self.db)
        self.pattern_detector = PatternDetector(self.config)
        self.telegram = telegram_notifier or TelegramNotifier(
            config=self.config, db=self.db
        )
        self.tuner = StrategyTuner(db=self.db, config=self.config)
        self.scorer = SignalScorer(
            db=self.db, config=self.config, market_client=self.market
        )

    def process_unprocessed_news(self, limit: int = 100) -> List[int]:
        """
        Main entry: fetch unprocessed news from DB (default limit), analyze them via LLMs,
        generate signals and optionally push to Telegram.

        Returns:
            List of created signal IDs.
        """
        created_signal_ids = []
        unprocessed_rows = self.db.get_unprocessed_news(limit=limit)
        if not unprocessed_rows:
            logger.debug("No unprocessed news to process")
            return created_signal_ids

        # Convert DB rows to NewsItem dataclasses for LLM client
        news_items: List[NewsItem] = []
        mapping_idx_to_rowid: Dict[int, int] = {}
        for i, r in enumerate(unprocessed_rows):
            ni = NewsItem(
                provider=r["provider"],
                url=r["url"],
                title=r["title"],
                content=r["content"],
                published_at=int(datetime.fromisoformat(r["published_at"]).timestamp())
                if r["published_at"]
                else None,
                asset=r["asset"],
                raw_json=None,
                fetched_at=now_ts(),
            )
            news_items.append(ni)
            mapping_idx_to_rowid[i] = int(r["id"])

        # Read runtime parameters (overlays on config) that can be adjusted live in the DB.
        # These values override the immutable ENV config where present and enable dynamic tuning.
        llm_min_conf = float(
            self.db.get_runtime_param_as_float(
                "llm_min_confidence", self.config.llm_min_confidence
            )
        )
        min_pattern_conf = float(
            self.db.get_runtime_param_as_float(
                "min_pattern_confidence", self.config.min_pattern_confidence
            )
        )
        min_rr = float(
            self.db.get_runtime_param_as_float(
                "min_risk_reward_ratio", self.config.min_risk_reward_ratio
            )
        )
        risk_pct = float(
            self.db.get_runtime_param_as_float(
                "risk_per_trade_pct", self.config.risk_per_trade_pct
            )
        )
        open_positions_limit = int(
            self.db.get_runtime_param_as_int(
                "open_positions_limit", self.config.open_positions_limit
            )
        )
        max_lev_crypto = int(
            self.db.get_runtime_param_as_int(
                "max_leverage_crypto", self.config.max_leverage_crypto
            )
        )
        max_lev_forex = int(
            self.db.get_runtime_param_as_int(
                "max_leverage_forex", self.config.max_leverage_forex
            )
        )

        logger.info(
            "Sending %s news items to LLM pool for analysis (llm_min_conf=%s, pattern_min_conf=%s)",
            len(news_items),
            llm_min_conf,
            min_pattern_conf,
        )
        analysis_pairs: List[Tuple[LLMSummary, Optional[int]]] = (
            self.llm_pool.analyze_and_store(
                news_items, only_store_confidence_min=llm_min_conf
            )
        )

        # For each analyzed result, try to produce signal(s)
        for i, (summ, analysis_id) in enumerate(analysis_pairs):
            news_rowid = mapping_idx_to_rowid.get(i)
            if summ is None:
                logger.debug(
                    "LLM summary missing for news idx=%s; marking as processed", i
                )
                if news_rowid:
                    self.db.mark_news_processed(news_rowid, processed=True)
                continue

            # Prefer the asset recommended by LLM; fallback to news asset
            asset_symbol = None
            if getattr(summ, "asset", None):
                asset_symbol = summ.asset
            else:
                # original news item
                item_guess = news_items[i].asset if news_items[i].asset else None
                asset_symbol = item_guess

            if not asset_symbol:
                logger.info(
                    "LLM didn't provide asset and none detected from news; mark as processed and continue"
                )
                if news_rowid:
                    self.db.mark_news_processed(news_rowid, processed=True)
                    try:
                        record_news_processed()
                    except Exception:
                        pass
                continue

            # Ensure normalized symbol: uppercased & trimmed
            asset_symbol = str(asset_symbol).upper().replace(" ", "")

            try:
                logger.debug(
                    "Preparing signals for news_id=%s asset=%s",
                    news_rowid,
                    asset_symbol,
                )
                # fetch 2h candles and detect patterns
                df_2h = self.market.get_2h_ohlc(
                    asset_symbol,
                    lookback_hours=max(48, self.config.timeframe_hours * 10),
                )
                if df_2h is None or df_2h.empty:
                    logger.debug(
                        "No market data for %s; skip signal generation", asset_symbol
                    )
                    if news_rowid:
                        self.db.mark_news_processed(news_rowid, processed=True)
                        try:
                            record_news_processed()
                        except Exception:
                            pass
                    continue
                patterns: List[PatternMatch] = self.pattern_detector.detect_patterns(
                    df_2h, lookback=24
                )
                if not patterns:
                    logger.info(
                        "No candlestick patterns found for %s; skipping", asset_symbol
                    )
                    # Optionally, we could still create a momentum-entry signal if LLM strongly confident
                    if news_rowid:
                        self.db.mark_news_processed(news_rowid, processed=True)
                        try:
                            record_news_processed()
                        except Exception:
                            pass
                    continue

                # For each pattern, combine LLM signal
                for p in patterns:
                    # Convert pattern.direction to LLM-like 'long'/'short'
                    p_direction = p.direction
                    llm_direction = getattr(summ, "direction", "neutral")
                    # Determine if pattern and LLMSummary align sufficiently
                    do_create = False
                    # Basic rule: both must match and both have minimum confidence
                    llm_conf = float(getattr(summ, "confidence", 0.0) or 0.0)
                    pattern_conf = float(getattr(p, "confidence", 0.0) or 0.0)

                    # Consult runtime per-pattern params (pattern-specific minimum confidence and enabled flag)
                    per_pattern_min_conf = float(
                        self.db.get_runtime_param_as_float(
                            f"pattern:{p.pattern_name}:min_confidence", min_pattern_conf
                        )
                    )
                    pattern_enabled = self.db.get_runtime_param_as_bool(
                        f"pattern:{p.pattern_name}:enabled", True
                    )
                    if not pattern_enabled:
                        logger.info(
                            "Pattern %s disabled in runtime params; skipping",
                            p.pattern_name,
                        )
                        continue
                    if pattern_conf < per_pattern_min_conf:
                        logger.debug(
                            "Pattern %s confidence %.2f < min (%.2f); skipping",
                            p.pattern_name,
                            pattern_conf,
                            per_pattern_min_conf,
                        )
                        continue

                    # Market session check (for forex)
                    session_ok = True
                    if _symbol_is_forex(asset_symbol):
                        session_ok = self._is_pref_session_ok(asset_symbol)
                        if not session_ok:
                            # if session mismatch reduce llm confidence a bit
                            llm_conf *= 0.8

                    # Check directional alignment strong-first rule
                    if llm_conf >= llm_min_conf and llm_direction == p_direction:
                        do_create = True
                    else:
                        # allow creation if pattern confidence is high >= 0.95 and LLM is neutral
                        if (
                            llm_direction == "neutral"
                            and pattern_conf >= 0.95
                            and llm_conf >= 0.4
                        ):
                            do_create = True
                        # or if pattern and LLM disagree but both are strongly confident and LLM confident > 0.85,
                        # we prefer LLM as a macro indicator: only create if both have high confidence and pattern is minor disagreement
                        elif (
                            llm_conf >= 0.9
                            and pattern_conf >= 0.8
                            and llm_direction == p_direction
                        ):
                            do_create = True
                        else:
                            do_create = False

                    if not do_create:
                        logger.debug(
                            "Skipping pattern for %s: llm_conf=%.2f llm_dir=%s pattern_conf=%.2f patt_dir=%s session_ok=%s",
                            asset_symbol,
                            llm_conf,
                            llm_direction,
                            pattern_conf,
                            p_direction,
                            session_ok,
                        )
                        continue

                    # Validate prices available in pattern
                    if not (p.entry_hint and p.stop_loss and p.take_profit):
                        logger.debug("Pattern lacking price info; skip %s", p)
                        continue

                    # Calculate RR and ensure meets min
                    rr = p.rr or 0.0
                    if rr < min_rr:
                        # attempt to upgrade take profit to respect runtime-configured min RR
                        p = self._upgrade_pattern_rr(p, min_rr)
                        rr = p.rr or 0.0
                        if rr < min_rr:
                            logger.debug(
                                "Pattern RR after upgrade is still %.2f < required %.2f => skip",
                                rr,
                                min_rr,
                            )
                            continue

                    # Compute leverage (respecting config)
                    leverage = p.recommended_leverage or 1
                    if _symbol_is_forex(asset_symbol):
                        leverage = min(leverage, max_lev_forex)
                    else:
                        leverage = min(leverage, max_lev_crypto)
                    if leverage < 1:
                        leverage = 1

                    # Compute risk amount in USD and position sizing using current market price
                    account_balance = self.config.account_balance_usd
                    risk_usd = account_balance * risk_pct
                    entry_price = float(p.entry_hint)
                    stop_loss = float(p.stop_loss)
                    # For instrument quotes, if they have the base in USD then difference is in USD; else, it's approximate.
                    # For simplicity: compute risk per unit in asset-price units
                    price_diff = abs(entry_price - stop_loss)
                    if price_diff <= 0:
                        logger.debug(
                            "Price diff is 0; skip signal to avoid division by zero"
                        )
                        continue

                    position_asset_units = risk_usd / price_diff
                    position_value_usd = position_asset_units * entry_price

                    # Scoring check: compute signal score using SignalScorer if available.
                    # Build a candidate signal-like row for scoring (this mirrors fields used in the scorer)
                    candidate_signal_row = {
                        "news_id": news_rowid,
                        "symbol": asset_symbol,
                        "analysis_ids": [analysis_id]
                        if analysis_id is not None
                        else None,
                        "rr": rr,
                        "leverage": leverage,
                        "position_size": position_asset_units,
                        "entry_price": entry_price,
                    }
                    # runtime-configurable threshold for minimum signal score; default 0.6
                    score_threshold = float(
                        self.db.get_runtime_param_as_float(
                            "signal_score_threshold", 0.6
                        )
                    )
                    score = 0.0
                    try:
                        if getattr(self, "scorer", None) is not None:
                            # the scorer returns a probability that this would be a winning trade
                            score = float(
                                self.scorer.predict_proba_from_signal_row(
                                    candidate_signal_row
                                )
                                or 0.0
                            )
                    except Exception:
                        # If the scorer fails, just fall back to the default logic and continue safely.
                        logger.exception(
                            "Scorer prediction failed; continuing without scoring filter for %s",
                            asset_symbol,
                        )

                    if score < score_threshold:
                        logger.info(
                            "Signal candidate rejected by scorer for %s: score=%.3f < threshold=%.3f",
                            asset_symbol,
                            score,
                            score_threshold,
                        )
                        # Skip generating this signal candidate if under threshold
                        continue

                    # sanity: don't open a position bigger than account * leverage * 20 (just an extra guard)
                    max_notional_allowed = account_balance * (leverage) * 20
                    if position_value_usd > max_notional_allowed:
                        # reduce position to be safe: cap position value
                        scale = max_notional_allowed / position_value_usd
                        position_asset_units *= scale
                        position_value_usd *= scale

                    # Avoid duplicate open signals for same symbol/side in short time
                    recent_same = self.db.find_duplicate_signals(
                        asset_symbol, p_direction, within_seconds=3600
                    )
                    if recent_same:
                        logger.info(
                            "Found recent open signal for %s %s; skipping duplicate",
                            asset_symbol,
                            p_direction,
                        )
                        continue

                    # Enforce an overall open positions limit before creating any new signal.
                    # This avoids over-exposure when many signals are generated at once.
                    try:
                        open_count_rows = self.db.execute_custom(
                            "SELECT COUNT(*) AS c FROM signals WHERE status = 'open'"
                        )
                        open_count = (
                            int(open_count_rows[0]["c"]) if open_count_rows else 0
                        )
                    except Exception:
                        # If we can't fetch the open count (DB error), log and allow signal creation,
                        # because failing closed vs open database read should not block the generator.
                        logger.exception(
                            "Failed to fetch open positions count; allowing signal to proceed"
                        )
                        open_count = 0

                    if open_count >= open_positions_limit:
                        logger.info(
                            "Open positions limit reached (%s >= %s); skipping new signal for %s %s",
                            open_count,
                            open_positions_limit,
                            asset_symbol,
                            p_direction,
                        )
                        continue

                    # Build Signal dataclass and insert
                    created_at = now_ts()
                    # Allow per-pattern max duration override if set in runtime params, and respect global max as upper bound
                    per_pattern_max_hours = int(
                        self.db.get_runtime_param_as_int(
                            f"pattern:{p.pattern_name}:max_trade_duration_hours",
                            self.config.max_trade_duration_hours,
                        )
                    )
                    per_pattern_max_hours = int(
                        min(per_pattern_max_hours, self.config.max_trade_duration_hours)
                    )
                    expires_at = created_at + int(per_pattern_max_hours * 3600)
                    signal_obj = SignalDataclass(
                        news_id=news_rowid,
                        symbol=asset_symbol,
                        side=p_direction,
                        entry_price=entry_price,
                        stop_loss=stop_loss,
                        take_profit=p.take_profit,
                        leverage=leverage,
                        position_size=position_asset_units,
                        risk_amount=risk_usd,
                        rr=rr,
                        timeframe_hours=self.config.timeframe_hours,
                        created_at=created_at,
                        expires_at=expires_at,
                        analysis_ids=[analysis_id] if analysis_id is not None else None,
                    )
                    try:
                        signal_id = self.db.create_signal(signal_obj)
                        created_signal_ids.append(signal_id)
                        try:
                            record_signal_created(
                                asset_symbol, p_direction, p.pattern_name
                            )
                        except Exception:
                            pass
                        try:
                            set_open_signals(open_count + 1)
                        except Exception:
                            pass
                        logger.info(
                            "Created signal id=%s for %s %s (entry=%s sl=%s tp=%s rr=%.2f lev=%s)",
                            signal_id,
                            asset_symbol,
                            p_direction,
                            entry_price,
                            stop_loss,
                            p.take_profit,
                            rr,
                            leverage,
                        )
                        # send via Telegram unless backtest or disabled
                        try:
                            if (
                                self.config.telegram_bot_token
                                and self.config.telegram_chat_id
                                and not self.config.backtest_mode
                            ):
                                self.telegram.send_signal(signal_obj)
                        except Exception:
                            logger.exception(
                                "Failed to send signal via Telegram for signal_id=%s",
                                signal_id,
                            )
                    except Exception:
                        logger.exception("Failed to create signal for %s", asset_symbol)

                # mark the news as processed afterwards for this asset
                if news_rowid:
                    self.db.mark_news_processed(news_rowid, processed=True)
                    try:
                        record_news_processed()
                    except Exception:
                        pass

            except Exception:
                logger.exception(
                    "Failed to generate signals for asset %s for news id %s",
                    asset_symbol,
                    news_rowid,
                )
                # Ensure we still mark news processed to avoid repeatedly failing
                if news_rowid:
                    self.db.mark_news_processed(news_rowid, processed=True)
                continue

        return created_signal_ids

    def _upgrade_pattern_rr(self, pattern: PatternMatch, min_rr: float) -> PatternMatch:
        """
        Try to upgrade the pattern's TP so it meets min_rr while keeping stop fixed.
        This is a best-effort, we do not look for trade obstacles; rely on tuning later.
        """
        try:
            entry = pattern.entry_hint
            stop = pattern.stop_loss
            if not (entry and stop):
                return pattern
            # new TP either way:
            new_tp = (
                entry + min_rr * abs(entry - stop)
                if pattern.direction == "long"
                else entry - min_rr * abs(entry - stop)
            )
            pattern.take_profit = float(new_tp)
            denom = abs(entry - stop)
            pattern.rr = (
                abs((pattern.take_profit - entry) / denom) if denom > 0 else pattern.rr
            )
        except Exception:
            logger.exception("Failed to upgrade pattern rr for %s", pattern)
        return pattern

    def _is_pref_session_ok(self, symbol: str) -> bool:
        """
        For forex symbols, check if the current UTC hour is within one of the preferred sessions for the base currency.
        Uses `config.currency_session_map` and `config.session_map`. If base currency not known, return True.
        """
        try:
            cfg = self.config
            base = symbol.upper().replace("/", "").replace("-", "")[0:3]
            sessions = cfg.currency_session_map.get(base, [])
            if not sessions:
                # fallback to True to avoid blocking signals by default
                return True
            now_utc = datetime.utcnow().hour
            # check if any preferred session is active
            for s in sessions:
                times = cfg.session_map.get(s)
                if not times:
                    continue
                start_h, end_h = times
                if start_h < end_h:
                    if start_h <= now_utc < end_h:
                        return True
                else:
                    # wrap across midnight
                    if now_utc >= start_h or now_utc < end_h:
                        return True
            return False
        except Exception:
            logger.exception("Failed to evaluate session for symbol=%s", symbol)
            return True

    def close_expired_signals(self) -> List[int]:
        """
        Find open signals with expires_at < now and close them, recording 'timeout' reason.
        Additionally record a "timeout" trade row for analytics, using the signal's entry price as the executed_price.
        Return list of closed signal ids.
        """
        closed_ids: List[int] = []
        open_signals = self.db.get_open_signals(limit=500)
        now_iso = datetime.now(timezone.utc).isoformat()
        for row in open_signals:
            expires_at = row["expires_at"]
            if not expires_at:
                continue
            # parse iso string
            try:
                exp_dt = datetime.fromisoformat(expires_at)
                if exp_dt.tzinfo is None:
                    exp_dt = exp_dt.replace(tzinfo=timezone.utc)
            except Exception:
                # If parse fails, skip
                continue
            if exp_dt < datetime.now(timezone.utc):
                # Close the signal in the DB
                self.db.close_signal(
                    int(row["id"]),
                    close_price=None,
                    outcome="timeout",
                    reason="timeout",
                    exit_time=now_ts(),
                    pnl=0.0,
                )
                try:
                    record_signal_closed("timeout", row.get("symbol"), row.get("side"))
                except Exception:
                    pass
                # Record a 'timeout' trade record for analytics. If a real executed_at/price isn't available,
                # use signal created time and entry_price as fallbacks.
                try:
                    signal_created_at_iso = row.get("created_at")
                    signal_created_ts = (
                        int(datetime.fromisoformat(signal_created_at_iso).timestamp())
                        if signal_created_at_iso
                        else now_ts()
                    )
                    executed_price = row.get("entry_price") or None
                    exit_price = None
                    # executed_price and executed_at are required by schema; when a signal was never actually executed,
                    # we record an estimated entry using the signal entry_price (if present) and mark exit as now
                    trade_record = TradeRecord(
                        signal_id=int(row["id"]),
                        executed_at=signal_created_ts,
                        executed_price=float(executed_price)
                        if executed_price is not None
                        else 0.0,
                        exit_at=now_ts(),
                        exit_price=exit_price,
                        pnl=0.0,
                        outcome="timeout",
                        notes="Auto-closed due to timeout (24h)",
                    )
                    self.db.record_trade(trade_record)
                except Exception:
                    logger.exception(
                        "Failed to record timeout trade for signal %s", int(row["id"])
                    )
                closed_ids.append(int(row["id"]))
        if closed_ids:
            logger.info("Closed %s expired signals", len(closed_ids))
        return closed_ids

    def record_trade_result(
        self,
        signal_id: int,
        executed_at_ts: int,
        executed_price: float,
        exit_at_ts: int,
        exit_price: Optional[float],
        pnl: float,
        outcome: str,
        notes: Optional[str] = None,
    ) -> Optional[int]:
        """
        Record a trade outcome into the DB and close the related signal.
        This will:
          - Update the signal row as closed
          - Insert a trades row with the provided execution/exit info
          - Update the strategy tuner statistics using the trade result
          - Update the online scoring model (if enabled)
        Returns the created trade id or None on failure.
        """
        try:
            # mark the signal as closed (if it still exists / open)
            try:
                self.db.close_signal(
                    int(signal_id),
                    close_price=exit_price,
                    outcome=outcome,
                    reason=notes or "recorded_trade_result",
                    exit_time=exit_at_ts,
                    pnl=pnl,
                )
            except Exception:
                # If close_signal fails, we still attempt to record the trade, but log the issue
                logger.debug(
                    "Signal %s may have been already closed or missing", signal_id
                )

            # Ensure executed_at is an int epoch (seconds)
            executed_at_ts = int(executed_at_ts)
            exit_at_ts = int(exit_at_ts) if exit_at_ts is not None else now_ts()

            trade = TradeRecord(
                signal_id=int(signal_id),
                executed_at=executed_at_ts,
                executed_price=float(executed_price),
                exit_at=exit_at_ts,
                exit_price=exit_price,
                pnl=pnl,
                outcome=outcome,
                notes=notes,
            )
            trade_id = self.db.record_trade(trade)
            try:
                record_trade_recorded(outcome)
            except Exception:
                pass

            # Update tuner with a best-effort set of metadata for learning
            try:
                rows = self.db.execute_custom(
                    "SELECT symbol, rr, analysis_ids FROM signals WHERE id = ? LIMIT 1",
                    (int(signal_id),),
                )
                symbol = rows[0]["symbol"] if rows else None
                rr_val = None
                if rows and rows[0].get("rr") is not None:
                    try:
                        rr_val = float(rows[0].get("rr"))
                    except Exception:
                        rr_val = None
                hold_seconds = (
                    float(exit_at_ts - executed_at_ts)
                    if exit_at_ts and executed_at_ts
                    else None
                )
                # pattern name is not stored on the signal; we use 'unknown' for now
                self.tuner.update_from_trade(
                    "unknown",
                    symbol,
                    True if outcome == "win" else False,
                    rr_val,
                    hold_seconds,
                )
            except Exception:
                logger.exception(
                    "Failed to update tuner after recording trade %s", trade_id
                )

            # Update the online scorer with this new example so it can learn from trade outcomes.
            # We attempt to fetch the signal row for context and feed the scorer.
            try:
                if getattr(self, "scorer", None) is not None:
                    # Retrieve the latest stored signal row from DB in case fields in DB differ.
                    # This will provide analysis_ids, rr, leverage, entry price, etc.
                    signal_rows = self.db.execute_custom(
                        "SELECT * FROM signals WHERE id = ? LIMIT 1", (int(signal_id),)
                    )
                    if signal_rows:
                        signal_row = dict(signal_rows[0])
                        # Update the scorer using the trade result (online learning)
                        updated_ok = self.scorer.update_from_trade(
                            signal_row, outcome, executed_price, exit_price
                        )
                        # Optionally persist updated model (save to db and disk) - keep this safe and conservative.
                        if (
                            updated_ok
                            and getattr(self.scorer, "is_trained", lambda: False)()
                        ):
                            try:
                                # Save both locally and in DB for redundancy if enabled
                                self.scorer.save_model_to_disk()
                                if getattr(self.scorer, "use_db_store", False):
                                    self.scorer.save_model_to_db()
                            except Exception:
                                # Do not fail the process because of persistence issues.
                                logger.exception(
                                    "Failed to persist updated scorer model"
                                )
            except Exception:
                logger.exception(
                    "Failed to update scorer with trade outcome for signal %s",
                    signal_id,
                )

            return trade_id
        except Exception:
            logger.exception("Failed to record trade result for signal %s", signal_id)
            return None

    def monitor_and_tune(self, since_seconds: int = 3600 * 24):
        """
        Scan recent trades and update tuning stats in DB using StrategyTuner.
        This should be called periodically to keep a running aggregated view.

        This method performs two passes:
        1) It scans recent trades and updates tuning stats from actual executed trades.
        2) It scans recent closed signals that have no associated trades (i.e. never executed or timed-out)
           and updates the tuner with a "negative" example (treated as a loss) so the system can learn from
           signals that were created but never taken or that timed out without execution.
        """
        try:
            # ---------------------------------------------------------------------
            # 1) Process actual executed trades and update tuners based on outcomes
            # ---------------------------------------------------------------------
            trade_rows = self.db.execute_custom(
                "SELECT trades.*, signals.symbol, signals.analysis_ids, signals.side, signals.created_at FROM trades JOIN signals ON trades.signal_id = signals.id WHERE trades.created_at >= datetime('now', ?)",
                (f"-{int(since_seconds)} seconds",),
            )
            for row in trade_rows:
                try:
                    # compute hold duration in seconds
                    executed_at = row["executed_at"]
                    exit_at = row["exit_at"]
                    if executed_at and exit_at:
                        t1 = int(datetime.fromisoformat(executed_at).timestamp())
                        t2 = int(datetime.fromisoformat(exit_at).timestamp())
                        hold_seconds = max(0.0, float(t2 - t1))
                    else:
                        hold_seconds = None
                    outcome = (
                        row.get("outcome") if isinstance(row, dict) else row["outcome"]
                    )
                    win = True if outcome == "win" else False
                    # perform update
                    # Try to extract pattern name from analysis_ids or signal meta if available; fallback to "unknown"
                    pattern_name = "unknown"
                    # call tuner update using pnl for rr if available
                    rr_val = None
                    try:
                        rr_val = (
                            float(row["pnl"]) if row.get("pnl") is not None else None
                        )
                    except Exception:
                        rr_val = None
                    self.tuner.update_from_trade(
                        pattern_name, row["symbol"], win, rr_val, hold_seconds
                    )
                except Exception:
                    logger.exception(
                        "Failed to update tuning for trade id %s", row.get("id")
                    )

            # ---------------------------------------------------------------------
            # 2) Find closed signals that have no trades associated with them (likely never executed / timed-out)
            #    and update tuner by marking them as negative examples.
            # ---------------------------------------------------------------------
            closed_signal_rows = self.db.execute_custom(
                "SELECT s.* FROM signals s WHERE s.status = 'closed' AND s.closed_at >= datetime('now', ?) AND s.id NOT IN (SELECT DISTINCT signal_id FROM trades)",
                (f"-{int(since_seconds)} seconds",),
            )
            for s in closed_signal_rows:
                try:
                    symbol = s["symbol"] if "symbol" in s.keys() else None
                    outcome = s.get("outcome") if isinstance(s, dict) else s["outcome"]
                    # For signals closed without trades (timeouts/cancelled) we count them as not successful (loss) for tuning.
                    win = True if outcome == "win" else False
                    pattern_name = "unknown"
                    # rr and hold_seconds are unknown here; pass None.
                    self.tuner.update_from_trade(pattern_name, symbol, win, None, None)
                except Exception:
                    logger.exception(
                        "Failed to update tuning for closed signal id %s", s.get("id")
                    )
        except Exception:
            logger.exception("Failed to monitor trades and update tuner")
        # Before auto-applying tuning suggestions, ensure the signal scorer is trained or a persisted model is loaded.
        # This supports online-learning and safer decisions when applying automated adjustments.
        try:
            # Determine lookback for historical training (default 30 days)
            scorer_train_lookback_seconds = self.db.get_runtime_param_as_int(
                "scorer_train_lookback_seconds", 3600 * 24 * 30
            )
            if getattr(self, "scorer", None) is not None:
                try:
                    # If the scorer is not yet trained, attempt to train it using historical trades.
                    if not self.scorer.is_trained():
                        trained_count, positives = self.scorer.train_on_history(
                            lookback_seconds=scorer_train_lookback_seconds
                        )
                        logger.info(
                            "Scorer trained on historical trades: samples=%s positives=%s",
                            trained_count,
                            positives,
                        )
                    else:
                        logger.debug(
                            "Scorer already trained; skipping historical training"
                        )
                except Exception:
                    logger.exception(
                        "Failed to train or initialize scorer before applying tuning suggestions"
                    )
            # Auto-apply tuning suggestions if enabled
            suggestions = self.tuner.suggest_adjustments()
            auto_apply = self.db.get_runtime_param_as_bool(
                "runtime_auto_apply_tuner", True
            )
            if auto_apply and suggestions:
                applied = self.tuner.apply_adjustments(suggestions, auto_apply=True)
                logger.info("Auto-applied %s tuning adjustment(s)", len(applied))
        except Exception:
            logger.exception("Failed to auto-apply tuning suggestions")

    # Convenience method that performs end-to-end daily processing
    def run_once(self, max_news: int = 100):
        """
        Convenience single-run:
         - fetch and process unprocessed news
         - close expired signals
         - optionally monitor trades and tune
        """
        created = self.process_unprocessed_news(limit=max_news)
        closed = self.close_expired_signals()
        # Optionally run tuner
        try:
            self.monitor_and_tune()
        except Exception:
            logger.debug("Failed to run monitor/tune cycle")
        return {"created": created, "closed": closed}
