# Simple-Trader/simple_trader/config.py
# -*- coding: utf-8 -*-
"""
Configuration loader for Simple-Trader.

This module provides a typed dataclass `Config` that centralizes environment-driven
settings used by the rest of the codebase. It uses sane defaults so the system can be
run out-of-the-box for testing (reading market data from freely available sources, using
RSS feeds for news) but allows overriding everything with environment variables.

Design goals:
- Keep secrets out of the source code: read credentials from environment variables.
- Provide explicit rate limits and concurrency knobs for LLM providers.
- Include market session mapping for forex (Sydney/London/Tokyo/New York).
- Provide sensible defaults for risk management: max leverage, min RR, timeframe, trade duration.
- Provide RSS & API defaults for news and market data.
"""

from __future__ import annotations

import logging
import os
from dataclasses import dataclass, field, asdict
from datetime import timedelta
from typing import Dict, List, Optional, Tuple

# A small helper to convert env strings to lists
def _parse_csv(value: Optional[str], default: Optional[List[str]] = None) -> List[str]:
    if not value:
        return default or []
    return [p.strip() for p in value.split(",") if p.strip()]


def _getenv_float(key: str, default: float) -> float:
    val = os.getenv(key)
    try:
        if val is None or val == "":
            return default
        return float(val)
    except Exception:
        return default


def _getenv_int(key: str, default: int) -> int:
    val = os.getenv(key)
    try:
        if val is None or val == "":
            return default
        return int(val)
    except Exception:
        return default


def _getenv_bool(key: str, default: bool) -> bool:
    val = os.getenv(key)
    if val is None:
        return default
    val_lower = val.strip().lower()
    return val_lower in ("1", "true", "yes", "y", "on")


@dataclass
class LLMProviderConfig:
    """
    Lightweight config for each LLM provider.
    The system will create an LLMPool (or equivalent) from this info and use
    per-provider rate-limits / concurrency to distribute requests.

    The schema matches OpenAI-style chat endpoints (model + messages) where possible.
    """
    name: str
    api_key: Optional[str] = None
    # Optional HTTP endpoint for the provider. If None, a sensible default for the provider will be used.
    endpoint: Optional[str] = None
    # Optional model name for OpenAI-compatible endpoints (e.g., "gpt-4o-mini").
    model: Optional[str] = None
    # Requests per minute that should not be exceeded for this provider.
    rate_limit_per_minute: int = 10
    # Weight for how many news items the provider should get relative to others (used to round-robin/weighted split).
    weight: int = 1
    # Whether to enable this provider
    enabled: bool = True
    # Optional HTTP header to use for the API key (e.g., "Authorization" or "X-API-Key")
    auth_header: Optional[str] = None
    # Optional prefix for the API key header value (e.g., "Bearer ")
    api_key_prefix: Optional[str] = None


@dataclass
class Config:
    """
    Central runtime configuration. Call `Config.from_env()` to build from current environment.
    """
    # Basic / runtime
    database_path: str = "simple_trader.db"
    tenant_id: str = "default"
    log_level: str = field(default="INFO")
    timezone: str = "UTC"

    # Account / risk management (tuned for hedge fund / production usage)
    # - Slightly larger default account balance for example deployments and a more conservative risk per trade.
    account_balance_usd: float = 200_000.0
    risk_per_trade_pct: float = 0.005  # 0.5% of account by default per trade (safer by default for production)
    min_risk_reward_ratio: float = 3.0  # Minimum risk-reward ratio; keep the policy at 3.0 minimum
    stop_loss_slippage_pct: float = 0.001  # 0.1% slippage assumed for entry/exit
    # Lower crypto leverage by default; hedge funds commonly reduce lever when automated signals are used.
    max_leverage_crypto: int = 5
    # Slightly reduce forex leverage to be conservative while still offering adequate exposure
    max_leverage_forex: int = 20

    # Timeframes
    timeframe_hours: int = 2  # 2-hour timeframe is our primary analysis window
    max_trade_duration_hours: int = 24  # No trade should be open for more than 24 hours
    news_max_age_hours: int = 24  # Ignore news older than this for triggering new signals

    # News & providers (default RSS + optional API keys)
    news_rss_feeds: List[str] = field(default_factory=list)
    news_api_key: Optional[str] = None  # e.g., NewsAPI key
    cryptonews_api_key: Optional[str] = None

    # Market Data providers
    alphavantage_api_key: Optional[str] = None  # For forex OHLC intraday
    # For crypto we use CoinGecko (no key required) by default.

    # LLMs: read keys and endpoints from env variables. We'll build a list of providers.
    llm_providers: List[LLMProviderConfig] = field(default_factory=list)
    # Slightly reduce global concurrency to better stay within provider free tiers without throttling spikes.
    llm_global_concurrency: int = 4  # how many concurrent LLM requests at any given time
    # Increase base LLM confidence threshold to reduce noise; must be in [0.0,1.0].
    llm_min_confidence: float = 0.7  # LLM must provide a min confidence to be considered

    # Scalars for filtering candlestick vs LLM disagreement
    # Strength required from a candlestick pattern to be considered; raising this reduces false positives.
    min_pattern_confidence: float = 0.7

    # Telegram (send signals)
    telegram_bot_token: Optional[str] = None
    telegram_chat_id: Optional[str] = None

    # Misc
    open_positions_limit: int = 10  # limit concurrently open signals
    backtest_mode: bool = False  # if true, the system won't send Telegram messages by default

    # Market session mapping (useful for forex trading)
    session_map: Dict[str, Tuple[int, int]] = field(default_factory=dict)
    # Map of base currencies to preferred trading sessions
    currency_session_map: Dict[str, List[str]] = field(default_factory=dict)

    # Misc monitoring
    enable_telemetry: bool = True

    # ============================================================
    # FULL INVESTING SYSTEM - Company Secret Formula Config
    # Focus: GOLD, SILVER, CRYPTO, FOREX, OIL only.
    # Philosophy: Alpha (news + short-term high-conviction) for profit,
    #             Core (Gold/Silver) for capital preservation + hedging.
    # ============================================================

    # Asset universe we trade (internal classification)
    # Supported: 'GOLD', 'SILVER', 'CRYPTO', 'FOREX', 'OIL'
    allowed_asset_classes: List[str] = field(default_factory=lambda: ["GOLD", "SILVER", "CRYPTO", "FOREX", "OIL"])

    # Bucket allocation (percent of account risk budget)
    # Core = defensive, preservation, hedge (Gold + Silver)
    # Alpha = aggressive short-term news/pattern trades (Crypto + Forex + Oil)
    core_bucket_target_pct: float = 0.55   # 55% defensive
    alpha_bucket_target_pct: float = 0.45  # 45% aggressive

    # Per asset-class hard exposure limits (of total account)
    max_exposure_gold: float = 0.40
    max_exposure_silver: float = 0.15
    max_exposure_crypto: float = 0.25
    max_exposure_forex: float = 0.20
    max_exposure_oil: float = 0.15

    # Book-level risk controls (circuit breakers)
    max_book_risk_pct: float = 0.08          # Max total risk across all open positions
    max_daily_loss_pct: float = 0.03         # Pause new alpha if daily loss > this
    max_drawdown_pause_pct: float = 0.08     # Global kill switch / pause on drawdown from peak
    circuit_breaker_cooldown_hours: int = 24 # How long to pause after breaker

    # Hedge configuration (use Gold/Silver to protect Alpha)
    enable_auto_hedge: bool = True
    hedge_ratio: float = 0.4                 # For every $1 aggressive risk, hedge ~$0.4 in gold/silver
    hedge_rebalance_threshold: float = 0.15  # Re-hedge when net exposure drifts >15%

    # Advanced sizing
    use_vol_targeting: bool = True
    target_vol_pct: float = 0.012            # Target ~1.2% daily vol for alpha positions (adjustable)
    fractional_kelly_factor: float = 0.25    # Use only 25% of Kelly to be conservative

    # Focused symbols (expand as needed). These are the instruments we actually trade.
    # Format: internal name -> data provider symbol(s)
    focused_symbols: Dict[str, List[str]] = field(default_factory=lambda: {
        "GOLD": ["XAUUSD", "GC=F", "XAU/USD"],           # Gold spot / futures
        "SILVER": ["XAGUSD", "SI=F", "XAG/USD"],
        "OIL": ["CL=F", "USOIL", "WTI", "OILUSD"],       # WTI Crude
        "CRYPTO": ["BTC", "ETH", "BTCUSDT", "ETHUSDT"], # Major coins only for now
        "FOREX": ["EURUSD", "GBPUSD", "USDJPY", "AUDUSD", "USDCAD"],  # Liquid majors
    })

    # Asset class to bucket mapping
    asset_class_to_bucket: Dict[str, str] = field(default_factory=lambda: {
        "GOLD": "CORE",
        "SILVER": "CORE",
        "CRYPTO": "ALPHA",
        "FOREX": "ALPHA",
        "OIL": "ALPHA",
        "WHALE": "ALPHA",      # on-chain whale moves feed crypto alpha
        "POLITICS": "ALPHA",   # politician/policy headlines as sentiment alpha
    })

    # Regime / news impact (for future regime detection)
    news_impact_boost_alpha: float = 1.5     # Multiply signal strength on high-impact news for alpha assets
    gold_hedge_on_risk_off: bool = True      # Auto increase gold hedge on "risk-off" news (LLM flagged)

    # Whale & Politician special signals (free/public data focus, zero extra keys)
    # Whale addresses: public on-chain wallets to monitor for large moves (user populates with known public ones)
    # BTC queries via free blockchain.info; ETH via public explorers (rate limited, no key for basic)
    whale_addresses: Dict[str, List[str]] = field(default_factory=lambda: {
        "BTC": [],  # e.g. add public Trump-related or known whale BTC addresses
        "ETH": [],  # public ETH addresses
    })
    whale_min_size_btc: float = 100.0
    whale_min_size_eth: float = 1000.0

    # Politicians: names/keywords for parsing disclosures or news (Trump family crypto, Pelosi trades, congress energy/crypto bills)
    monitor_politicians: List[str] = field(default_factory=lambda: ["Trump", "Pelosi", "congress", "senate", "disclosure", "WLFI"])

    def as_dict(self) -> Dict:
        return asdict(self)

    def validate(self) -> None:
        """Assert that the config is sane."""
        tenant = (self.tenant_id or "").strip()
        if not tenant:
            raise ValueError("tenant_id must not be empty")
        if len(tenant) > 64:
            raise ValueError("tenant_id length must be <= 64")
        if not all(c.isalnum() or c in ("-", "_") for c in tenant):
            raise ValueError("tenant_id must contain only letters, numbers, '-' or '_'")
        if self.min_risk_reward_ratio < 1.0:
            raise ValueError("min_risk_reward_ratio must be >= 1")
        if self.risk_per_trade_pct <= 0 or self.risk_per_trade_pct > 0.2:
            # Very conservative bounds; a hedge fund shouldn't risk > 20% of capital on one trade
            raise ValueError("risk_per_trade_pct must be >0 and <= 0.2")
        if self.timeframe_hours <= 0:
            raise ValueError("timeframe_hours must be positive")
        if self.max_trade_duration_hours < self.timeframe_hours:
            # the max trade duration should at least be one timeframe
            raise ValueError("max_trade_duration_hours must be >= timeframe_hours")
        if self.open_positions_limit <= 0:
            raise ValueError("open_positions_limit must be > 0")
        if not (0.0 < self.llm_min_confidence <= 1.0):
            raise ValueError("llm_min_confidence must be in (0.0, 1.0]")

        # Full system validations
        if abs(self.core_bucket_target_pct + self.alpha_bucket_target_pct - 1.0) > 0.01:
            raise ValueError("core_bucket_target_pct + alpha_bucket_target_pct must be ~1.0")
        for pct in [self.max_exposure_gold, self.max_exposure_silver, self.max_exposure_crypto,
                    self.max_exposure_forex, self.max_exposure_oil]:
            if not (0 <= pct <= 0.6):
                raise ValueError("Per-asset max exposure must be between 0 and 60%")
        if self.max_book_risk_pct > 0.15:
            raise ValueError("max_book_risk_pct > 15% is insane for a serious investment company")
        if self.max_daily_loss_pct <= 0 or self.max_daily_loss_pct > 0.1:
            raise ValueError("max_daily_loss_pct must be positive and reasonable (<=10%)")

    @property
    def sqlite_connection_string(self) -> str:
        # Useful replacement for SQLAlchemy or sqlite3 connection.
        # We'll use simple relative sqlite path by default.
        if self.database_path.startswith("sqlite:///"):
            return self.database_path
        return f"sqlite:///{self.database_path}"

    def get_llm_providers_enabled(self) -> List[LLMProviderConfig]:
        return [p for p in self.llm_providers if p.enabled and p.api_key]

    def provider_is_enabled(self, name: str) -> bool:
        for p in self.llm_providers:
            if p.name.lower() == name.lower():
                return p.enabled and bool(p.api_key)
        return False

    def token_bucket_capacity_per_minute(self) -> int:
        """
        Return total active capacity per minute across all enabled LLM providers.
        This is useful for batch sizing / scheduling.
        """
        total = 0
        for p in self.get_llm_providers_enabled():
            total += int(p.rate_limit_per_minute * (p.weight or 1))
        # enforce a minimum of 1 per minute
        return max(1, total)


def _session_defaults() -> Dict[str, Tuple[int, int]]:
    """
    Forex market sessions in UTC. These are approximate and don't take DST into account;
    they are here to help choose which session to prefer when picking forex trades.

    Each tuple is (start_utc_hour, end_utc_hour), 24-hour clock.
    If end_utc_hour <= start_utc_hour then the session wraps across midnight.
    """
    return {
        "sydney": (22, 7),  # 22:00 - 07:00 UTC (wraps midnight)
        "tokyo": (0, 9),  # 00:00 - 09:00 UTC
        "london": (7, 16),  # 07:00 - 16:00 UTC
        "new_york": (12, 21),  # 12:00 - 21:00 UTC
    }


def _currency_to_sessions_defaults() -> Dict[str, List[str]]:
    """
    Map a base currency symbol to the most active sessions for that currency.
    This is intentionally generic and can be tuned by the StrategyTuner later.
    """
    return {
        "AUD": ["sydney", "tokyo"],  # EUR/AUD etc: Australia influence
        "NZD": ["sydney"],
        "JPY": ["tokyo", "sydney"],
        "SGD": ["tokyo"],
        "EUR": ["london", "new_york"],
        "GBP": ["london", "new_york"],
        "CHF": ["london", "new_york"],
        "USD": ["new_york", "london"],
        "CAD": ["new_york", "london"],
    }


def get_default_rss_feeds() -> List[str]:
    """
    Maximum free public RSS for our focused universe (GOLD/SILVER/OIL/FOREX/CRYPTO).
    Zero API keys. Prioritizes commodity and macro sources.
    """
    return [
        # Gold / Silver / Metals (excellent free sources)
        "https://www.kitco.com/rss/",
        "https://news.goldseek.com/goldseek.rss",
        # Oil / Energy / Commodities
        "https://oilprice.com/rss",
        "https://www.reuters.com/markets/commodities/rss",
        # Forex + Macro (risk sentiment, rates)
        "https://www.reuters.com/finance/markets/rss",
        "https://www.investing.com/rss/news.rss",
        "https://feeds.marketwatch.com/marketwatch/topstories/",
        # Crypto (for sentiment cross-asset)
        "https://cointelegraph.com/rss",
        "https://cryptonews.com/news/feed/",
        # Quality macro
        "https://www.ft.com/markets?format=rss",
        # Whale & on-chain sentiment (free RSS aggregators)
        "https://cryptoslate.com/feed/",
        "https://www.theblock.co/rss",
        # Politician / policy trades & crypto policy (headlines often move markets)
        "https://www.reuters.com/markets/cryptocurrencies/rss",
        "https://www.bloomberg.com/feeds/markets.rss",
    ]


def _default_llm_provider_envs() -> List[LLMProviderConfig]:
    """
    Build the default LLM provider list from environment variables.
    For production, each provider SHOULD have an api key specified in the env.

    Environment variables:
      - GROQ_API_KEY, GROQ_ENDPOINT, GROQ_RATE_LIMIT_PER_MINUTE
      - CLOUDFLARE_API_KEY, CLOUDFLARE_ACCOUNT, CLOUDFLARE_RATE_LIMIT_PER_MINUTE
      - GOOGLE_AI_API_KEY, GOOGLE_AI_RATE_LIMIT_PER_MINUTE
    """
    providers: List[LLMProviderConfig] = []

    # Groq
    groq_key = os.getenv("GROQ_API_KEY")
    groq_endpoint = os.getenv("GROQ_API_ENDPOINT", "https://api.groq.com/openai/v1")
    groq_model = os.getenv("GROQ_MODEL", "llama3-8b-8192")
    groq_rate_limit = _getenv_int("GROQ_RATE_LIMIT_PER_MINUTE", 20)
    groq_auth_header = os.getenv("GROQ_AUTH_HEADER", "Authorization")
    groq_auth_prefix = os.getenv("GROQ_AUTH_PREFIX", "Bearer ")
    providers.append(
        LLMProviderConfig(
            name="groq",
            api_key=groq_key,
            endpoint=groq_endpoint,
            model=groq_model,
            rate_limit_per_minute=groq_rate_limit,
            weight=_getenv_int("GROQ_WEIGHT", 1),
            enabled=bool(groq_key),
            auth_header=groq_auth_header,
            api_key_prefix=groq_auth_prefix,
        )
    )

    # Cloudflare (Workers + R2 model proxies)
    # Note: Cloudflare model endpoints are frequently structured under an 'accounts' path.
    # When you provide `CLOUDFLARE_ACCOUNT`, and you did not provide a full model endpoint,
    # we automatically construct a reasonable model-specific path using the base account path:
    #   <CLOUDFLARE_API_ENDPOINT>/<CLOUDFLARE_ACCOUNT>/workers/models/<CLOUDFLARE_MODEL>/responses
    # This reduces the need to manually pass a fully-formed endpoint and keeps
    # a consistent convention for Cloudflare Workers model responses.
    cf_key = os.getenv("CLOUDFLARE_API_KEY")
    cf_endpoint = os.getenv("CLOUDFLARE_API_ENDPOINT", "https://api.cloudflare.com/client/v4")
    cf_model = os.getenv("CLOUDFLARE_MODEL", "@cf/meta/llama-3.1-8b-instruct")
    cf_account = os.getenv("CLOUDFLARE_ACCOUNT", None)
    # If caller specified a Cloudflare account and the endpoint appears to be a base accounts path,
    # append the account and model-specific responses path to form the full model endpoint.
    try:
        if cf_account and cf_endpoint and cf_endpoint.rstrip("/").endswith("/client/v4"):
            # build Cloudflare Workers AI run route for the provided account and model
            cf_endpoint = f"{cf_endpoint.rstrip('/')}/accounts/{cf_account}/ai/run/{cf_model}"
    except Exception:
        # Keep the configured endpoint untouched on any construction error; let the user override
        pass
    cf_rate_limit = _getenv_int("CLOUDFLARE_RATE_LIMIT_PER_MINUTE", 20)
    cf_auth_header = os.getenv("CLOUDFLARE_AUTH_HEADER", "Authorization")
    cf_auth_prefix = os.getenv("CLOUDFLARE_AUTH_PREFIX", "Bearer ")
    providers.append(
        LLMProviderConfig(
            name="cloudflare",
            api_key=cf_key,
            endpoint=cf_endpoint,
            model=cf_model,
            rate_limit_per_minute=cf_rate_limit,
            weight=_getenv_int("CLOUDFLARE_WEIGHT", 1),
            enabled=bool(cf_key),
            auth_header=cf_auth_header,
            api_key_prefix=cf_auth_prefix,
        )
    )

    # Google AI Studio
    ga_key = os.getenv("GOOGLE_AI_API_KEY")
    ga_model = os.getenv("GOOGLE_MODEL", "gpt-4o-mini")
    ga_rate_limit = _getenv_int("GOOGLE_AI_RATE_LIMIT_PER_MINUTE", 20)
    ga_endpoint = os.getenv("GOOGLE_AI_API_ENDPOINT", "https://generative.googleapis.com/v1beta2/models")
    ga_auth_header = os.getenv("GOOGLE_AI_AUTH_HEADER", "Authorization")
    ga_auth_prefix = os.getenv("GOOGLE_AI_AUTH_PREFIX", "Bearer ")
    providers.append(
        LLMProviderConfig(
            name="google",
            api_key=ga_key,
            endpoint=ga_endpoint,
            model=ga_model,
            rate_limit_per_minute=ga_rate_limit,
            weight=_getenv_int("GOOGLE_WEIGHT", 1),
            enabled=bool(ga_key),
            auth_header=ga_auth_header,
            api_key_prefix=ga_auth_prefix,
        )
    )

    # Add a simple local/mock provider automatically enabled when no keys set.
    # Useful for testing without API creds.
    if not any(p.enabled for p in providers):
        providers.append(
            LLMProviderConfig(
                name="mock",
                api_key=None,
                endpoint=None,
                rate_limit_per_minute=1000,
                weight=1,
                enabled=True,
            )
        )

    return providers


def from_env() -> Config:
    """
    Build a Config object from the environment.

    Example environment variables:
      - DATABASE_PATH=simple_trader.db
      - TELEGRAM_BOT_TOKEN=...
      - TELEGRAM_CHAT_ID=...
      - NEWS_RSS_FEEDS=https://cointelegraph.com/rss,https://www.coindesk.com/arc/outboundfeeds/rss/
      - ALPHAVANTAGE_API_KEY=yourkey
      - GROQ_API_KEY=...
      - CLOUDFLARE_API_KEY=...
      - GOOGLE_AI_API_KEY=...
      - ACCOUNT_BALANCE_USD=200000
      - RISK_PER_TRADE_PCT=0.01
    """
    # Basic config
    database_path = os.getenv("DATABASE_PATH", "simple_trader.db")
    tenant_id = os.getenv("TENANT_ID", "default")
    log_level = os.getenv("LOG_LEVEL", "INFO")
    timezone = os.getenv("TIMEZONE", "UTC")

    # Account & risk
    account_balance_usd = _getenv_float("ACCOUNT_BALANCE_USD", 100000.0)
    risk_per_trade_pct = _getenv_float("RISK_PER_TRADE_PCT", 0.005)  # default to conservative 0.5%
    min_rr = _getenv_float("MIN_RISK_REWARD_RATIO", 3.0)
    stop_loss_slippage_pct = _getenv_float("STOP_LOSS_SLIPPAGE_PCT", 0.001)

    max_leverage_crypto = _getenv_int("MAX_LEVERAGE_CRYPTO", 10)
    max_leverage_forex = _getenv_int("MAX_LEVERAGE_FOREX", 30)

    # Timeframes & trade duration
    timeframe_hours = _getenv_int("TIMEFRAME_HOURS", 2)
    max_trade_duration_hours = _getenv_int("MAX_TRADE_DURATION_HOURS", 24)
    news_max_age_hours = _getenv_int("NEWS_MAX_AGE_HOURS", 24)

    # News sources
    rss_feeds = _parse_csv(os.getenv("NEWS_RSS_FEEDS"), get_default_rss_feeds())
    news_api_key = os.getenv("NEWS_API_KEY")
    cryptonews_api_key = os.getenv("CRYPTONEWS_API_KEY")

    # Market data (forex/alpha)
    alphavantage_key = os.getenv("ALPHAVANTAGE_API_KEY")

    # LLMs
    llm_providers = _default_llm_provider_envs()
    llm_global_concurrency = _getenv_int("LLM_GLOBAL_CONCURRENCY", 5)
    llm_min_confidence = _getenv_float("LLM_MIN_CONFIDENCE", 0.6)

    # Signals / telegram
    telegram_bot_token = os.getenv("TELEGRAM_BOT_TOKEN")
    telegram_chat_id = os.getenv("TELEGRAM_CHAT_ID")

    # Operational knobs
    open_positions_limit = _getenv_int("OPEN_POSITIONS_LIMIT", 10)
    backtest_mode = _getenv_bool("BACKTEST_MODE", False)
    enable_telemetry = _getenv_bool("ENABLE_TELEMETRY", True)

    min_pattern_confidence = _getenv_float("MIN_PATTERN_CONFIDENCE", 0.6)

    # Session & currency mapping
    session_map = _session_defaults()
    currency_session_map = _currency_to_sessions_defaults()

    # === Full Investing System (Secret Formula) env overrides ===
    core_bucket_target_pct = _getenv_float("CORE_BUCKET_TARGET_PCT", 0.55)
    alpha_bucket_target_pct = _getenv_float("ALPHA_BUCKET_TARGET_PCT", 0.45)

    max_exposure_gold = _getenv_float("MAX_EXPOSURE_GOLD", 0.40)
    max_exposure_silver = _getenv_float("MAX_EXPOSURE_SILVER", 0.15)
    max_exposure_crypto = _getenv_float("MAX_EXPOSURE_CRYPTO", 0.25)
    max_exposure_forex = _getenv_float("MAX_EXPOSURE_FOREX", 0.20)
    max_exposure_oil = _getenv_float("MAX_EXPOSURE_OIL", 0.15)

    max_book_risk_pct = _getenv_float("MAX_BOOK_RISK_PCT", 0.08)
    max_daily_loss_pct = _getenv_float("MAX_DAILY_LOSS_PCT", 0.03)
    max_drawdown_pause_pct = _getenv_float("MAX_DRAWDOWN_PAUSE_PCT", 0.08)
    circuit_breaker_cooldown_hours = _getenv_int("CIRCUIT_BREAKER_COOLDOWN_HOURS", 24)

    enable_auto_hedge = _getenv_bool("ENABLE_AUTO_HEDGE", True)
    hedge_ratio = _getenv_float("HEDGE_RATIO", 0.4)
    hedge_rebalance_threshold = _getenv_float("HEDGE_REBALANCE_THRESHOLD", 0.15)

    use_vol_targeting = _getenv_bool("USE_VOL_TARGETING", True)
    target_vol_pct = _getenv_float("TARGET_VOL_PCT", 0.012)
    fractional_kelly_factor = _getenv_float("FRACTIONAL_KELLY_FACTOR", 0.25)

    news_impact_boost_alpha = _getenv_float("NEWS_IMPACT_BOOST_ALPHA", 1.5)
    gold_hedge_on_risk_off = _getenv_bool("GOLD_HEDGE_ON_RISK_OFF", True)

    cfg = Config(
        database_path=database_path,
        tenant_id=tenant_id,
        log_level=log_level,
        timezone=timezone,
        account_balance_usd=account_balance_usd,
        risk_per_trade_pct=risk_per_trade_pct,
        min_risk_reward_ratio=min_rr,
        stop_loss_slippage_pct=stop_loss_slippage_pct,
        max_leverage_crypto=max_leverage_crypto,
        max_leverage_forex=max_leverage_forex,
        timeframe_hours=timeframe_hours,
        max_trade_duration_hours=max_trade_duration_hours,
        news_max_age_hours=news_max_age_hours,
        news_rss_feeds=rss_feeds,
        news_api_key=news_api_key,
        cryptonews_api_key=cryptonews_api_key,
        alphavantage_api_key=alphavantage_key,
        llm_providers=llm_providers,
        llm_global_concurrency=llm_global_concurrency,
        llm_min_confidence=llm_min_confidence,
        telegram_bot_token=telegram_bot_token,
        telegram_chat_id=telegram_chat_id,
        open_positions_limit=open_positions_limit,
        backtest_mode=backtest_mode,
        enable_telemetry=enable_telemetry,
        min_pattern_confidence=min_pattern_confidence,
        session_map=session_map,
        currency_session_map=currency_session_map,

        # Full system
        core_bucket_target_pct=core_bucket_target_pct,
        alpha_bucket_target_pct=alpha_bucket_target_pct,
        max_exposure_gold=max_exposure_gold,
        max_exposure_silver=max_exposure_silver,
        max_exposure_crypto=max_exposure_crypto,
        max_exposure_forex=max_exposure_forex,
        max_exposure_oil=max_exposure_oil,
        max_book_risk_pct=max_book_risk_pct,
        max_daily_loss_pct=max_daily_loss_pct,
        max_drawdown_pause_pct=max_drawdown_pause_pct,
        circuit_breaker_cooldown_hours=circuit_breaker_cooldown_hours,
        enable_auto_hedge=enable_auto_hedge,
        hedge_ratio=hedge_ratio,
        hedge_rebalance_threshold=hedge_rebalance_threshold,
        use_vol_targeting=use_vol_targeting,
        target_vol_pct=target_vol_pct,
        fractional_kelly_factor=fractional_kelly_factor,
        news_impact_boost_alpha=news_impact_boost_alpha,
        gold_hedge_on_risk_off=gold_hedge_on_risk_off,
    )

    # Validate sanity of config
    cfg.validate()

    # Optional debugging/logging to let the dev know the chosen config.
    # Avoid printing secrets
    logger = logging.getLogger("simple_trader.config")
    logger.setLevel(getattr(logging, cfg.log_level.upper(), logging.INFO))
    logger.debug("Loaded configuration from environment (with secrets masked):")
    logger.debug("database_path=%s", cfg.database_path)
    logger.debug("tenant_id=%s", cfg.tenant_id)
    logger.debug("queue capacity per minute (LLM): %s", cfg.token_bucket_capacity_per_minute())
    logger.debug("number of LLM providers enabled: %s", len(cfg.get_llm_providers_enabled()))
    logger.debug("rss_feeds_count=%s", len(cfg.news_rss_feeds))
    logger.debug("backtest_mode=%s; telemetry=%s", cfg.backtest_mode, cfg.enable_telemetry)

    # Remove or mask any secrets from printed config left in memory when using debug logging.
    # A production app would be careful about leaking configuration into logs.
    return cfg


# For convenience if modules import the config at module import time:
CONFIG = from_env()
load_config = from_env
