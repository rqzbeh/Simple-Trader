"""
Technical + Meta Indicators for Simple-Trader

Provides a rich set of indicators that feed:
- Heuristic analyzer (confluence for direction/conf)
- Scorer (new features: rsi, macd, bb, vol_regime, confluence, etc.)
- Decision audit (stored for cause attribution and ML)
- Regime-adaptive logic
- Meta-indicators from public data (politician impact, whale velocity)

All functions are pandas/numpy only (TA-Lib optional for speed/accuracy).
Designed for 1H/2H resampled data but works on any OHLCV df with columns:
['open','high','low','close','volume'] (volume optional for some metrics).

Key outputs are normalized where possible (0-1 or z-scored) for direct use in ML.
"""

from __future__ import annotations

import json
import logging
from typing import Dict, Optional

import numpy as np
import pandas as pd

try:
    import talib  # type: ignore
    HAS_TALIB = True
except Exception:
    HAS_TALIB = False

logger = logging.getLogger("simple_trader.indicators")
logger.addHandler(logging.NullHandler())

# ----------------------
# Core Technical Indicators
# ----------------------

def compute_rsi(df: pd.DataFrame, period: int = 14) -> pd.Series:
    """Relative Strength Index. Returns series 0-100 (or NaN)."""
    close = df["close"].astype(float)
    if HAS_TALIB:
        try:
            return pd.Series(talib.RSI(close.values, timeperiod=period), index=df.index)
        except Exception:
            pass
    # Pure pandas fallback (Wilder's smoothing)
    delta = close.diff()
    gain = delta.where(delta > 0, 0.0)
    loss = -delta.where(delta < 0, 0.0)
    avg_gain = gain.ewm(alpha=1/period, adjust=False).mean()
    avg_loss = loss.ewm(alpha=1/period, adjust=False).mean()
    rs = avg_gain / avg_loss.replace(0, np.nan)
    rsi = 100 - (100 / (1 + rs))
    return rsi.clip(0, 100)

def compute_macd(df: pd.DataFrame, fast: int = 12, slow: int = 26, signal: int = 9) -> Dict[str, pd.Series]:
    """MACD, signal line, histogram. Returns dict with 'macd', 'signal', 'hist'."""
    close = df["close"].astype(float)
    if HAS_TALIB:
        try:
            macd, macdsignal, macdhist = talib.MACD(close.values, fastperiod=fast, slowperiod=slow, signalperiod=signal)
            return {
                "macd": pd.Series(macd, index=df.index),
                "signal": pd.Series(macdsignal, index=df.index),
                "hist": pd.Series(macdhist, index=df.index),
            }
        except Exception:
            pass
    ema_fast = close.ewm(span=fast, adjust=False).mean()
    ema_slow = close.ewm(span=slow, adjust=False).mean()
    macd_line = ema_fast - ema_slow
    signal_line = macd_line.ewm(span=signal, adjust=False).mean()
    hist = macd_line - signal_line
    return {"macd": macd_line, "signal": signal_line, "hist": hist}

def compute_bollinger(df: pd.DataFrame, period: int = 20, std_dev: float = 2.0) -> Dict[str, pd.Series]:
    """Bollinger Bands. Returns 'upper', 'middle', 'lower', 'percent_b', 'bandwidth'."""
    close = df["close"].astype(float)
    if HAS_TALIB:
        try:
            upper, middle, lower = talib.BBANDS(close.values, timeperiod=period, nbdevup=std_dev, nbdevdn=std_dev)
            upper_s = pd.Series(upper, index=df.index)
            middle_s = pd.Series(middle, index=df.index)
            lower_s = pd.Series(lower, index=df.index)
        except Exception:
            sma = close.rolling(window=period).mean()
            std = close.rolling(window=period).std()
            upper_s = sma + (std * std_dev)
            middle_s = sma
            lower_s = sma - (std * std_dev)
    else:
        sma = close.rolling(window=period).mean()
        std = close.rolling(window=period).std()
        upper_s = sma + (std * std_dev)
        middle_s = sma
        lower_s = sma - (std * std_dev)

    percent_b = (close - lower_s) / (upper_s - lower_s).replace(0, np.nan)
    bandwidth = (upper_s - lower_s) / middle_s.replace(0, np.nan)
    return {
        "upper": upper_s,
        "middle": middle_s,
        "lower": lower_s,
        "percent_b": percent_b.clip(0, 1),
        "bandwidth": bandwidth,
    }

def compute_atr_enhanced(df: pd.DataFrame, period: int = 14) -> Dict[str, float | pd.Series]:
    """
    Enhanced ATR: returns both raw ATR series and normalized ATR% + simple regime flag.
    Normalized ATR% = ATR / close (recent average).
    vol_regime: 1 if current ATR% > 75th percentile of last 100 bars (high vol), else 0.
    """
    high = df["high"].astype(float)
    low = df["low"].astype(float)
    close = df["close"].astype(float)

    if HAS_TALIB:
        try:
            atr = pd.Series(talib.ATR(high.values, low.values, close.values, timeperiod=period), index=df.index)
        except Exception:
            tr = pd.concat([high - low, (high - close.shift()).abs(), (low - close.shift()).abs()], axis=1).max(axis=1)
            atr = tr.ewm(alpha=1/period, adjust=False).mean()
    else:
        tr = pd.concat([high - low, (high - close.shift()).abs(), (low - close.shift()).abs()], axis=1).max(axis=1)
        atr = tr.ewm(alpha=1/period, adjust=False).mean()

    atr_pct = (atr / close).replace(0, np.nan)
    # Simple regime: high vol if current atr_pct > 75th pct of recent window
    recent_atr = atr_pct.rolling(100, min_periods=20).quantile(0.75)
    vol_regime = (atr_pct > recent_atr).astype(float).fillna(0.0)

    return {
        "atr": atr,
        "atr_pct": atr_pct,
        "vol_regime": vol_regime,  # 0 or 1 (high vol flag)
        "atr_pct_current": float(atr_pct.iloc[-1]) if len(atr_pct) > 0 else 0.0,
    }

def compute_volume_metrics(df: pd.DataFrame, window: int = 20) -> Dict[str, pd.Series | float]:
    """OBV + volume z-score + relative volume."""
    close = df["close"].astype(float)
    vol = df.get("volume")
    if vol is None or vol.isna().all():
        return {"obv": pd.Series(0.0, index=df.index), "vol_zscore": pd.Series(0.0, index=df.index), "rel_volume": 1.0}

    vol = vol.astype(float).fillna(0)
    obv = (np.sign(close.diff()) * vol).cumsum()
    vol_mean = vol.rolling(window, min_periods=5).mean()
    vol_std = vol.rolling(window, min_periods=5).std().replace(0, 1)
    vol_z = ((vol - vol_mean) / vol_std).clip(-3, 3)
    rel_vol = (vol / vol_mean).replace(0, 1).clip(0.1, 5)

    return {
        "obv": obv,
        "vol_zscore": vol_z,
        "rel_volume": rel_vol,
        "rel_volume_current": float(rel_vol.iloc[-1]) if len(rel_vol) > 0 else 1.0,
    }

# ----------------------
# Confluence & Regime
# ----------------------

def compute_indicator_confluence(df: pd.DataFrame) -> Dict[str, float]:
    """
    Returns a dict of normalized indicators + overall confluence score (0-1).
    Higher score = stronger directional agreement across indicators.
    """
    if len(df) < 30:
        return {"confluence": 0.5, "rsi": 50.0, "macd_hist": 0.0, "bb_percent_b": 0.5, "vol_regime": 0.0}

    rsi = compute_rsi(df).iloc[-1]
    macd = compute_macd(df)
    macd_hist = macd["hist"].iloc[-1] if len(macd["hist"]) > 0 else 0.0
    bb = compute_bollinger(df)
    bb_pb = bb["percent_b"].iloc[-1] if len(bb["percent_b"]) > 0 else 0.5
    atr_info = compute_atr_enhanced(df)
    vol_reg = atr_info.get("vol_regime", pd.Series([0.0])).iloc[-1]

    # Normalize
    rsi_norm = (rsi - 50) / 50.0  # -1 to +1
    macd_norm = np.tanh(macd_hist / (df["close"].iloc[-1] * 0.01 + 1e-9))  # rough scale
    bb_norm = (bb_pb - 0.5) * 2  # -1 to +1 (extreme mean-reversion)
    vol_norm = float(vol_reg)

    # Simple directional confluence (positive = bullish bias)
    # Weight: RSI + MACD momentum, BB for extremes, reduce in high vol
    raw = (0.4 * rsi_norm + 0.4 * macd_norm + 0.2 * bb_norm) * (1 - 0.3 * vol_norm)
    confluence = float(np.clip((raw + 1) / 2, 0.0, 1.0))  # map to 0-1

    return {
        "confluence": round(confluence, 4),
        "rsi": round(float(rsi), 2),
        "macd_hist": round(float(macd_hist), 6),
        "bb_percent_b": round(float(bb_pb), 4),
        "vol_regime": round(float(vol_norm), 2),
        "rsi_norm": round(float(rsi_norm), 4),
        "macd_norm": round(float(macd_norm), 4),
        "bb_norm": round(float(bb_norm), 4),
    }

def get_regime_from_indicators(df: pd.DataFrame) -> str:
    """Simple regime label for adaptive logic."""
    if len(df) < 20:
        return "unknown"
    atr = compute_atr_enhanced(df)
    vol_series = atr.get("vol_regime", pd.Series([0.0]))
    vol = float(vol_series.iloc[-1]) if hasattr(vol_series, 'iloc') else float(vol_series or 0)
    close = df["close"].iloc[-1]
    sma20 = df["close"].rolling(20).mean().iloc[-1]
    trend = "trending" if abs(close - sma20) / sma20 > 0.03 else "ranging"
    vol_label = "high_vol" if vol > 0.5 else "low_vol"
    return f"{vol_label}_{trend}"

# ----------------------
# Simple in-memory cache for expensive indicator computations (performance opt)
# ----------------------
_indicator_cache: dict = {}  # key -> (result, ts)

def _get_cache_key(symbol: str, lookback: int) -> str:
    return f"{symbol}:{lookback}"

def compute_all_indicators_cached(df: pd.DataFrame, symbol: str = "UNKNOWN", max_age_sec: int = 120) -> Dict[str, float | str]:
    """Cached version of compute_all_indicators for dashboard / frequent calls."""
    import time
    key = _get_cache_key(symbol, len(df) if df is not None else 0)
    now = time.time()
    if key in _indicator_cache:
        res, ts = _indicator_cache[key]
        if now - ts < max_age_sec:
            return res
    res = compute_all_indicators(df)
    _indicator_cache[key] = (res, now)
    # crude cleanup
    if len(_indicator_cache) > 200:
        for k in list(_indicator_cache.keys())[:50]:
            _indicator_cache.pop(k, None)
    return res

# ----------------------
# Public API
# ----------------------

def compute_all_indicators(df: pd.DataFrame) -> Dict[str, float | str]:
    """Convenience: returns flat dict suitable for decision_audit and scorer features."""
    conf = compute_indicator_confluence(df)
    regime = get_regime_from_indicators(df)
    atr = compute_atr_enhanced(df)
    volm = compute_volume_metrics(df)

    return {
        "indicator_confluence": conf["confluence"],
        "rsi": conf["rsi"],
        "macd_hist": conf["macd_hist"],
        "bb_percent_b": conf["bb_percent_b"],
        "vol_regime": conf["vol_regime"],
        "atr_pct": atr.get("atr_pct_current", 0.0),
        "rel_volume": volm.get("rel_volume_current", 1.0),
        "regime_label": regime,
    }

def get_indicator_features_for_signal(df: pd.DataFrame) -> Dict[str, float]:
    """Returns only the numeric features to append to scorer row (safe for ML)."""
    all_ind = compute_all_indicators(df)
    return {
        "ind_confluence": all_ind["indicator_confluence"],
        "ind_rsi_norm": (all_ind["rsi"] - 50) / 50.0,
        "ind_macd_norm": np.tanh(all_ind["macd_hist"] / 10.0),  # rough
        "ind_bb_norm": (all_ind["bb_percent_b"] - 0.5) * 2,
        "ind_vol_regime": all_ind["vol_regime"],
        "ind_atr_pct": all_ind["atr_pct"],
        "ind_rel_volume": min(all_ind["rel_volume"], 3.0),
    }

# ----------------------
# Meta-Indicators (Public Data Edge - Simons style)
# These use existing regret/cause data + public signals for "statistical factors"
# ----------------------

def compute_politician_disclosure_impact(db, symbol: str, asset_class: str = None, lookback_days: int = 60) -> float:
    """
    Meta-indicator: How "toxic" has politician disclosure activity been for this symbol/asset recently?
    Returns negative penalty (e.g. -0.15) if recent losses attributed to disclosures without hedge.
    Uses cause_weights + regret_table (already populated by the ML loop).
    """
    try:
        if not hasattr(db, "get_cause_weights"):
            return 0.0
        weights = db.get_cause_weights() or {}
        penalty = 0.0
        bad_causes = [c for c in weights if "politician" in c.lower() or "disclosure" in c.lower() or "insufficient_hedge" in c.lower()]
        for c in bad_causes:
            w = weights.get(c, 0.0)
            if w < 0:
                penalty += w * 0.6  # dampened
        # Extra hit for this asset class if many recent regrets
        if hasattr(db, "get_regrets"):
            regrets = db.get_regrets(limit=30) or []
            recent_bad = 0
            for r in regrets:
                if (r.get("symbol") or "").upper() == (symbol or "").upper():
                    causes = []
                    try:
                        causes = json.loads(r.get("causes", "[]"))
                    except:
                        pass
                    if any("politician" in str(c).lower() or "disclosure" in str(c).lower() for c in causes):
                        recent_bad += 1
            if recent_bad >= 2:
                penalty -= 0.08 * recent_bad
        return max(-0.35, min(0.05, penalty))
    except Exception:
        return 0.0

def compute_whale_flow_velocity(db_or_data: dict | list, window: int = 5) -> float:
    """
    Enhanced whale meta-indicator: velocity / net pressure from recent large moves.
    Expects list of recent whale items or raw data. Returns z-like score (-1 to +1).
    Positive = accumulation (bullish bias for crypto).
    """
    try:
        values = []
        if isinstance(db_or_data, (list, tuple)):
            for item in db_or_data[-window*2:]:
                val = float(item.get("value", 0) or 0)
                if val > 0:
                    values.append(val)
        if not values:
            return 0.0
        arr = np.array(values[-window:])
        z = (arr[-1] - arr.mean()) / (arr.std() + 1e-9) if len(arr) > 1 else 0.0
        return float(np.clip(z, -2, 2) / 2.0)  # scale to ~ -1..1
    except Exception:
        return 0.0

def compute_cross_asset_regime(corr_df: pd.DataFrame = None, gold_perf: float = None, crypto_perf: float = None) -> float:
    """
    Simple cross-asset regime indicator (e.g. Crypto vs Gold correlation flip as risk signal).
    Returns value in [-1, 1]: negative = risk-off (increase hedge).
    """
    try:
        if corr_df is not None and not corr_df.empty and "GOLD" in corr_df and "CRYPTO" in corr_df:
            recent = corr_df[["GOLD", "CRYPTO"]].tail(20).corr().iloc[0,1]
            return float(np.clip(-recent * 1.5, -1, 1))  # flip for hedge signal
        if gold_perf is not None and crypto_perf is not None:
            # Rough: if gold rising while crypto falling -> risk off
            return float(np.clip((gold_perf - crypto_perf) * 2, -1, 1))
        return 0.0
    except Exception:
        return 0.0

def compute_volume_news_velocity(news_count_4h: int = 0, avg_news: float = 3.0, rel_volume: float = 1.0) -> float:
    """Combines volume surge + news flow into a single 'event velocity' indicator."""
    try:
        news_z = (news_count_4h - avg_news) / max(1.0, avg_news)
        event = (rel_volume - 1.0) * 0.6 + news_z * 0.4
        return float(np.clip(event, -1.5, 2.5))
    except Exception:
        return 0.0

# Convenience to attach meta indicators to a signal dict (called from signal_manager or process)
def attach_meta_indicators(signal_row: dict, db=None, recent_whales: list = None, news_velocity: dict = None) -> dict:
    """Mutates and returns signal_row with meta indicator fields for audit + scorer."""
    try:
        sym = signal_row.get("symbol", "")
        ac = signal_row.get("asset_class", "")
        pol_impact = compute_politician_disclosure_impact(db, sym, ac) if db else 0.0
        whale_vel = compute_whale_flow_velocity(recent_whales or []) if recent_whales else 0.0
        cross = compute_cross_asset_regime()
        vel = compute_volume_news_velocity(** (news_velocity or {}))
        meta = signal_row.setdefault("meta_indicators", {})
        meta.update({
            "politician_impact": round(pol_impact, 4),
            "whale_velocity": round(whale_vel, 4),
            "cross_asset_regime": round(cross, 4),
            "event_velocity": round(vel, 4),
        })
        return signal_row
    except Exception:
        return signal_row