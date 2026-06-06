# -*- coding: utf-8 -*-
"""
Market data module with CoinGecko and AlphaVantage clients and 2H candle resampling.

This module provides:
- MarketDataClient: high-level client to load OHLC data for crypto and forex.
- coin/gecko helper: convert tickers to coin ids and fetch market_chart
- alpha vantage helper: fetch FX_INTRADAY and convert to OHLC
- resampling helpers (pandas-based) to make 2H candles
- automatic caching into the SQLite DB (market_data table)

Notes:
- Crypto uses CoinGecko (no key required). We map symbols like "BTC" -> "bitcoin"
  by fetching `/coins/list` and caching a mapping in memory.
- Forex uses AlphaVantage FX_INTRADAY (requires API key in env var `ALPHAVANTAGE_API_KEY`).
  AlphaVantage returns minutes/60min data, we resample to 2H.
- This module depends on `pandas`, so ensure the environment has it available.
- The client stores consolidated 2H candles into the DB via `db.upsert_market_data_row`.
- Example:
    from simple_trader.market_data import MarketDataClient
    client = MarketDataClient(CONFIG, db)
    df = client.get_2h_ohlc("BTC", lookback_hours=48)
    df2 = client.get_2h_ohlc("EURUSD", lookback_hours=48)

"""

from __future__ import annotations

import logging
import math
import time
from datetime import datetime, timedelta, timezone
from typing import Dict, List, Optional, Tuple

import requests

try:
    import pandas as pd
    from pandas import DataFrame
except Exception:  # pragma: no cover - environment specific
    pd = None  # type: ignore
    DataFrame = None  # type: ignore

# Optional yfinance for excellent coverage of Gold (GC=F), Silver (SI=F), Oil (CL=F), Forex (EURUSD=X)
try:
    import yfinance as yf  # type: ignore
    HAS_YFINANCE = True
except Exception:
    HAS_YFINANCE = False

from simple_trader.config import Config, CONFIG
from simple_trader.db import Database, get_default_db, now_ts

logger = logging.getLogger("simple_trader.market_data")
logger.addHandler(logging.NullHandler())

# CoinGecko endpoints
COINGECKO_API_BASE = "https://api.coingecko.com/api/v3"

# AlphaVantage endpoint
ALPHAVANTAGE_API_BASE = "https://www.alphavantage.co/query"


def _utcts_to_dt(ts_ms: int) -> datetime:
    # timestamps returned by coin gecko are milliseconds; convert to UTC datetime
    return datetime.fromtimestamp(ts_ms / 1000.0, tz=timezone.utc)


def _dt_to_epoch(dt: datetime) -> int:
    return int(dt.astimezone(timezone.utc).timestamp())


def _normalize_symbol(symbol: str) -> str:
    return symbol.strip().upper().replace(" ", "").replace("-", "").replace("\\", "/" if "/" in symbol else "")


def _symbol_is_forex(symbol: str) -> bool:
    """
    Determine if symbol refers to forex pair.
    """
    s = symbol.upper().replace("-", "").replace("/", "").replace("=X", "")
    if len(s) == 6 and s.isalpha():
        fiats = {"USD", "EUR", "GBP", "JPY", "AUD", "NZD", "CAD", "CHF", "CHF"}
        base = s[0:3]
        quote = s[3:6]
        if base in fiats and quote in fiats:
            return True
    return False

def _get_asset_class(symbol: str) -> str:
    """Classify into our focused universe (international + Iranian Bourse)."""
    s = symbol.upper().replace("/", "").replace("-", "").replace("=F", "").replace("=X", "")
    if s in ("XAUUSD", "XAU", "GC", "GOLD"):
        return "GOLD"
    if s in ("XAGUSD", "XAG", "SI", "SILVER"):
        return "SILVER"
    if s in ("CL", "USOIL", "WTI", "OIL", "CRUDE"):
        return "OIL"
    if any(c in s for c in ("BTC", "ETH", "SOL", "XRP")) or s.startswith("CRYPTO"):
        return "CRYPTO"
    if _symbol_is_forex(symbol):
        return "FOREX"
    # Iranian market (Tehran Stock Exchange / Fara Bourse / Codal filings)
    # Supports Persian tickers and English keywords
    if any(k in s for k in ("IRAN", "فولاد", "شستا", "وبملت", "خودرو", "بورس", "کدال", "CODAL", "ETF", "اوراق", "صندوق", "خزانه")):
        if any(k in s for k in ("اوراق", "خزانه", "TREASURY", "BOND")):
            return "IRAN_TREASURY" if "خزانه" in s or "TREASURY" in s else "IRAN_BOND"
        if any(k in s for k in ("صندوق", "FUND", "FIXED_INCOME", "درآمد ثابت")):
            return "IRAN_FIXED_INCOME"
        if "ETF" in s:
            return "IRAN_ETF"
        return "IRAN_STOCK"
    return "OTHER"


class MarketDataClient:
    """
    MarketDataClient handles fetching OHLC data for crypto (via CoinGecko) and forex (AlphaVantage),
    resampling to the configured timeframe (default 2H), caching to DB and returning pandas DataFrames.

    Configuration is obtained via `simple_trader.config.CONFIG` by default, but can be provided.
    """

    def __init__(self, config: Optional[Config] = None, db: Optional[Database] = None):
        self.config = config or CONFIG
        self.db = db or get_default_db(
            self.config.database_path, tenant_id=self.config.tenant_id
        )
        self.session = requests.Session()
        self.session.headers.update({"User-Agent": "SimpleTrader-MarketData/1.0"})
        self.coingecko_symbol_map: Dict[str, str] = {}  # symbol->id for CoinGecko
        self.last_coingecko_list_fetch: Optional[int] = None
        self.coingecko_list_ttl_seconds = 60 * 30  # refresh coin list every 30 minutes
        self.alphavantage_key = self.config.alphavantage_api_key
        # Basic validation
        if pd is None:
            raise RuntimeError("pandas is required for market data processing. Please `pip install pandas`")
        # yfinance is strongly preferred for free coverage of GOLD/SILVER/OIL/FOREX
        if not HAS_YFINANCE:
            logger.warning("yfinance not installed. Install with `pip install yfinance` for best free market data on metals, oil, forex.")

    # Public API ----------------------------------------------------

    def get_2h_ohlc(self, symbol: str, lookback_hours: int = 48, force_refresh: bool = False) -> DataFrame:
        """
        Return a DataFrame containing 2H candles for the given symbol.
        The dataframe index is timezone-aware datetime UTC, and columns are ['open', 'high', 'low', 'close', 'volume'].
        Values are numeric floats; volume may be None for some data sources.

        The method will attempt to retrieve cached data from DB; if `force_refresh` is True
        or cached data is out-of-range for the requested lookback, it will fetch new data and upsert into DB.
        """
        symbol = symbol.strip()
        tframe = "2H"
        now_epoch = now_ts()
        start_epoch = now_epoch - int(lookback_hours * 3600)

        # Attempt to load cached data
        cached_df = self._get_cached_ohlc(symbol, tframe, start_epoch)
        if not force_refresh:
            # cached data is enough if its earliest timestamp <= requested start
            if cached_df is not None and not cached_df.empty:
                first_idx = cached_df.index[0]
                if int(first_idx.timestamp()) <= start_epoch:
                    logger.debug("Using cached market data for %s", symbol)
                    # ensure we return data limited to lookback_hours
                    cutoff = datetime.fromtimestamp(start_epoch, tz=timezone.utc)
                    return cached_df[cached_df.index >= cutoff]

        # If we have no cached data or we need force refresh, call provider
        asset_cls = _get_asset_class(symbol)

        # Iranian assets: limited free OHLC; prefer news/heuristic signals
        if asset_cls.startswith("IRAN"):
            iran_df = self.get_iran_ohlc(symbol, lookback_days=max(1, lookback_hours // 24))
            if iran_df is not None and not iran_df.empty:
                return self._resample_df_to_hours(iran_df, 2)
            # No reliable free candles -> return None so system uses pure news signals (still powerful for IRAN)
            return None

        # Prefer yfinance for Gold/Silver/Oil/Forex (much better data quality for our universe)
        df = None
        if asset_cls in ("GOLD", "SILVER", "OIL", "FOREX") and HAS_YFINANCE:
            df = self._get_yfinance_ohlc(symbol, lookback_hours)

        if df is None or df.empty:
            if _symbol_is_forex(symbol) or asset_cls in ("GOLD", "SILVER", "OIL", "FOREX"):
                df = self._get_forex_ohlc(symbol, lookback_hours)
            else:
                df = self._get_crypto_ohlc(symbol, lookback_hours)

        # Resample to 2H
        df_resampled = self._resample_df_to_hours(df, 2)
        # Cache into db
        self._upsert_df_to_db(df_resampled, symbol, tframe)
        # Trim to requested lookback period
        cutoff_dt = datetime.fromtimestamp(start_epoch, tz=timezone.utc)
        df_resampled = df_resampled[df_resampled.index >= cutoff_dt]
        return df_resampled

    def get_ohlc(self, symbol: str, timeframe_hours: int = 1, lookback_hours: int = 48, force_refresh: bool = False) -> DataFrame:
        """
        Return a DataFrame containing candles resampled to `timeframe_hours`.
        If `timeframe_hours == 2` this defers to `get_2h_ohlc` to reuse caching logic.
        The method will attempt to retrieve cached data from DB; if `force_refresh` is True
        or cached data is out-of-range for the requested lookback, it will fetch new raw data,
        resample to the requested timeframe, upsert it into the DB cache, and return the DataFrame.

        Example: get_ohlc(symbol='BTC', timeframe_hours=1) -> returns 1H OHLC dataframe
        """
        # Normalize and guard
        symbol = symbol.strip()
        if timeframe_hours <= 0:
            raise ValueError("timeframe_hours must be >= 1")

        # If 2H is requested, prefer the specialized method that handles caching/resample semantics
        if timeframe_hours == 2:
            return self.get_2h_ohlc(symbol, lookback_hours=lookback_hours, force_refresh=force_refresh)

        tframe = f"{timeframe_hours}H"
        now_epoch = now_ts()
        start_epoch = now_epoch - int(lookback_hours * 3600)

        # Attempt to load cached data for the requested timeframe
        cached_df = self._get_cached_ohlc(symbol, tframe, start_epoch)
        if not force_refresh:
            if cached_df is not None and not cached_df.empty:
                first_idx = cached_df.index[0]
                if int(first_idx.timestamp()) <= start_epoch:
                    logger.debug("Using cached %s market data for %s", tframe, symbol)
                    cutoff = datetime.fromtimestamp(start_epoch, tz=timezone.utc)
                    return cached_df[cached_df.index >= cutoff]

        asset_cls = _get_asset_class(symbol)

        raw_df = None
        if asset_cls in ("GOLD", "SILVER", "OIL", "FOREX") and HAS_YFINANCE:
            raw_df = self._get_yfinance_ohlc(symbol, lookback_hours)

        if raw_df is None or raw_df.empty:
            if _symbol_is_forex(symbol) or asset_cls in ("GOLD", "SILVER", "OIL", "FOREX"):
                raw_df = self._get_forex_ohlc(symbol, lookback_hours)
            else:
                raw_df = self._get_crypto_ohlc(symbol, lookback_hours)

        if raw_df is None or raw_df.empty:
            logger.debug("No raw market data returned for %s (timeframe %s)", symbol, tframe)
            return raw_df if raw_df is not None else None

        df_resampled = self._resample_df_to_hours(raw_df, timeframe_hours)
        # Cache the resampled timeframe to DB
        try:
            self._upsert_df_to_db(df_resampled, symbol, tframe)
        except Exception:
            logger.exception("Failed to upsert %s market data to DB for %s", tframe, symbol)

        # Trim to requested lookback
        cutoff_dt = datetime.fromtimestamp(start_epoch, tz=timezone.utc)
        df_resampled = df_resampled[df_resampled.index >= cutoff_dt]
        return df_resampled

    # CoinGecko impl ------------------------------------------------

    def _get_coingecko_coin_id(self, symbol: str) -> Optional[str]:
        """
        Convert a symbol like 'BTC' or 'bitcoin' to CoinGecko id 'bitcoin'. This will
        fetch the coin list if needed and keep it cached for `coingecko_list_ttl_seconds`.
        """
        s = symbol.upper()
        # Quick pass: if symbol exactly matches id cached key
        if s in self.coingecko_symbol_map:
            return self.coingecko_symbol_map[s]

        now_stamp = int(time.time())
        if not self.last_coingecko_list_fetch or (now_stamp - (self.last_coingecko_list_fetch or 0)) > self.coingecko_list_ttl_seconds:
            # Refresh coin list
            try:
                logger.debug("Fetching CoinGecko coin list to resolve symbols")
                url = f"{COINGECKO_API_BASE}/coins/list"
                r = self.session.get(url, timeout=20)
                if r.status_code != 200:
                    logger.warning("Failed to fetch CoinGecko list: status %s", r.status_code)
                else:
                    coins = r.json()
                    local_map = {}
                    # build mapping for ticker and id lowercase
                    for coin in coins:
                        cid = coin.get("id")
                        symbol_name = coin.get("symbol", "").upper()
                        # some coins may have symbol duplicates; we don't attempt to de-duplicate heavily
                        local_map[symbol_name] = cid
                        local_map[cid.upper()] = cid
                    self.coingecko_symbol_map.update(local_map)
                    self.last_coingecko_list_fetch = now_stamp
            except Exception:
                logger.exception("Error fetching coin list from CoinGecko")
        # Suggest token id for symbol
        if s in self.coingecko_symbol_map:
            return self.coingecko_symbol_map[s]
        # fallback: accept lowercased symbol itself if user passed full id like "bitcoin"
        if symbol.lower() in self.coingecko_symbol_map.values():
            return symbol.lower()
        return None

    def _get_crypto_ohlc(self, symbol: str, lookback_hours: int = 48) -> DataFrame:
        """
        Return proper OHLCV using CoinGecko /ohlc endpoint (preferred for real candles)
        + volumes from market_chart if available. Much better for candlestick pattern detection
        than deriving OHLC from a price series.
        """
        base_symbol = symbol.split("/")[0] if "/" in symbol else symbol.split("-")[0] if "-" in symbol else symbol
        vs_currency = "usd"
        if "/" in symbol:
            quote = symbol.split("/")[1].lower()
            if quote in ("usdt", "usd", "eur"):
                vs_currency = quote
        coin_id = self._get_coingecko_coin_id(base_symbol)
        if not coin_id:
            raise RuntimeError(f"Unable to map crypto symbol to CoinGecko id: {symbol}")

        days = max(1, math.ceil(lookback_hours / 24))

        # Preferred: real OHLC from /ohlc
        ohlc_url = f"{COINGECKO_API_BASE}/coins/{coin_id}/ohlc"
        params = {"vs_currency": vs_currency, "days": days, "precision": "2"}
        logger.debug("Fetching CoinGecko /ohlc for %s (days=%s)", coin_id, days)
        try:
            r = self.session.get(ohlc_url, params=params, timeout=25)
            if r.status_code == 200:
                ohlc_data = r.json()  # list of [timestamp_ms, open, high, low, close]
                if ohlc_data:
                    idx = []
                    opens, highs, lows, closes = [], [], [], []
                    for row in ohlc_data:
                        if len(row) >= 5:
                            dt = datetime.fromtimestamp(row[0] / 1000.0, tz=timezone.utc)
                            idx.append(dt)
                            opens.append(float(row[1]))
                            highs.append(float(row[2]))
                            lows.append(float(row[3]))
                            closes.append(float(row[4]))
                    if idx:
                        df = pd.DataFrame({
                            "open": opens, "high": highs, "low": lows, "close": closes
                        }, index=pd.DatetimeIndex(idx))
                        df = df.sort_index()
                        # Try to attach volume from market_chart (best effort)
                        try:
                            vol_url = f"{COINGECKO_API_BASE}/coins/{coin_id}/market_chart"
                            vr = self.session.get(vol_url, params={"vs_currency": vs_currency, "days": days}, timeout=20)
                            if vr.status_code == 200:
                                vpayload = vr.json()
                                volumes = vpayload.get("total_volumes", [])
                                if volumes:
                                    vdict = {datetime.fromtimestamp(v[0]/1000.0, tz=timezone.utc): float(v[1]) for v in volumes}
                                    vseries = pd.Series(vdict).sort_index()
                                    vseries = vseries.resample("1H").sum().reindex(df.index, method="nearest")
                                    df["volume"] = vseries
                        except Exception:
                            df["volume"] = None
                        if "volume" not in df.columns:
                            df["volume"] = None
                        return df
        except Exception as e:
            logger.warning("CoinGecko /ohlc fetch failed for %s, falling back: %s", coin_id, e)

        # Fallback: old market_chart price series (poor quality for H/L)
        logger.warning("Falling back to low-quality price-derived candles for %s", symbol)
        days = max(1, math.ceil(lookback_hours / 24))
        url = f"{COINGECKO_API_BASE}/coins/{coin_id}/market_chart"
        params = {"vs_currency": vs_currency, "days": days, "interval": "hourly"}
        r = self.session.get(url, params=params, timeout=25)
        if r.status_code != 200:
            logger.warning("CoinGecko API error status=%s content=%s", r.status_code, r.text)
            r.raise_for_status()
        payload = r.json()
        prices = payload.get("prices", [])
        volumes = payload.get("total_volumes", [])

        if not prices:
            raise RuntimeError(f"No price data returned from CoinGecko for {coin_id}")
        idx = []
        vals = []
        for ts, price in prices:
            idx.append(datetime.fromtimestamp(ts / 1000.0, tz=timezone.utc))
            vals.append(price)
        price_df = pd.DataFrame({"price": vals}, index=pd.DatetimeIndex(idx))
        price_df = price_df.sort_index()
        price_df = price_df.resample("1H").ffill()

        vol_dict = {}
        if volumes:
            for ts, vol in volumes:
                vol_dict[datetime.fromtimestamp(ts / 1000.0, tz=timezone.utc)] = vol
            vol_df = pd.DataFrame({"volume": list(vol_dict.values())}, index=pd.DatetimeIndex(list(vol_dict.keys())))
            vol_df = vol_df.resample("1H").ffill()
            price_df["volume"] = vol_df.reindex(price_df.index)["volume"].ffill()
        else:
            price_df["volume"] = None

        ohlc_1h = price_df["price"].resample("1H").ohlc()
        if "volume" in price_df.columns:
            ohlc_1h["volume"] = price_df["volume"].resample("1H").sum().reindex(ohlc_1h.index)
        ohlc_1h.columns = ["open", "high", "low", "close", "volume"] if "volume" in ohlc_1h.columns else ["open", "high", "low", "close"]
        return ohlc_1h

    def _get_yfinance_ohlc(self, symbol: str, lookback_hours: int = 48) -> Optional[DataFrame]:
        """Use yfinance for Gold/Silver/Oil/Forex when available. Excellent for our focused assets."""
        if not HAS_YFINANCE or pd is None:
            return None

        yf_symbol = symbol
        # Common mappings for futures/spot
        mappings = {
            "XAUUSD": "GC=F", "GOLD": "GC=F", "XAU": "GC=F",
            "XAGUSD": "SI=F", "SILVER": "SI=F",
            "CL": "CL=F", "OIL": "CL=F", "USOIL": "CL=F", "WTI": "CL=F",
            "EURUSD": "EURUSD=X", "GBPUSD": "GBPUSD=X", "USDJPY": "USDJPY=X",
        }
        for k, v in mappings.items():
            if k in symbol.upper():
                yf_symbol = v
                break

        try:
            period = "5d" if lookback_hours <= 120 else "1mo"
            ticker = yf.Ticker(yf_symbol)
            df = ticker.history(period=period, interval="1h")
            if df is None or df.empty:
                return None
            df = df.rename(columns={"Open": "open", "High": "high", "Low": "low", "Close": "close", "Volume": "volume"})
            df = df[["open", "high", "low", "close", "volume"]].dropna(subset=["open"])
            df.index = pd.to_datetime(df.index).tz_localize("UTC") if df.index.tz is None else df.index.tz_convert("UTC")
            logger.debug("yfinance succeeded for %s as %s", symbol, yf_symbol)
            return df
        except Exception as e:
            logger.debug("yfinance failed for %s: %s", symbol, e)
            return None

    # AlphaVantage forex impl -------------------------------------

    def _get_forex_ohlc(self, symbol: str, lookback_hours: int = 48) -> DataFrame:
        """
        Fetch forex intraday candles. 
        Priority: yfinance (free, no key) > AlphaVantage (if key present).
        """
        # Always try yfinance first for free coverage (EURUSD=X etc.)
        if HAS_YFINANCE:
            df = self._get_yfinance_ohlc(symbol, lookback_hours)
            if df is not None and not df.empty:
                return df

        if not self.alphavantage_key:
            logger.warning("No AlphaVantage key and yfinance failed or not available for %s. Returning empty data.", symbol)
            return pd.DataFrame() if pd else None

        # Parse symbol into from_symbol/to_symbol
        s = symbol.upper().replace("-", "").replace("/", "")
        if len(s) < 6:
            raise RuntimeError(f"Invalid forex symbol: {symbol}")
        from_symbol = s[0:3]
        to_symbol = s[3:6]

        params = {
            "function": "FX_INTRADAY",
            "from_symbol": from_symbol,
            "to_symbol": to_symbol,
            "interval": "60min",
            "outputsize": "full",  # try to get enough data
            "apikey": self.alphavantage_key,
        }
        logger.debug("Fetching AlphaVantage FX_INTRADAY for %s/%s", from_symbol, to_symbol)
        # We fetch and parse JSON
        r = self.session.get(ALPHAVANTAGE_API_BASE, params=params, timeout=25)
        if r.status_code != 200:
            logger.warning("AlphaVantage returned non-200 status: %s", r.status_code)
            r.raise_for_status()
        payload = r.json()
        # AlphaVantage returns Data under "Time Series FX (60min)" key; detect key
        ts_key = None
        for key in payload.keys():
            if "Time Series FX" in key:
                ts_key = key
                break
        if not ts_key or ts_key not in payload:
            # Could be an error: { "Note": ... } or { "Error Message": ... }
            logger.warning("AlphaVantage response missing expected time series key: %s", payload.keys())
            raise RuntimeError(f"AlphaVantage response error: {payload}")

        series = payload[ts_key]
        # series: mapping from timestamp -> { "1. open": "...", ...}
        datetimes = []
        opens = []
        highs = []
        lows = []
        closes = []
        volumes = []
        for ts_str, values in series.items():
            # ts_str is like '2023-08-29 23:00:00'
            try:
                dt = datetime.fromisoformat(ts_str).replace(tzinfo=timezone.utc)
            except Exception:
                # fallback parse
                dt = datetime.strptime(ts_str, "%Y-%m-%d %H:%M:%S").replace(tzinfo=timezone.utc)
            datetimes.append(dt)
            opens.append(float(values.get("1. open")))
            highs.append(float(values.get("2. high")))
            lows.append(float(values.get("3. low")))
            closes.append(float(values.get("4. close")))
            # no volume field in FX_INTRADAY; append None
            volumes.append(None)

        # Create DF then sort ascending
        df = pd.DataFrame({"open": opens, "high": highs, "low": lows, "close": closes, "volume": volumes}, index=pd.DatetimeIndex(datetimes))
        df = df.sort_index()
        # We may only have a 60min frequency; ensure it by resampling to 1H (filling missings)
        df = df.resample("1H").agg({"open": "first", "high": "max", "low": "min", "close": "last", "volume": "sum"})
        # Filter for lookback window
        cutoff_dt = datetime.now(timezone.utc) - timedelta(hours=lookback_hours)
        df = df[df.index >= cutoff_dt]
        return df

    # Resampling and DB upsert ------------------------------------

    def _resample_df_to_hours(self, df: DataFrame, hours: int) -> DataFrame:
        """
        Resample a dataframe (with hourly or finer rows) to `hours` timeframe,
        computing OHLC and sum(volume).
        """
        if df is None or df.empty:
            return pd.DataFrame()
        rule = f"{hours}H"
        df = df.copy()
        # Ensure index is timezone-aware and in UTC
        if df.index.tzinfo is None or df.index.tz is None:
            df.index = df.index.tz_localize(timezone.utc)
        else:
            df.index = df.index.tz_convert(timezone.utc)
        ohlc = df.resample(rule).agg({"open": "first", "high": "max", "low": "min", "close": "last", "volume": "sum"})
        # Drop empty rows where open is NaN
        ohlc = ohlc.dropna(subset=["open"])
        return ohlc

    def _upsert_df_to_db(self, df: DataFrame, symbol: str, timeframe: str) -> None:
        """
        Save all candles in the DataFrame into DB using upsert_market_data_row. For each candle, compute start_ts
        as epoch >= 0 int.
        """
        if df is None or df.empty:
            return
        # Ensure index sorted ascending
        df = df.sort_index()
        # iterate rows
        for idx, row in df.iterrows():
            # idx is datetime with timezone
            start_ts = int(idx.timestamp())
            open_ = float(row["open"])
            high = float(row["high"])
            low = float(row["low"])
            close = float(row["close"])
            volume = float(row["volume"]) if (row["volume"] is not None and not (isinstance(row["volume"], float) and math.isnan(row["volume"]))) else None
            try:
                self.db.upsert_market_data_row(symbol, timeframe, start_ts, open_, high, low, close, volume)
            except Exception:
                logger.exception("Failed to upsert market data row for %s at %s", symbol, idx)

    def _get_cached_ohlc(self, symbol: str, timeframe: str, start_ts_min: int, limit: int = 500) -> Optional[DataFrame]:
        """
        Query DB for previously cached candles and return a DataFrame indexed by UTC datetime
        """
        rows = self.db.get_market_data(symbol, timeframe, start_ts_min=start_ts_min, limit=limit)
        if not rows:
            return None
        # Build DataFrame
        idx = []
        opens = []
        highs = []
        lows = []
        closes = []
        volumes = []
        for r in rows:
            ts = int(r["start_ts"])
            idx.append(datetime.fromtimestamp(ts, tz=timezone.utc))
            opens.append(float(r["open"]))
            highs.append(float(r["high"]))
            lows.append(float(r["low"]))
            closes.append(float(r["close"]))
            volumes.append(float(r["volume"]) if r["volume"] is not None else None)
        df = pd.DataFrame({"open": opens, "high": highs, "low": lows, "close": closes, "volume": volumes}, index=pd.DatetimeIndex(idx))
        df = df.sort_index()
        return df

    # Additional utilities ----------------------------------------

    def compute_atr(self, df: DataFrame, period: int = 14) -> float:
        """
        Compute a simple ATR (Average True Range) for a DataFrame with 'high', 'low', 'close' columns.
        Returns the latest ATR value.
        """
        if df is None or df.empty:
            return 0.0
        high = df["high"]
        low = df["low"]
        close = df["close"]
        prev_close = close.shift(1)
        tr1 = high - low
        tr2 = (high - prev_close).abs()
        tr3 = (low - prev_close).abs()
        tr = pd.concat([tr1, tr2, tr3], axis=1).max(axis=1)
        atr = tr.ewm(span=period, min_periods=1).mean()
        return float(atr.iloc[-1])

    def get_latest_price(self, symbol: str) -> Optional[float]:
        """
        Return the latest close price using 2H timeframe - this is a convenience method.
        """
        df = self.get_2h_ohlc(symbol, lookback_hours=6)
        if df is None or df.empty:
            return None
        return float(df["close"].iloc[-1])

    def get_iran_ohlc(self, symbol: str, lookback_days: int = 30) -> Optional[pd.DataFrame]:
        """
        SEPARATE chart sources for Iran (dedicated module because decoupled from global).
        Uses IranMarketData from iran.py with hooks for tsetmc.com, ifb.ir, codal public data.
        Returns None -> Iran-specific news algorithms (Codal/Eghtesad local view) take over.
        This is intentional: Iranian market often moves on domestic narratives even when global markets are stressed.
        """
        try:
            from .iran import IranMarketData
            client = IranMarketData()
            data = client.get_ohlc(symbol)
            if data and isinstance(data, pd.DataFrame) and not data.empty:
                return data
        except Exception:
            pass
        logger.debug("Dedicated Iran chart source returned no OHLC for %s - using separate Iran news/algorithm path.", symbol)
        return None


# Example / debug entrypoint for the module
if __name__ == "__main__":  # pragma: no cover - demo usage
    logging.basicConfig(level=logging.DEBUG)
    cfg = CONFIG
    db = get_default_db(cfg.database_path, tenant_id=cfg.tenant_id)
    md = MarketDataClient(cfg, db=db)
    try:
        print("Getting 2H OHLC for BTC...")
        df = md.get_2h_ohlc("BTC", lookback_hours=48)
        print(df.tail(5))
    except Exception as e:
        logger.exception("Error fetching crypto OHLC: %s", e)
    try:
        print("Getting 2H OHLC for EURUSD (forex)...")
        df_fx = md.get_2h_ohlc("EURUSD", lookback_hours=48)
        print(df_fx.tail(5))
    except Exception as e:
        logger.exception("Error fetching forex OHLC: %s", e)
