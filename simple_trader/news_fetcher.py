# Simple-Trader/simple_trader/news_fetcher.py
# -*- coding: utf-8 -*-
"""
News fetching module for Simple-Trader.

Responsibilities:
- Fetch news from configured RSS feeds and (optionally) NewsAPI (or other HTTP JSON news APIs).
- Parse and normalize news articles into `NewsItem` dataclass instances.
- Try to heuristically extract an associated asset or market symbol from the text (cryptocurrency or forex pairs).
- Insert new news rows into the SQLite database, avoiding duplicates via a hash.
- Provide an API that other modules (LLM pipeline / signal generator) can use to retrieve
  fresh unprocessed news items.

Key notes:
- The code uses `feedparser` for RSS feeds and `requests` for HTTP JSON APIs.
  If `feedparser` isn't installed, the RSS fetcher will raise a helpful error.
- The heuristics to extract asset symbols are intentionally conservative — it's much more
  robust to pass the news content to the LLM to confirm which asset is relevant.
- This module is designed to be safe to call periodically and idempotent (will not re-insert duplicates).
"""

from __future__ import annotations

import hashlib
import json
import logging
import re
import time
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Dict, Iterable, List, Optional, Tuple

import requests

try:
    import feedparser  # type: ignore
except Exception:  # pragma: no cover - fallback for minimal environments
    feedparser = None  # type: ignore

from simple_trader.config import Config, CONFIG
from simple_trader.db import Database, NewsItem, get_default_db, now_ts
from simple_trader.knowledge_base import ASSET_KNOWLEDGE  # for context

logger = logging.getLogger("simple_trader.news_fetcher")
logger.addHandler(logging.NullHandler())

# Simple mapping to help detect common asset names -> ticker symbols for cryptos
COMMON_CRYPTO_NAMES = {
    "bitcoin": "BTC",
    "btc": "BTC",
    "ethereum": "ETH",
    "eth": "ETH",
    "ripple": "XRP",
    "xrp": "XRP",
    "cardano": "ADA",
    "ada": "ADA",
    "solana": "SOL",
    "sol": "SOL",
    "binance": "BNB",
    "bnb": "BNB",
    "litecoin": "LTC",
    "ltc": "LTC",
    "dogecoin": "DOGE",
    "doge": "DOGE",
}


# regex to detect forex like EUR/USD, EURUSD, EUR USD
_FOREX_RE = re.compile(r"\b([A-Z]{3})[ /]?([A-Z]{3})\b")

# regex to detect crypto tickers with a $ prefix (like $BTC, $ETH)
_CRYPTO_TICKER_DOLLAR_RE = re.compile(r"\$(?P<ticker>[A-Za-z0-9]{2,5})\b")

# Generic ticker-like pattern (e.g. BTC, ETH) — we'll require at least one such mention near "coin" or currency words
_GENERIC_TICKER_RE = re.compile(r"\b([A-Z]{2,5})\b")

# For normalizing dates returned by different APIs
ISO_DATE_RE = re.compile(
    r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})?$"
)


@dataclass
class FetchResult:
    provider: str
    news_item: NewsItem
    inserted_id: int


class NewsFetcher:
    """
    Orchestrate fetching news from different providers and store them into the database.
    """

    def __init__(self, config: Optional[Config] = None, db: Optional[Database] = None):
        self.config = config or CONFIG
        self.db = db or get_default_db(
            self.config.database_path, tenant_id=self.config.tenant_id
        )
        self.session = requests.Session()
        # Standard HTTP headers for our requests
        self.session.headers.update(
            {
                "User-Agent": "SimpleTrader/1.0 (+https://example.com)",
            }
        )

    # ----------------------
    # Public fetch methods
    # ----------------------
    def fetch_all(self, rss_feeds: Optional[Iterable[str]] = None) -> List[FetchResult]:
        """
        Fetch news from the configured sources (RSS + News API if enabled).
        Also fetches free/public whale on-chain moves and politician/policy signals.
        Returns a list of FetchResult for each inserted or deduplicated NewsItem.
        """
        results: List[FetchResult] = []

        # RSS feeds
        feeds = list(rss_feeds or self.config.news_rss_feeds or [])
        if feeds:
            try:
                logger.debug("Fetching RSS feeds, count=%s", len(feeds))
                results.extend(self.fetch_rss_feeds(feeds))
            except Exception:
                logger.exception("Error while fetching RSS feeds")

        # NewsAPI (optional)
        if self.config.news_api_key:
            try:
                logger.debug("Fetching NewsAPI data")
                results.extend(self.fetch_newsapi())
            except Exception:
                logger.exception("Error while fetching from NewsAPI")

        # Free whale + politics (on-chain public for BTC/ETH whales, RSS keywords + disclosures for politicians)
        # No extra keys; graceful if rate limited or no addresses configured.
        try:
            special = self.fetch_whale_and_politics()
            results.extend(special)
            if special:
                logger.debug("Added %d free whale/politics signals", len(special))
        except Exception:
            logger.debug("Whale/politics special fetch skipped (free/public sources)")

        # Dedicated public politician disclosures (lagged but official/public filings + news)
        try:
            from .politician_disclosures import fetch_and_process_politician_disclosures
            pol_items = fetch_and_process_politician_disclosures(self.config, self.db)
            if pol_items:
                logger.info("Added %d public politician disclosure signals (free, official data)", len(pol_items))
                # Convert to FetchResult for consistency (they are already inserted)
                for item in pol_items:
                    results.append(FetchResult("politics_disclosure", item, 0))  # id not critical here
        except Exception as e:
            logger.debug("Politician disclosures fetch skipped (public sources): %s", e)

        # NEW: Iranian Bourse support - SEPARATE algorithms & news input
        # Uses dedicated IranNewsProcessor from iran.py for local "different world view"
        # (Codal as primary disclosure alpha, sanctions-resilience narratives, oil beta despite isolation).
        # Still integrated into unified portfolio but processed with Iran-specific logic.
        try:
            from .iran import fetch_iran_dedicated_news
            iran_items = fetch_iran_dedicated_news(self.config)
            for item in iran_items:
                inserted = self._insert_news_item(item)
                results.append(FetchResult("iran_dedicated", item, inserted))
            if iran_items:
                logger.info("Added %d dedicated Iranian signals (separate Iran module: Codal/Eghtesad local view)", len(iran_items))
        except Exception as e:
            logger.debug("Dedicated Iran news skipped (free/public, separate module): %s", e)

        logger.debug("Total news fetched (new inserts / deduped): %s", len(results))
        return results

    # ----------------------
    # Fetchers
    # ----------------------
    def fetch_rss_feeds(self, rss_urls: Iterable[str]) -> List[FetchResult]:
        """
        Parse a list of RSS URLs and insert distinct news items into DB.
        """
        if feedparser is None:
            raise RuntimeError(
                "feedparser is required to fetch RSS feeds. Install feedparser to enable RSS support."
            )

        inserted: List[FetchResult] = []

        for url in rss_urls:
            try:
                logger.debug("Parsing RSS feed: %s", url)
                feed = feedparser.parse(url)
                provider_name = self._derive_provider_from_feed(url, feed)

                if not feed or not getattr(feed, "entries", None):
                    logger.debug("RSS feed returned no entries (or failed to parse): %s", url)
                    continue

                for entry in feed.entries:
                    try:
                        item = self._parse_rss_entry(entry, url, provider_name)
                        if self._is_news_too_old(item):
                            logger.debug("Skipping too-old news: %s", item.title)
                            continue
                        inserted_id = self._insert_news_item(item)
                        # store inserted id and provider
                        inserted.append(FetchResult(provider=provider_name, news_item=item, inserted_id=inserted_id))
                    except Exception:
                        logger.exception("Failed to process RSS entry for feed %s", url)
                        continue
            except Exception:
                logger.exception("Failed to fetch or parse RSS feed: %s", url)
                continue

        return inserted

    def fetch_newsapi(self) -> List[FetchResult]:
        """
        Fetch articles via NewsAPI.org (as an example JSON API).
        The query is intentionally broad and seeks financial/crypto/forex keywords; in a production
        set-up you'd want to fine-tune queries per asset or maintain a keyword list.
        """
        if not self.config.news_api_key:
            logger.debug("No NewsAPI key available; skipping NewsAPI fetch")
            return []

        api_key = self.config.news_api_key
        endpoint = "https://newsapi.org/v2/everything"

        # Build query words — a conservative default set. Users should customize this via separate integration.
        keywords = [
            "bitcoin OR btc",
            "ethereum OR eth",
            "cryptocurrency OR crypto",
            "forex OR \"EUR/USD\" OR EURUSD OR \"GBP/USD\" OR GBPUSD",
            "macro OR inflation OR fed OR rate hike",
            "bitcoin price",
        ]
        query = " OR ".join(keywords)
        page_size = 50
        page = 1
        inserted: List[FetchResult] = []

        # Heuristic limit to avoid too many pages
        max_pages = 2

        while True:
            params = {
                "q": query,
                "language": "en",
                "pageSize": page_size,
                "page": page,
                "apiKey": api_key,
                "sortBy": "publishedAt",
            }

            resp = self.session.get(endpoint, params=params, timeout=20)
            if resp.status_code != 200:
                logger.warning("NewsAPI returned non-200 status: %s - %s", resp.status_code, resp.text)
                break

            payload = resp.json()
            if payload.get("status") != "ok":
                logger.warning("NewsAPI error response: %s", payload)
                break

            articles = payload.get("articles", [])
            if not articles:
                break

            for a in articles:
                try:
                    item = self._parse_newsapi_article(a)
                    if self._is_news_too_old(item):
                        logger.debug("Skipping too-old NewsAPI article: %s", item.title)
                        continue
                    inserted_id = self._insert_news_item(item)
                    inserted.append(
                        FetchResult(provider="newsapi", news_item=item, inserted_id=inserted_id)
                    )
                except Exception:
                    logger.exception("Failed to parse NewsAPI article")
                    continue

            # Stop if we reached the max effective pages or no more results
            page += 1
            if page > max_pages or page * page_size >= payload.get("totalResults", 0):
                break

        return inserted

    # ----------------------
    # Parsing & normalization helpers
    # ----------------------
    def _derive_provider_from_feed(self, feed_url: str, feed_obj) -> str:
        """
        Heuristic provider name: try to extract domain or feed title.
        """
        # prefer feed title when available
        try:
            title = getattr(feed_obj.feed, "title", None)
            if title:
                return f"rss:{re.sub(r'\\s+', '_', title.strip().lower())}"
        except Exception:
            pass
        # fallback: use domain
        try:
            from urllib.parse import urlparse

            domain = urlparse(feed_url).netloc
            return f"rss:{domain}"
        except Exception:
            return "rss:unknown"

    def _parse_rss_entry(self, entry: Dict, feed_url: str, provider_name: str) -> NewsItem:
        """
        Normalize `feedparser` entry to NewsItem.
        """
        title = entry.get("title") or entry.get("headline") or ""
        link = entry.get("link") or entry.get("id")
        # feedparser often has `published_parsed` as struct_time; attempt to use it
        published_ts = None
        if getattr(entry, "published_parsed", None):
            try:
                published_ts = int(time.mktime(entry.published_parsed))
            except Exception:
                published_ts = None
        elif entry.get("published"):
            # Try ISO-like parse; fallback to None
            try:
                pub = entry.get("published")
                # some feeds put timezone info
                published_ts = self._parse_date_to_ts(pub)
            except Exception:
                published_ts = None

        content = None
        # feedparser entries sometimes place content in `summary`, `description` or `content[0].value`
        if entry.get("summary"):
            content = entry.get("summary")
        elif entry.get("description"):
            content = entry.get("description")
        elif entry.get("content") and isinstance(entry.get("content"), list) and entry.get("content"):
            try:
                content = entry.get("content")[0].get("value")
            except Exception:
                content = None

        if not content:
            # final fallback for content: title and link
            content = title or ""

        # Detect asset from content and title heuristically
        asset_guess = self.extract_asset_from_text(title + "\n" + (content or ""))

        # Build a raw_json for provenance
        raw_json = {
            "feed_url": feed_url,
            "entry": {
                "title": title,
                "link": link,
                "published": published_ts,
            },
        }

        item = NewsItem(
            provider=provider_name,
            url=link,
            title=title,
            content=content,
            published_at=published_ts,
            asset=asset_guess,
            raw_json=raw_json,
            fetched_at=now_ts(),
        )
        return item

    def _parse_newsapi_article(self, a: Dict) -> NewsItem:
        """
        Normalize an article from NewsAPI into NewsItem.
        NewsAPI 'publishedAt' is ISO 8601 typically.
        """
        title = a.get("title") or ""
        link = a.get("url")
        content = a.get("description") or a.get("content") or title
        published_at_ts = None
        if a.get("publishedAt"):
            published_at_ts = self._parse_date_to_ts(a.get("publishedAt"))
        asset_guess = self.extract_asset_from_text(title + "\n" + (content or ""))

        raw_json = {"source": a.get("source", {}), "raw": a}

        item = NewsItem(
            provider="newsapi",
            url=link,
            title=title,
            content=content,
            published_at=published_at_ts,
            asset=asset_guess,
            raw_json=raw_json,
            fetched_at=now_ts(),
        )
        return item

    def _parse_date_to_ts(self, date_text: str) -> Optional[int]:
        """
        Try to parse several common date formats to a UTC epoch timestamp.
        """
        if date_text is None:
            return None
        date_text_str = str(date_text).strip()
        try:
            # If it matches ISO format, use fromisoformat (py3.7+)
            if ISO_DATE_RE.match(date_text_str):
                # datetime.fromisoformat doesn't parse 'Z' in Python < 3.11; handle 'Z'
                if date_text_str.endswith("Z"):
                    date_text_str = date_text_str.replace("Z", "+00:00")
                dt = datetime.fromisoformat(date_text_str)
                if dt.tzinfo is None:
                    dt = dt.replace(tzinfo=timezone.utc)
                return int(dt.astimezone(timezone.utc).timestamp())
        except Exception:
            pass

        # Fallback for RSS/other structured times using feedparser's parsing
        try:
            import email.utils

            parsed = email.utils.parsedate_to_datetime(date_text_str)
            if parsed:
                return int(parsed.astimezone(timezone.utc).timestamp())
        except Exception:
            pass

        # Last resort: return current time
        return int(now_ts())

    # ----------------------
    # Identification heuristics
    # ----------------------
    def extract_asset_from_text(self, text: str) -> Optional[str]:
        """
        Heuristic attempt to find a relevant asset symbol from a news headline or body.
        This supports basic crypto ticker detection and forex pairs in a conservative way.
        We prefer returning 1) a crypto ticker (BTC, ETH), 2) cryptocurrency tickers with $ prefix,
        3) standard forex pairs like EURUSD or EUR/USD normalized to EURUSD.

        Returns normalized string or None.
        """
        if not text:
            return None

        t = text.lower()

        # Quick 'bitcoin' -> 'BTC' mapping
        for name, ticker in COMMON_CRYPTO_NAMES.items():
            if name in t:
                return ticker

        # $BTC style
        dollar_match = _CRYPTO_TICKER_DOLLAR_RE.search(text)
        if dollar_match:
            ticker = dollar_match.group("ticker").upper()
            # basic filter: avoid three-letter forex currency false positives like $USD mapped to USD,
            # prefer cryptos if ticker is not a fiat currency
            if ticker and ticker not in ("USD", "EUR", "GBP", "JPY", "AUD", "CAD", "CHF"):
                return ticker

        # Forex pattern like EUR/USD or EURUSD
        forex_match = _FOREX_RE.search(text.upper())
        if forex_match:
            a, b = forex_match.groups()
            # normalize to AAA BBB -> AAABBB
            pair = f"{a}{b}"
            return pair

        # Generic uppercase tickers: look for frequent crypto tickers or well known pairs
        # We avoid returning single letter uppercase tokens.
        tokens = set([m.group(1) for m in _GENERIC_TICKER_RE.finditer(text)])
        # Preference list: top cryptos / known currencies
        known_candidates = ["BTC", "ETH", "BNB", "SOL", "ADA", "XRP", "LTC", "DOGE", "DOT", "LINK"]
        for cand in known_candidates:
            if cand in tokens:
                return cand

        # fallback: None - LLM will be asked to confirm the exact asset
        return None

    # ----------------------
    # DB helpers & dedupe
    # ----------------------
    def _insert_news_item(self, item: NewsItem) -> int:
        """
        Insert a news item into DB while avoiding duplicates by hash. We compute a stable hash
        using URL + title + published_at (if present).
        The DB insert function also sets a hash if the input doesn’t include one.
        Returns the news ID (existing or newly created).
        """
        # Ensure we have a stable hash
        if not item.hash:
            base = (item.url or "") + "|" + (item.title or "") + "|" + str(item.published_at or "")
            item.hash = hashlib.sha256(base.encode("utf-8")).hexdigest()

        existing = self.db.get_news_by_hash(item.hash)
        if existing:
            logger.debug("News already exists in DB (hash found): %s", item.title)
            return int(existing["id"])

        # Insert the news row
        news_id = self.db.insert_news(item)
        logger.debug("Inserted news id=%s title=%s", news_id, item.title)
        return news_id

    def _is_news_too_old(self, item: NewsItem) -> bool:
        """
        Filter old news; config.news_max_age_hours defines how old news can be before we ignore them
        (relative to published_at if present or fetched_at).
        """
        max_age_seconds = int(self.config.news_max_age_hours * 3600)
        time_origin = item.published_at or item.fetched_at or now_ts()
        age = int(now_ts()) - int(time_origin)
        return age > max_age_seconds

    # ----------------------
    # Utility helpers
    # ----------------------
    def get_unprocessed_news(self, limit: int = 100) -> List[Dict]:
        """
        Return up to `limit` unprocessed news rows as dict-like objects.
        This function is a thin wrapper around the DB.
        """
        rows = self.db.get_unprocessed_news(limit=limit)
        parsed: List[Dict] = []
        for r in rows:
            parsed.append(dict(r))
        return parsed

    # --- New: Whale trades & Politician disclosures (free/public data, zero extra keys) ---
    def fetch_whale_and_politics(self) -> List[FetchResult]:
        """
        Fetch special 'whale' (on-chain large moves) and 'politics' (disclosures, announcements)
        as high-impact signals for CRYPTO (and correlated OIL/FOREX/GOLD).
        Designed for free operation:
        - Whale: Query public blockchain explorers (blockchain.info for BTC - no key).
        - ETH/other: Falls back to RSS/news if no free endpoint or rate limit.
        - Politics: Relies on existing RSS + keyword detection (disclosures are public but lagged).
        Results are turned into NewsItem with asset=CRYPTO or relevant, provider="whale" or "politics".
        Inserted via normal dedup path.
        """
        results: List[FetchResult] = []
        now = now_ts()

        # 1. Whale monitoring (configurable public addresses)
        whale_addrs = getattr(self.config, "whale_addresses", {}) or {}
        for chain, addrs in whale_addrs.items():
            for addr in addrs:
                try:
                    if chain.upper() == "BTC":
                        # Free public API, no key
                        url = f"https://blockchain.info/rawaddr/{addr}?limit=5"
                        r = self.session.get(url, timeout=15)
                        if r.status_code == 200:
                            data = r.json()
                            total_received = data.get("total_received", 0) / 1e8  # sat to BTC
                            txs = data.get("txs", [])
                            for tx in txs[:2]:  # recent
                                # Simple: if large value in or out
                                value = sum(o.get("value", 0) for o in tx.get("out", [])) / 1e8
                                if value >= self.config.whale_min_size_btc:
                                    title = f"WHALE: Large BTC move {value:.1f} BTC involving {addr[:8]}..."
                                    content = f"On-chain whale activity detected. Possible accumulation or distribution. Cross with volume and news."
                                    item = NewsItem(
                                        provider="whale_btc",
                                        url=f"https://blockchain.info/address/{addr}",
                                        title=title,
                                        content=content,
                                        published_at=now,
                                        asset="BTC",
                                        raw_json={"chain": "BTC", "value": value, "addr": addr},
                                        fetched_at=now,
                                    )
                                    inserted = self._insert_news_item(item)
                                    results.append(FetchResult("whale", item, inserted))
                                    logger.info("Whale BTC signal: %.1f BTC", value)
                    elif chain.upper() == "ETH":
                        # Public explorers (no key for basic; rate limited)
                        # Use blockscout public API as example (free tier-ish)
                        url = f"https://eth.blockscout.com/api/v2/addresses/{addr}/transactions?filter=from"
                        r = self.session.get(url, timeout=15)
                        if r.status_code == 200:
                            data = r.json()
                            items = data.get("items", [])[:1]
                            for tx in items:
                                value_eth = float(tx.get("value", "0")) / 1e18 if tx.get("value") else 0
                                if value_eth >= self.config.whale_min_size_eth:
                                    title = f"WHALE: Large ETH transfer {value_eth:.1f} from {addr[:8]}..."
                                    content = "On-chain whale move in ETH. Monitor for exchange deposit (sell pressure) or accumulation."
                                    item = NewsItem(
                                        provider="whale_eth",
                                        url=f"https://eth.blockscout.com/address/{addr}",
                                        title=title,
                                        content=content,
                                        published_at=now,
                                        asset="ETH",
                                        raw_json={"chain": "ETH", "value": value_eth, "addr": addr},
                                        fetched_at=now,
                                    )
                                    inserted = self._insert_news_item(item)
                                    results.append(FetchResult("whale", item, inserted))
                except Exception as e:
                    logger.debug("Whale monitor error for %s %s: %s (free endpoint rate limit or parse)", chain, addr, e)

        # 2. Politician / policy signals (public disclosures + announcements)
        # Since real-time disclosures are lagged (US 45 days), we lean on RSS (already fetched) + special keyword scan.
        # Here we can add a lightweight "recent disclosure" note if user configures, or just generate from knowledge.
        # For demo: scan recent unprocessed for politician keywords and re-tag as high impact.
        try:
            recent = self.get_unprocessed_news(limit=20)
            pol_keywords = [k.lower() for k in getattr(self.config, "monitor_politicians", [])]
            for row in recent:
                text = (row.get("title", "") + " " + row.get("content", "")).lower()
                if any(kw in text for kw in pol_keywords + ["pelosi", "trump", "congress", "senate", "disclosure", "wlfi"]):
                    asset = row.get("asset") or "CRYPTO"  # default to crypto for Trump-related
                    title = f"POLITICS: {row.get('title', 'Policy/Disclosure signal')}"
                    content = (row.get("content", "") + " Public politician or policy move - high headline impact for crypto/policy-sensitive assets. Cross with on-chain.")
                    item = NewsItem(
                        provider="politics",
                        url=row.get("url", ""),
                        title=title,
                        content=content,
                        published_at=row.get("published_at"),
                        asset=asset,
                        raw_json={"source": "disclosure_or_announcement", "keywords": [k for k in pol_keywords if k in text]},
                        fetched_at=now,
                    )
                    inserted = self._insert_news_item(item)
                    results.append(FetchResult("politics", item, inserted))
                    logger.info("Politics signal detected from RSS: %s", title[:60])
        except Exception as e:
            logger.debug("Politics scan error (uses existing RSS): %s", e)

        return results

    # --- NEW: Iranian Bourse (free/public: Eghtesad News RSS + Codal keyword coverage) ---
    def fetch_iran_news(self) -> List[FetchResult]:
        """
        Fetch Iranian market news from free/public sources:
        - Eghtesad News RSS (bourse, stocks, funds, policy, Codal reports coverage).
        - Keyword scan for Codal (کدال) filings, stocks (e.g. فولاد), ETFs, Islamic Treasury Bonds (اوراق خزانه اسلامی),
          fixed income funds (صندوق درآمد ثابت).
        Returns NewsItems mapped to IRAN_STOCK / IRAN_ETF / IRAN_BOND / IRAN_FIXED_INCOME / IRAN_TREASURY.
        No paid API. Persian + English keywords. Deduped via normal path.
        """
        results: List[FetchResult] = []
        now = now_ts()
        cfg = self.config

        feeds = getattr(cfg, "iran_rss_feeds", []) or []
        keywords = [k.lower() for k in getattr(cfg, "iran_keywords", [])]

        for feed_url in feeds:
            try:
                resp = self.session.get(feed_url, timeout=15)
                if resp.status_code != 200:
                    continue
                try:
                    import feedparser
                    feed = feedparser.parse(resp.text)
                    entries = feed.entries[:15]
                except Exception:
                    entries = self._basic_rss_parse(resp.text)  # reuse from whale/politics

                for entry in entries:
                    title = (entry.get("title") or getattr(entry, "title", "") or "").strip()
                    link = entry.get("link") or getattr(entry, "link", "")
                    summary = (entry.get("summary") or getattr(entry, "summary", "") or title).strip()
                    text = (title + " " + summary).lower()

                    if not any(kw in text for kw in keywords + ["بورس", "سهام", "اوراق", "صندوق", "کدال", "codal"]):
                        continue

                    # Map to Iranian asset class
                    asset = "IRAN_STOCK"
                    if any(k in text for k in ["اوراق", "خزانه", "treasury", "bond", "سخاب"]):
                        asset = "IRAN_TREASURY" if "خزانه" in text or "treasury" in text else "IRAN_BOND"
                    elif any(k in text for k in ["صندوق", "درآمد ثابت", "fixed income", "fund"]):
                        asset = "IRAN_FIXED_INCOME"
                    elif "etf" in text or "صندوق etf" in text:
                        asset = "IRAN_ETF"

                    item = NewsItem(
                        provider="iran_bourse",
                        url=link,
                        title=f"IRAN: {title[:70]}",
                        content=summary[:350] + " (Free public source: Eghtesad/Codal coverage)",
                        published_at=now,
                        asset=asset,
                        raw_json={"source": feed_url, "keywords_matched": [k for k in keywords if k in text]},
                        fetched_at=now,
                    )
                    inserted = self._insert_news_item(item)
                    results.append(FetchResult("iran", item, inserted))
                    logger.info("Iran Bourse signal: %s -> %s", title[:50], asset)
            except Exception as e:
                logger.debug("Iran RSS error for %s: %s (free source, graceful)", feed_url, e)

        # Bonus: scan recent unprocessed general news for Codal/Iran keywords (if other RSS already fetched)
        try:
            recent = self.get_unprocessed_news(limit=30)
            for row in recent:
                text = (row.get("title", "") + " " + row.get("content", "")).lower()
                if any(kw in text for kw in keywords + ["کدال", "codal", "بورس تهران", "اوراق خزانه"]):
                    asset = "IRAN_STOCK"
                    if any(k in text for k in ["اوراق", "خزانه"]):
                        asset = "IRAN_TREASURY"
                    elif "صندوق" in text and "درآمد" in text:
                        asset = "IRAN_FIXED_INCOME"
                    item = NewsItem(
                        provider="iran_codal",
                        url=row.get("url", ""),
                        title=f"IRAN/CODAL: {row.get('title', '')[:70]}",
                        content=(row.get("content", "") + " (Codal filing or Iran bourse news)"),
                        published_at=row.get("published_at"),
                        asset=asset,
                        raw_json={"source": "general_rss_codal_scan"},
                        fetched_at=now,
                    )
                    inserted = self._insert_news_item(item)
                    results.append(FetchResult("iran_codal", item, inserted))
        except Exception as e:
            logger.debug("Iran secondary Codal scan skipped: %s", e)

        return results


# Quick unit-like entrypoint to exercise the RSS / NewsAPI flows (intended for dev/testing)
def _run_demo():
    import logging

    logging.basicConfig(level=logging.DEBUG)
    cfg = CONFIG
    db = get_default_db(cfg.database_path, tenant_id=cfg.tenant_id)
    nf = NewsFetcher(cfg, db=db)

    logger.info("Fetching from RSS...")
    rss_results = nf.fetch_rss_feeds(cfg.news_rss_feeds[:5])
    logger.info("RSS fetch inserted %s items", len(rss_results))

    if cfg.news_api_key:
        logger.info("Fetching from NewsAPI...")
        api_results = nf.fetch_newsapi()
        logger.info("NewsAPI fetch inserted %s items", len(api_results))
    else:
        logger.info("No NewsAPI key configured; skipping NewsAPI fetch")

    # Print a couple of unprocessed items
    new_unprocessed = nf.get_unprocessed_news(limit=10)
    logger.info("Unprocessed news count: %s", len(new_unprocessed))
    for n in new_unprocessed[:5]:
        logger.info("news id=%s title=%s provider=%s", n["id"], n["title"], n["provider"])


if __name__ == "__main__":  # pragma: no cover - demo usage
    _run_demo()
