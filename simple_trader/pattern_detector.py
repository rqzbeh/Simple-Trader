# Simple-Trader\simple_trader\pattern_detector.py
# -*- coding: utf-8 -*-
"""
Candlestick pattern detector for 2H timeframe with simple heuristics.

This module provides:
- a `PatternMatch` dataclass describing simple pattern signals,
- a `PatternDetector` class that detects popular candlestick patterns on
  OHLC DataFrames (pandas) and provides utilities to create initial entry,
  stop-loss and take-profit suggestions with a minimum risk-reward constraint.

Design goals:
- Lightweight, dependency on pandas only (assumes DataFrame with 'open', 'high', 'low', 'close', 'volume').
- Heuristics focus on clarity and conservative defaults (suitable for signal generation, not order execution).
- Provide `confidence` metrics to enable higher-level filters (like "only use patterns with conf >= 0.6").
"""

from __future__ import annotations

import logging
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from typing import Dict, List, Optional, Tuple

import numpy as np
import pandas as pd

try:
    import talib  # type: ignore

    HAS_TALIB = True
except Exception:
    HAS_TALIB = False

from simple_trader.config import CONFIG, Config

logger = logging.getLogger("simple_trader.pattern_detector")
logger.addHandler(logging.NullHandler())


# ---- data types ----


@dataclass
class PatternMatch:
    """
    A normalized description of a detected candlestick pattern.
    - pattern_name: e.g. 'bullish_engulfing', 'hammer'
    - direction: 'long' | 'short' | 'neutral'
    - confidence: 0..1
    - start_idx / end_idx: integer indices of DataFrame rows used by the pattern
    - entry_hint: approximate price to consider as entry (float) - often the breakout price based on the pattern high/low
    - stop_loss: approximate stop loss (float)
    - take_profit: approximate take profit (float)
    - rr: risk-reward ratio (float)
    - meta: additional provider-specific information, e.g., wick sizes, avg range, etc.
    """

    pattern_name: str
    direction: str
    confidence: float
    start_idx: int
    end_idx: int
    entry_hint: Optional[float] = None
    stop_loss: Optional[float] = None
    take_profit: Optional[float] = None
    rr: Optional[float] = None
    recommended_leverage: Optional[int] = None
    meta: Dict = None


# ---- helper / metrics ----


def _calc_metrics_from_row(row: pd.Series) -> Dict[str, float]:
    """
    Compute body, wicks, and ratios for a single OHLC row.
    """
    o = float(row["open"])
    h = float(row["high"])
    l = float(row["low"])
    c = float(row["close"])
    total_range = h - l if h != l else 1e-9
    body = abs(c - o)
    upper_wick = h - max(o, c)
    lower_wick = min(o, c) - l
    # body ratio relative to total range
    body_ratio = body / total_range if total_range > 0 else 0.0
    # upper/lower wicks relative to total range
    upper_wick_pct = upper_wick / total_range
    lower_wick_pct = lower_wick / total_range
    direction = "bullish" if c > o else "bearish" if c < o else "neutral"
    return {
        "open": o,
        "high": h,
        "low": l,
        "close": c,
        "body": body,
        "body_ratio": body_ratio,
        "upper_wick": upper_wick,
        "lower_wick": lower_wick,
        "upper_wick_pct": upper_wick_pct,
        "lower_wick_pct": lower_wick_pct,
        "total_range": total_range,
        "direction": direction,
    }


def compute_atr(df: pd.DataFrame, period: int = 14) -> float:
    """
    Compute ATR over the provided dataframe (expects 'high','low','close').
    Returns the latest ATR float.
    """
    if df is None or len(df) < 2:
        return 0.0
    high = df["high"].astype(float)
    low = df["low"].astype(float)
    close = df["close"].astype(float)
    prev_close = close.shift(1)
    tr1 = (high - low).abs()
    tr2 = (high - prev_close).abs()
    tr3 = (low - prev_close).abs()
    tr = pd.concat([tr1, tr2, tr3], axis=1).max(axis=1)
    atr = tr.ewm(span=period, min_periods=1).mean()
    # return latest or 0
    return float(atr.iloc[-1]) if not atr.empty else 0.0


def compute_rsi(df: pd.DataFrame, period: int = 14) -> pd.Series:
    """
    Compute RSI over the provided dataframe (expects 'close').
    Returns a pandas.Series containing RSI values aligned with `df.index`.

    Uses TA-Lib if available for better numerical stability, otherwise falls back to
    an EMA-based Wilder's smoothing computation which is analogous to the TA-Lib approach.
    """
    if df is None or df.empty:
        return pd.Series(dtype=float)

    close = df["close"].astype(float)
    try:
        # If TA-Lib available, prefer it
        import talib as _talib  # type: ignore

        rsi_vals = _talib.RSI(close.values, timeperiod=period)
        # Wrap into Series for consistent downstream handling
        return pd.Series(rsi_vals, index=close.index)
    except Exception:
        # Fallback: pandas-based Wilder's smoothing (exponential style)
        delta = close.diff(1)
        gain = delta.clip(lower=0.0)
        loss = -delta.clip(upper=0.0)

        # Wilder's EMA with alpha = 1/period
        avg_gain = gain.ewm(alpha=1.0 / period, min_periods=period, adjust=False).mean()
        avg_loss = loss.ewm(alpha=1.0 / period, min_periods=period, adjust=False).mean()
        rs = avg_gain / avg_loss
        rsi = 100.0 - (100.0 / (1.0 + rs))
        return rsi


# ---- pattern heuristics ----


# ---- pattern heuristics ----


class PatternDetector:
    """
    Use a set of heuristics to detect candlestick patterns. Patterns are detected in the slice
    of the DataFrame that is passed; we assume the DataFrame index is chronological (ascending).
    """

    def __init__(self, config: Optional[Config] = None):
        self.cfg = config or CONFIG
        self.min_pattern_confidence = getattr(self.cfg, "min_pattern_confidence", 0.6)
        # Minimum body ratio to be considered a 'strong' candle vs noise (relative to range)
        self.default_min_body_ratio = 0.25

    def detect_patterns(
        self, df: pd.DataFrame, lookback: int = 24
    ) -> List[PatternMatch]:
        """
        Scan a DataFrame for patterns. Will analyze up to `lookback` most recent candles
        and return a list of PatternMatch objects, ordered by increasing end_idx (earliest first).
        """
        if df is None or df.empty:
            return []

        # ensure we have necessary columns
        for col in ["open", "high", "low", "close"]:
            if col not in df.columns:
                raise ValueError(f"DataFrame must contain '{col}' column")

        # using last `lookback` rows
        df_slice = df.tail(lookback).copy()
        df_slice = df_slice.reset_index(drop=False)
        # this df_slice has a sequential numeric index (0..n-1) while its original index may be datetimes.
        # We'll report start_idx / end_idx relative to the slice start (still consistent with the slice).
        n = len(df_slice)
        matches: List[PatternMatch] = []

        # Precompute basic metrics
        metrics = [None] * n
        for i in range(n):
            metrics[i] = _calc_metrics_from_row(df_slice.iloc[i])

        # compute average body size for confidence scaling
        avg_body = float(np.mean([m["body"] for m in metrics])) if metrics else 0.0
        atr = compute_atr(df, period=14)
        # guard: if atr is 0, fallback to average body size or a minimal number
        avg_vol = atr if atr > 0.0 else (avg_body or 0.0001)

        # Try detecting patterns with TA-Lib when available, as it tends to be more robust.
        matches: List[PatternMatch] = []
        if HAS_TALIB:
            try:
                talib_matches = self._detect_patterns_with_talib(df_slice, metrics, atr)
                if talib_matches:
                    matches.extend(talib_matches)
            except Exception:
                logger.exception(
                    "TA-Lib detection failed; falling back to internal heuristics"
                )

        # iterate and detect patterns that require 1..3 or more candles
        for i in range(n):
            # single candle patterns: doji, hammer/shooting star
            row_metrics = metrics[i]
            # doji
            if self._is_doji(row_metrics):
                conf = self._confidence_from_strength(
                    row_metrics["body_ratio"], avg_body, avg_vol, "doji"
                )
                match = self._build_one_candle_match(
                    "doji", "neutral", conf, i, i, df_slice, metrics, atr
                )
                if match:
                    matches.append(match)

            # hammer / inverted hammer / shooting star / hanging man
            if self._is_hammer(row_metrics):
                direction = "long"
                conf = self._confidence_from_strength(
                    row_metrics["lower_wick_pct"], avg_body, avg_vol, "hammer"
                )
                match = self._build_one_candle_match(
                    "hammer", direction, conf, i, i, df_slice, metrics, atr
                )
                if match:
                    matches.append(match)
            if self._is_shooting_star(row_metrics):
                direction = "short"
                conf = self._confidence_from_strength(
                    row_metrics["upper_wick_pct"], avg_body, avg_vol, "shooting_star"
                )
                match = self._build_one_candle_match(
                    "shooting_star", direction, conf, i, i, df_slice, metrics, atr
                )
                if match:
                    matches.append(match)

            # 2-candle patterns: engulfing, harami
            if i >= 1:
                m_prev = metrics[i - 1]
                m_curr = row_metrics
                if self._is_bullish_engulfing(m_prev, m_curr):
                    conf = self._confidence_engulfing(m_prev, m_curr, avg_vol)
                    match = self._build_two_candle_match(
                        "bullish_engulfing",
                        "long",
                        conf,
                        i - 1,
                        i,
                        df_slice,
                        metrics,
                        atr,
                    )
                    if match:
                        matches.append(match)
                if self._is_bearish_engulfing(m_prev, m_curr):
                    conf = self._confidence_engulfing(m_prev, m_curr, avg_vol)
                    match = self._build_two_candle_match(
                        "bearish_engulfing",
                        "short",
                        conf,
                        i - 1,
                        i,
                        df_slice,
                        metrics,
                        atr,
                    )
                    if match:
                        matches.append(match)
                if self._is_harami(m_prev, m_curr):
                    # direction is opposite of previous candle for classic maoming
                    dirn = "short" if m_prev["direction"] == "bullish" else "long"
                    conf = max(
                        0.2,
                        self._confidence_from_strength(
                            1.0 - m_curr["body_ratio"], avg_body, avg_vol, "harami"
                        ),
                    )
                    match = self._build_two_candle_match(
                        "harami", dirn, conf, i - 1, i, df_slice, metrics, atr
                    )
                    if match:
                        matches.append(match)

            # 3-candle patterns: morning star / evening star / three soldiers / three crows
            if i >= 2:
                m0, m1, m2 = metrics[i - 2], metrics[i - 1], metrics[i]
                # morning star / evening star
                if self._is_morning_star(m0, m1, m2):
                    conf = self._confidence_morning_evening(m0, m1, m2)
                    match = self._build_three_candle_match(
                        "morning_star", "long", conf, i - 2, i, df_slice, metrics, atr
                    )
                    if match:
                        matches.append(match)
                if self._is_evening_star(m0, m1, m2):
                    conf = self._confidence_morning_evening(m0, m1, m2)
                    match = self._build_three_candle_match(
                        "evening_star", "short", conf, i - 2, i, df_slice, metrics, atr
                    )
                    if match:
                        matches.append(match)
                # three white soldiers / three black crows
                if self._is_three_white_soldiers(metrics, i - 2, i):
                    conf = 0.85
                    match = self._build_three_candle_match(
                        "three_white_soldiers",
                        "long",
                        conf,
                        i - 2,
                        i,
                        df_slice,
                        metrics,
                        atr,
                    )
                    if match:
                        matches.append(match)
                if self._is_three_black_crows(metrics, i - 2, i):
                    conf = 0.85
                    match = self._build_three_candle_match(
                        "three_black_crows",
                        "short",
                        conf,
                        i - 2,
                        i,
                        df_slice,
                        metrics,
                        atr,
                    )
                    if match:
                        matches.append(match)

        # filter and normalize matches: ensure confidence > min pattern confidence; ensure TP/SL meets RR requirement
        filtered: List[PatternMatch] = []
        # Try detecting additional patterns (inside/outside bars, MA confluence, RSI divergence)
        try:
            additional_matches = self._detect_additional_patterns(
                df_slice, metrics, atr
            )
            if additional_matches:
                matches.extend(additional_matches)
        except Exception:
            logger.exception("Failed to detect additional patterns")

        for m in matches:
            if m.confidence is None:
                c = 0.0
            else:
                c = float(m.confidence)
            if c < self.min_pattern_confidence:
                continue
            # Ensure TP/SL are set; if not, attempt to compute them
            match_with_price = self._ensure_trade_prices(m, df_slice)
            if not match_with_price:
                continue
            # enforce min RR (config)
            rr = match_with_price.rr or 0.0
            if rr < getattr(self.cfg, "min_risk_reward_ratio", 3.0):
                # try upgrading TP to reach min RR by preserving TP/SL direction
                match_with_price = self._upgrade_take_profit_to_min_rr(
                    match_with_price, df_slice
                )
                if match_with_price.rr < getattr(
                    self.cfg, "min_risk_reward_ratio", 3.0
                ):
                    # can't satisfy rr -> skip
                    continue
            filtered.append(match_with_price)
        return filtered

    # ---- helpers for building matches ----

    def _build_one_candle_match(
        self,
        name: str,
        direction: str,
        confidence: float,
        start_idx: int,
        end_idx: int,
        df_slice: pd.DataFrame,
        metrics: List[Dict],
        atr: float,
    ) -> Optional[PatternMatch]:
        m = PatternMatch(
            pattern_name=name,
            direction=direction,
            confidence=max(0.0, min(1.0, confidence)),
            start_idx=start_idx,
            end_idx=end_idx,
            meta={},
        )
        # propose entry and SL based on this candle
        idx = end_idx
        if idx >= 0 and idx < len(df_slice):
            row = df_slice.iloc[idx]
            mm = metrics[idx]
            price = float(row["close"])
            if direction == "long":
                entry_hint = float(mm["high"] + (atr * 0.05))
                stop_loss = float(mm["low"] - (atr * 0.1))
            elif direction == "short":
                entry_hint = float(mm["low"] - (atr * 0.05))
                stop_loss = float(mm["high"] + (atr * 0.1))
            else:
                entry_hint = float(price)
                stop_loss = float(price)
            # initial TP halfway or 3x risk by default
            risk = abs(entry_hint - stop_loss)
            tp = (
                entry_hint + (risk * max(3.0, 3.0))
                if direction == "long"
                else entry_hint - (risk * max(3.0, 3.0))
            )
            m.entry_hint = entry_hint
            m.stop_loss = stop_loss
            m.take_profit = tp
            m.rr = (
                abs((tp - entry_hint) / (entry_hint - stop_loss))
                if (entry_hint - stop_loss) != 0
                else None
            )
            m.recommended_leverage = self._suggest_leverage(m.confidence)
            m.meta = {"body_ratio": mm["body_ratio"], "atr": atr}
            return m
        return None

    def _build_two_candle_match(
        self,
        name: str,
        direction: str,
        confidence: float,
        start_idx: int,
        end_idx: int,
        df_slice: pd.DataFrame,
        metrics: List[Dict],
        atr: float,
    ) -> Optional[PatternMatch]:
        m = PatternMatch(
            pattern_name=name,
            direction=direction,
            confidence=max(0.0, min(1.0, confidence)),
            start_idx=start_idx,
            end_idx=end_idx,
            meta={},
        )
        # Use extremes of the involved candles
        srow = df_slice.iloc[start_idx]
        erow = df_slice.iloc[end_idx]
        high = float(max(srow["high"], erow["high"]))
        low = float(min(srow["low"], erow["low"]))
        if direction == "long":
            entry_hint = float(high + (atr * 0.05))
            stop_loss = float(low - (atr * 0.1))
            risk = abs(entry_hint - stop_loss)
            tp = entry_hint + (risk * self.cfg.min_risk_reward_ratio)
        else:
            entry_hint = float(low - (atr * 0.05))
            stop_loss = float(high + (atr * 0.1))
            risk = abs(entry_hint - stop_loss)
            tp = entry_hint - (risk * self.cfg.min_risk_reward_ratio)
        m.entry_hint = entry_hint
        m.stop_loss = stop_loss
        m.take_profit = tp
        m.rr = (
            abs((tp - entry_hint) / (entry_hint - stop_loss))
            if (entry_hint - stop_loss) != 0
            else None
        )
        m.recommended_leverage = self._suggest_leverage(m.confidence)
        m.meta = {
            "candle0": _calc_metrics_from_row(srow),
            "candle1": _calc_metrics_from_row(erow),
            "atr": atr,
        }
        return m

    def _build_three_candle_match(
        self,
        name: str,
        direction: str,
        confidence: float,
        start_idx: int,
        end_idx: int,
        df_slice: pd.DataFrame,
        metrics: List[Dict],
        atr: float,
    ) -> Optional[PatternMatch]:
        m = PatternMatch(
            pattern_name=name,
            direction=direction,
            confidence=max(0.0, min(1.0, confidence)),
            start_idx=start_idx,
            end_idx=end_idx,
            meta={},
        )
        # extremes across the three candles
        rows = df_slice.iloc[start_idx : end_idx + 1]
        high = float(rows["high"].max())
        low = float(rows["low"].min())
        if direction == "long":
            entry_hint = float(rows.iloc[-1]["close"] + (atr * 0.05))
            stop_loss = float(low - (atr * 0.1))
            risk = abs(entry_hint - stop_loss)
            tp = entry_hint + (risk * self.cfg.min_risk_reward_ratio)
        else:
            entry_hint = float(rows.iloc[-1]["close"] - (atr * 0.05))
            stop_loss = float(high + (atr * 0.1))
            risk = abs(entry_hint - stop_loss)
            tp = entry_hint - (risk * self.cfg.min_risk_reward_ratio)
        m.entry_hint = entry_hint
        m.stop_loss = stop_loss
        m.take_profit = tp
        m.rr = (
            abs((tp - entry_hint) / (entry_hint - stop_loss))
            if (entry_hint - stop_loss) != 0
            else None
        )
        m.recommended_leverage = self._suggest_leverage(m.confidence)
        m.meta = {"high": high, "low": low, "atr": atr}
        return m

    # ---- core pattern detectors ----

    def _is_doji(self, m: Dict) -> bool:
        # doji if body is very small relative to candle range
        return float(m["body_ratio"]) <= 0.1

    def _is_hammer(self, m: Dict) -> bool:
        # hammer: long lower wick, small body near top, upper wick small
        return (
            (m["lower_wick_pct"] >= 0.6)
            and (m["upper_wick_pct"] <= 0.2)
            and (m["body_ratio"] <= 0.3)
            and (m["direction"] == "bullish")
        )

    def _is_shooting_star(self, m: Dict) -> bool:
        # shooting star: long upper wick, small body near bottom
        return (
            (m["upper_wick_pct"] >= 0.6)
            and (m["lower_wick_pct"] <= 0.2)
            and (m["body_ratio"] <= 0.3)
            and (m["direction"] == "bearish")
        )

    def _is_bullish_engulfing(self, m_prev: Dict, m_curr: Dict) -> bool:
        # Prev must be bearish, curr bullish and body of curr engulf prev body (in price)
        if not (m_prev["direction"] == "bearish" and m_curr["direction"] == "bullish"):
            return False
        # body is bigger and engulf body zone
        prev_open = m_prev["open"]
        prev_close = m_prev["close"]
        curr_open = m_curr["open"]
        curr_close = m_curr["close"]
        # Check body size and engulf condition
        return (
            (abs(curr_close - curr_open) > abs(prev_close - prev_open))
            and (curr_open < prev_close)
            and (curr_close > prev_open)
        )

    def _is_bearish_engulfing(self, m_prev: Dict, m_curr: Dict) -> bool:
        if not (m_prev["direction"] == "bullish" and m_curr["direction"] == "bearish"):
            return False
        prev_open = m_prev["open"]
        prev_close = m_prev["close"]
        curr_open = m_curr["open"]
        curr_close = m_curr["close"]
        return (
            (abs(curr_close - curr_open) > abs(prev_close - prev_open))
            and (curr_open > prev_close)
            and (curr_close < prev_open)
        )

    def _is_harami(self, m_prev: Dict, m_curr: Dict) -> bool:
        # Harami: previous big candle with opposite direction; current small body contained within previous body
        if m_prev["direction"] == m_curr["direction"]:
            return False
        prev_open = m_prev["open"]
        prev_close = m_prev["close"]
        curr_open = m_curr["open"]
        curr_close = m_curr["close"]
        prev_low = min(prev_open, prev_close)
        prev_high = max(prev_open, prev_close)
        curr_low = min(curr_open, curr_close)
        curr_high = max(curr_open, curr_close)
        return (
            (curr_low >= prev_low)
            and (curr_high <= prev_high)
            and (m_prev["body_ratio"] > 0.35)
            and (m_curr["body_ratio"] < 0.25)
        )

    def _is_morning_star(self, m0: Dict, m1: Dict, m2: Dict) -> bool:
        # m0 bearish strong, m1 small (doji or small), m2 bullish strong closing well into m0's body
        if not (m0["direction"] == "bearish" and m2["direction"] == "bullish"):
            return False
        if not (m0["body_ratio"] >= 0.35):
            return False
        if not (m1["body_ratio"] <= 0.25):
            return False
        # last close above the midpoint of m0
        m0_mid = (m0["open"] + m0["close"]) / 2.0
        return m2["close"] >= m0_mid

    def _is_evening_star(self, m0: Dict, m1: Dict, m2: Dict) -> bool:
        # inverse of morning star
        if not (m0["direction"] == "bullish" and m2["direction"] == "bearish"):
            return False
        if not (m0["body_ratio"] >= 0.35):
            return False
        if not (m1["body_ratio"] <= 0.25):
            return False
        m0_mid = (m0["open"] + m0["close"]) / 2.0
        return m2["close"] <= m0_mid

    def _three_consecutive_direction(
        self, metrics: List[Dict], start_idx: int, end_idx: int, direction: str
    ) -> bool:
        for i in range(start_idx, end_idx + 1):
            if metrics[i]["direction"] != direction:
                return False
        return True

    def _is_three_white_soldiers(
        self, metrics: List[Dict], start_idx: int, end_idx: int
    ) -> bool:
        # three bullish bodies with consecutive higher opens/closes and relatively long bodies compared to average
        if not self._three_consecutive_direction(
            metrics, start_idx, end_idx, "bullish"
        ):
            return False
        # checks
        opens = [metrics[i]["open"] for i in range(start_idx, end_idx + 1)]
        closes = [metrics[i]["close"] for i in range(start_idx, end_idx + 1)]
        return (
            (closes[0] < closes[1] < closes[2])
            and (opens[0] < opens[1] < opens[2])
            and all(
                metrics[i]["body_ratio"] > 0.2 for i in range(start_idx, end_idx + 1)
            )
        )

    def _is_three_black_crows(
        self, metrics: List[Dict], start_idx: int, end_idx: int
    ) -> bool:
        # three bearish bodies with consecutive lower opens/closes and relatively long bodies
        if not self._three_consecutive_direction(
            metrics, start_idx, end_idx, "bearish"
        ):
            return False
        opens = [metrics[i]["open"] for i in range(start_idx, end_idx + 1)]
        closes = [metrics[i]["close"] for i in range(start_idx, end_idx + 1)]
        return (
            (closes[0] > closes[1] > closes[2])
            and (opens[0] > opens[1] > opens[2])
            and all(
                metrics[i]["body_ratio"] > 0.2 for i in range(start_idx, end_idx + 1)
            )
        )

    def _detect_additional_patterns(
        self, df_slice: pd.DataFrame, metrics: List[Dict], atr: float
    ) -> List[PatternMatch]:
        """
        Extra patterns: inside/outside bar, moving-average confluence, and RSI divergence.
        This helper runs after the core heuristics and TA-Lib results, and returns additional
        PatternMatch objects (confidence & direction assigned heuristically).
        """
        matches: List[PatternMatch] = []
        try:
            n = len(df_slice)
            if n < 2:
                return matches

            # --------------- INSIDE/OUTSIDE BAR DETECTION --------------
            for i in range(1, n):
                prev = df_slice.iloc[i - 1]
                curr = df_slice.iloc[i]

                prev_high = float(prev["high"])
                prev_low = float(prev["low"])
                prev_open = float(prev["open"])
                prev_close = float(prev["close"])
                prev_range = prev_high - prev_low

                curr_high = float(curr["high"])
                curr_low = float(curr["low"])
                curr_open = float(curr["open"])
                curr_close = float(curr["close"])
                curr_range = (
                    curr_high - curr_low
                    if curr_high is not None and curr_low is not None
                    else 0.0
                )

                # Inside bar: current candle completely within previous high/low and materially smaller range
                if (
                    curr_high <= prev_high
                    and curr_low >= prev_low
                    and prev_range > 0
                    and curr_range < (prev_range * 0.85)
                ):
                    direction = "long" if prev_close > prev_open else "short"
                    # Confidence partially based on how smaller curr range is vs prev range
                    conf = min(
                        0.99,
                        0.45 + max(0.0, (prev_range - curr_range) / prev_range) * 0.5,
                    )
                    m = self._build_two_candle_match(
                        "inside_bar", direction, conf, i - 1, i, df_slice, metrics, atr
                    )
                    if m:
                        matches.append(m)

                # Outside bar: current spans prior high/low (usually indicates a volatility-breakout)
                if (
                    curr_high > prev_high
                    and curr_low < prev_low
                    and prev_range > 0
                    and curr_range > (prev_range * 1.05)
                ):
                    direction = "long" if curr_close > curr_open else "short"
                    conf = min(
                        0.99, 0.45 + (curr_range / max(prev_range, 1e-9) - 1.0) * 0.35
                    )
                    m = self._build_two_candle_match(
                        "outside_bar", direction, conf, i - 1, i, df_slice, metrics, atr
                    )
                    if m:
                        matches.append(m)

            # --------------- MOVING AVERAGE CONFLUENCE (last candle) --------------
            try:
                closes = df_slice["close"].astype(float)
                if len(closes) >= 50:
                    sma20 = closes.rolling(20).mean()
                    sma50 = closes.rolling(50).mean()
                    last_idx = len(closes) - 1
                    if not pd.isna(sma20.iloc[last_idx]) and not pd.isna(
                        sma50.iloc[last_idx]
                    ):
                        last_close = closes.iloc[last_idx]
                        sma20_last = float(sma20.iloc[last_idx])
                        sma50_last = float(sma50.iloc[last_idx])
                        # bullish confluence: price above both SMA and SMA20 above SMA50
                        if last_close > sma20_last and sma20_last > sma50_last:
                            conf = min(
                                0.98,
                                0.55
                                + (
                                    abs((last_close - sma20_last) / max(atr, 1e-9))
                                    * 0.03
                                ),
                            )
                            m = self._build_one_candle_match(
                                "ma_confluence",
                                "long",
                                conf,
                                last_idx,
                                last_idx,
                                df_slice,
                                metrics,
                                atr,
                            )
                            if m:
                                matches.append(m)
                        # bearish confluence: price below both SMA and SMA20 below SMA50
                        if last_close < sma20_last and sma20_last < sma50_last:
                            conf = min(
                                0.98,
                                0.55
                                + (
                                    abs((sma20_last - last_close) / max(atr, 1e-9))
                                    * 0.03
                                ),
                            )
                            m = self._build_one_candle_match(
                                "ma_confluence",
                                "short",
                                conf,
                                last_idx,
                                last_idx,
                                df_slice,
                                metrics,
                                atr,
                            )
                            if m:
                                matches.append(m)
            except Exception:
                logger.exception(
                    "MA-confluence detection failed; continuing without MA signals"
                )

            # --------------- RSI DIVERGENCE DETECTION --------------
            try:
                rsi_series = compute_rsi(df_slice, period=14)
                closes = df_slice["close"].astype(float)
                # gather last two peaks and troughs conservatively by scanning from the end
                peaks = []
                troughs = []
                # simple local-extrema detection
                for j in range(len(closes) - 2, 0, -1):
                    c = float(closes.iloc[j])
                    if c > float(closes.iloc[j - 1]) and c > float(closes.iloc[j + 1]):
                        peaks.append(j)
                    if c < float(closes.iloc[j - 1]) and c < float(closes.iloc[j + 1]):
                        troughs.append(j)
                    if len(peaks) >= 2 and len(troughs) >= 2:
                        break

                # Bearish divergence: price makes higher high, RSI lower high
                if len(peaks) >= 2:
                    p0 = peaks[0]
                    p1 = peaks[1]
                    p0_close = float(closes.iloc[p0])
                    p1_close = float(closes.iloc[p1])
                    p0_rsi = float(rsi_series.iloc[p0])
                    p1_rsi = float(rsi_series.iloc[p1])
                    if p0_close > p1_close and p0_rsi < p1_rsi:
                        conf = min(
                            0.99,
                            0.55
                            + (abs(p0_close - p1_close) / max(p1_close, 1e-9)) * 0.5
                            + abs(p1_rsi - p0_rsi) / 100.0,
                        )
                        m = self._build_two_candle_match(
                            "rsi_divergence",
                            "short",
                            conf,
                            min(p1, p0),
                            max(p1, p0),
                            df_slice,
                            metrics,
                            atr,
                        )
                        if m:
                            matches.append(m)

                # Bullish divergence: price makes lower low, RSI higher low
                if len(troughs) >= 2:
                    t0 = troughs[0]
                    t1 = troughs[1]
                    t0_close = float(closes.iloc[t0])
                    t1_close = float(closes.iloc[t1])
                    t0_rsi = float(rsi_series.iloc[t0])
                    t1_rsi = float(rsi_series.iloc[t1])
                    if t0_close < t1_close and t0_rsi > t1_rsi:
                        conf = min(
                            0.99,
                            0.55
                            + (abs(t1_close - t0_close) / max(t1_close, 1e-9)) * 0.5
                            + abs(t0_rsi - t1_rsi) / 100.0,
                        )
                        m = self._build_two_candle_match(
                            "rsi_divergence",
                            "long",
                            conf,
                            min(t1, t0),
                            max(t1, t0),
                            df_slice,
                            metrics,
                            atr,
                        )
                        if m:
                            matches.append(m)
            except Exception:
                logger.exception(
                    "RSI divergence heuristics failed; skipping RSI-based patterns"
                )

        except Exception:
            logger.exception("Additional patterns detection failed")

        return matches

    def _detect_patterns_with_talib(
        self, df_slice: pd.DataFrame, metrics: List[Dict], atr: float
    ) -> List[PatternMatch]:
        """
        Use TA-Lib to detect patterns. Map TA-Lib function outputs to PatternMatch objects.
        TA-Lib returns strong signals that we treat as high-confidence matches (0.85-0.98).
        This function should return PatternMatch objects using the same fields the heuristics produce.
        """
        matches: List[PatternMatch] = []
        if not HAS_TALIB:
            return matches

        # Prepare arrays
        try:
            open_arr = df_slice["open"].astype(float).values
            high_arr = df_slice["high"].astype(float).values
            low_arr = df_slice["low"].astype(float).values
            close_arr = df_slice["close"].astype(float).values
        except Exception:
            # If df doesn't have expected columns, return empty
            logger.exception("TA-Lib detection failed to prepare arrays")
            return matches

        def _register(idx, start_idx, end_idx, patt, direction, conf):
            # compute suggested prices similarly to heuristics but based on extremes
            try:
                srow = df_slice.iloc[start_idx]
                erow = df_slice.iloc[end_idx]
            except Exception:
                return
            high = float(max(srow["high"], erow["high"]))
            low = float(min(srow["low"], erow["low"]))
            if direction == "long":
                entry_hint = float(high + atr * 0.05)
                stop_loss = float(low - atr * 0.1)
                risk = abs(entry_hint - stop_loss)
                take_profit = float(
                    entry_hint + (risk * max(self.cfg.min_risk_reward_ratio, 3.0))
                )
            elif direction == "short":
                entry_hint = float(low - atr * 0.05)
                stop_loss = float(high + atr * 0.1)
                risk = abs(entry_hint - stop_loss)
                take_profit = float(
                    entry_hint - (risk * max(self.cfg.min_risk_reward_ratio, 3.0))
                )
            else:
                # neutral fallback
                entry_hint = float(erow["close"])
                stop_loss = float(low)
                take_profit = float(high)
            rr = (
                abs((take_profit - entry_hint) / (entry_hint - stop_loss))
                if entry_hint != stop_loss
                else None
            )
            pm = PatternMatch(
                pattern_name=patt,
                direction=direction,
                confidence=conf,
                start_idx=start_idx,
                end_idx=end_idx,
                entry_hint=entry_hint,
                stop_loss=stop_loss,
                take_profit=take_profit,
                rr=rr,
                recommended_leverage=self._suggest_leverage(conf),
                meta={"talib_signal_at": idx},
            )
            matches.append(pm)

        try:
            # Common TA-Lib pattern functions we map to our names
            try:
                eng = talib.CDLENGULFING(open_arr, high_arr, low_arr, close_arr)
            except Exception:
                eng = None
            try:
                hammer = talib.CDLHAMMER(open_arr, high_arr, low_arr, close_arr)
            except Exception:
                hammer = None
            try:
                doji = talib.CDLDOJI(open_arr, high_arr, low_arr, close_arr)
            except Exception:
                doji = None
            try:
                shooting = talib.CDLSHOOTINGSTAR(open_arr, high_arr, low_arr, close_arr)
            except Exception:
                shooting = None
            try:
                three_white = talib.CDL3WHITESOLDIERS(
                    open_arr, high_arr, low_arr, close_arr
                )
            except Exception:
                three_white = None
            try:
                three_black = talib.CDL3BLACKCROWS(
                    open_arr, high_arr, low_arr, close_arr
                )
            except Exception:
                three_black = None
            try:
                morning = talib.CDLMORNINGSTAR(open_arr, high_arr, low_arr, close_arr)
            except Exception:
                morning = None
            try:
                evening = talib.CDLEVENINGSTAR(open_arr, high_arr, low_arr, close_arr)
            except Exception:
                evening = None

            # For each talib output array, find non-zero indices and register matches
            L = len(close_arr)
            for i in range(L):
                idx0, idx1 = i, i
                # Engulfing (2-candle)
                if eng is not None and eng[i] != 0:
                    sign = eng[i]
                    # bullish if positive, bearish if negative; in talib positive -> bullish
                    direction = "long" if sign > 0 else "short"
                    start_idx = max(0, i - 1)
                    end_idx = i
                    patt = "bullish_engulfing" if sign > 0 else "bearish_engulfing"
                    _register(i, start_idx, end_idx, patt, direction, 0.92)

                # Hammer
                if hammer is not None and hammer[i] != 0:
                    sign = hammer[i]
                    # Hammer typically bullish (positive); negative sometimes indicates inverted hammer (bearish-ish)
                    direction = "long" if sign > 0 else "short"
                    patt = "hammer" if sign > 0 else "inverted_hammer"
                    start_idx = i
                    end_idx = i
                    _register(i, start_idx, end_idx, patt, direction, 0.9)

                # Doji
                if doji is not None and doji[i] != 0:
                    start_idx = i
                    end_idx = i
                    _register(i, start_idx, end_idx, "doji", "neutral", 0.85)

                # Shooting star (bearish)
                if shooting is not None and shooting[i] != 0:
                    start_idx = i
                    end_idx = i
                    _register(i, start_idx, end_idx, "shooting_star", "short", 0.9)

                # Three white soldiers / three black crows
                if three_white is not None and three_white[i] != 0:
                    # 3 candle patterns: mark start as i-2 if available
                    start_idx = max(0, i - 2)
                    end_idx = i
                    _register(
                        i, start_idx, end_idx, "three_white_soldiers", "long", 0.9
                    )

                if three_black is not None and three_black[i] != 0:
                    start_idx = max(0, i - 2)
                    end_idx = i
                    _register(i, start_idx, end_idx, "three_black_crows", "short", 0.9)

                # Morning star / Evening star
                if morning is not None and morning[i] != 0:
                    start_idx = max(0, i - 2)
                    end_idx = i
                    _register(i, start_idx, end_idx, "morning_star", "long", 0.92)

                if evening is not None and evening[i] != 0:
                    start_idx = max(0, i - 2)
                    end_idx = i
                    _register(i, start_idx, end_idx, "evening_star", "short", 0.92)
        except Exception:
            logger.exception("TA-Lib pattern detection failed during processing")
        return matches

    # ---- confidence heuristics ----

    def _confidence_from_strength(
        self, score_metric: float, avg_body: float, atr: float, what: str = ""
    ) -> float:
        """
        Build a simple confidence metric (0..1) based on the absolute strength score (like wick_pct or body ratio)
        and the market volatility (ATR).
        """
        # Normalization approach: score_metric likely between 0..1; combine with ATR / avg_body to boost confidence if unique
        base = float(score_metric or 0.0)
        # If `avg_body` is tiny but ATR is larger, lower the confidence a bit
        volatility_adjust = 1.0
        if atr and avg_body:
            volatility_adjust = min(2.0, max(0.5, avg_body / atr if atr > 0 else 1.0))
        conf = base * 0.8 + 0.2 * volatility_adjust
        # clamp
        conf = max(0.0, min(1.0, conf))
        return conf

    def _confidence_engulfing(self, m_prev: Dict, m_curr: Dict, atr: float) -> float:
        # If current body is much larger than previous, confidence grows
        ratio = 0.0
        prev_body = m_prev["body"]
        curr_body = m_curr["body"]
        if prev_body > 0:
            ratio = curr_body / prev_body
        # normalize
        base = min(2.0, ratio) / 2.0  # 0..1
        # a bigger curr body -> stronger
        conf = 0.5 + base * 0.5
        # reduce confidence if ATR is high (noisy) - an arbitrary heuristic
        if atr and curr_body < atr * 0.2:
            conf *= 0.7
        return float(max(0.0, min(1.0, conf)))

    def _confidence_morning_evening(self, m0: Dict, m1: Dict, m2: Dict) -> float:
        # The more the third candle recovers into the first candle's body, the higher the confidence
        m0_mid = (m0["open"] + m0["close"]) / 2.0
        rec_perc = abs((m2["close"] - m0_mid) / (m0["body"] or 1e-9))
        conf = min(1.0, 0.5 + rec_perc)
        return conf

    # ---- trade price generators & refiners ----

    def _ensure_trade_prices(
        self, match: PatternMatch, df_slice: pd.DataFrame
    ) -> Optional[PatternMatch]:
        """
        Ensure entry_hint, stop_loss, take_profit prices are set. If not present, try to compute them.
        """
        if match.entry_hint and match.stop_loss and match.take_profit:
            return match
        # fallback: try to set using end_idx row
        try:
            row = df_slice.iloc[match.end_idx]
            price = float(row["close"])
            atr = compute_atr(df_slice, period=14)
            if match.direction == "long":
                if not match.entry_hint:
                    match.entry_hint = float(row["high"] + (atr * 0.05))
                if not match.stop_loss:
                    match.stop_loss = float(row["low"] - (atr * 0.1))
                if not match.take_profit:
                    risk = abs(match.entry_hint - match.stop_loss)
                    match.take_profit = float(
                        match.entry_hint + (risk * self.cfg.min_risk_reward_ratio)
                    )
            elif match.direction == "short":
                if not match.entry_hint:
                    match.entry_hint = float(row["low"] - (atr * 0.05))
                if not match.stop_loss:
                    match.stop_loss = float(row["high"] + (atr * 0.1))
                if not match.take_profit:
                    risk = abs(match.entry_hint - match.stop_loss)
                    match.take_profit = float(
                        match.entry_hint - (risk * self.cfg.min_risk_reward_ratio)
                    )
            # recompute rr
            if (
                match.entry_hint is not None
                and match.stop_loss is not None
                and match.take_profit is not None
            ):
                denom = (
                    abs(match.entry_hint - match.stop_loss)
                    if match.entry_hint != match.stop_loss
                    else None
                )
                if denom:
                    match.rr = abs((match.take_profit - match.entry_hint) / denom)
            if match.recommended_leverage is None:
                match.recommended_leverage = self._suggest_leverage(match.confidence)
            if match.meta is None:
                match.meta = {}
            match.meta.update({"computed_prices": True})
            return match
        except Exception as e:
            logger.exception(
                "Failed to derive prices for pattern %s: %s", match.pattern_name, e
            )
            return None

    def _upgrade_take_profit_to_min_rr(
        self, match: PatternMatch, df_slice: pd.DataFrame
    ) -> PatternMatch:
        """
        Increase take profit to reach the configured minimum RR if possible, keeping stop_loss constant.
        """
        try:
            min_rr = getattr(self.cfg, "min_risk_reward_ratio", 3.0)
            if match.entry_hint is None or match.stop_loss is None:
                return match
            risk = abs(match.entry_hint - match.stop_loss)
            target_tp = (
                match.entry_hint + (risk * min_rr)
                if match.direction == "long"
                else match.entry_hint - (risk * min_rr)
            )
            match.take_profit = float(target_tp)
            match.rr = min_rr
        except Exception:
            logger.exception("Failed to upgrade TP for match %s", match.pattern_name)
        return match

    def _suggest_leverage(self, confidence: float) -> int:
        """
        Suggest leverage using a normalized mapping of confidence to the maximum allowed
        leverages in config. For crypto use max_leverage_crypto; for forex we don't know
        the symbol here; signal_manager should overwrite if necessary. We map confidence linearly.
        """
        max_leverage = getattr(self.cfg, "max_leverage_crypto", 10)
        # If config min max_... not set, use a conservative default
        max_leverage = max(1, int(max_leverage or 10))
        # map confidence in [0,1] -> [1,max_leverage]
        suggested = 1 + int(round(confidence * (max_leverage - 1)))
        # clamp
        suggested = max(1, min(max_leverage, suggested))
        return suggested


# -------------------------
# Utility function exported
# -------------------------


def detect_patterns_and_suggest_trades(
    df: pd.DataFrame, cfg: Optional[Config] = None, lookback: int = 24
) -> List[PatternMatch]:
    """
    Convenience entrypoint: run detection and return PatternMatch list with price targets.
    """
    cfg = cfg or CONFIG
    det = PatternDetector(cfg)
    return det.detect_patterns(df, lookback=lookback)


# `__all__` vector for import hygiene
__all__ = [
    "PatternDetector",
    "PatternMatch",
    "detect_patterns_and_suggest_trades",
    "compute_atr",
]
