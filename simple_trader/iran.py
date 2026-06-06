"""
Iran Market Module - Dedicated Algorithms, News, and Data for Iranian Bourse (TSE/IFB)

This module provides **separated** logic for the Iranian market because:
- It is largely decoupled from global financial systems (sanctions, rial economy, local liquidity).
- Local dynamics often diverge: market can rally on domestic policy, oil production resilience, or Codal-driven corporate news even during international conflicts/sanctions.
- News sources and "world view" are different: heavy focus on Codal.ir disclosures (like local EDGAR + insider reports), Eghtesad News, domestic inflation, central bank policy, energy exports despite pressure, and sharia-compliant instruments.
- Different risk perception: High inflation makes fixed income/treasury bonds and funds act as "preservation" tools locally; stocks/ETFs are high-beta on local macro + global oil.

Design:
- Separate news input processor (enhances the general fetch_iran_news).
- Iran-specific indicators and algorithms (Codal event strength, sanctions-resilience scoring, local oil beta).
- Chart/data sources (stubs + hooks for tsetmc.com, ifb.ir, codal.ir public data).
- Dedicated heuristic/algorithm layer that can run in parallel or feed the main system.
- Fully integrated with the unified portfolio (Iran assets go into ALPHA or CORE buckets as configured), but with its own "secret sauce" rules.

Usage:
- Called from news_fetcher for dedicated Iran stream.
- HeuristicAnalyzer can use IranHeuristic for IRAN_* assets.
- MarketDataClient dispatches to Iran-specific fetchers.
- Signals created with Iran-specific decision_audit fields.

Keep everything free/public where possible (RSS, Codal search pages, public bourse data).
"""

from __future__ import annotations

import logging
import re
from datetime import datetime, timezone
from typing import Dict, List, Optional

import requests

from .config import CONFIG, Config
from .db import NewsItem, get_default_db, now_ts

logger = logging.getLogger("simple_trader.iran")
logger.addHandler(logging.NullHandler())

# Iranian-specific RSS and public sources (free)
IRAN_PRIMARY_RSS = [
    "https://www.eghtesadnews.com/rss",           # Main economic + bourse
    "https://www.eghtesadnews.com/category/bourse/rss",  # Dedicated bourse section
    # Codal coverage often appears in general economic RSS or company news sections.
    # For deeper Codal, one can later add a lightweight scraper for https://www.codal.ir/ latest reports.
]

# Strong Iran-specific keywords (Persian + English) for Codal, local macro, sanctions-resilience
IRAN_STRONG_KEYWORDS = [
    "کدال", "codal", "افشا", "گزارش", "مجمع", "سود", "زیان",
    "اوراق خزانه", "سخاب", "خزانه اسلامی", "درآمد ثابت", "صندوق etf",
    "تورم", "نرخ بهره", "ریال", "دلار آزاد", "تحریم", "صادرات نفت",
    "بورس تهران", "فرابورس", "ifb", "tse", "tsetmc"
]

class IranNewsProcessor:
    """
    Dedicated news input processor for Iranian market.
    Separate from global RSS/whale/politics to capture the "different world view".
    - Prioritizes Codal filings (major corporate events, financials - local "disclosure alpha").
    - Eghtesad News for policy, oil resilience, inflation context.
    - Maps to IRAN_* asset classes with local flavor (e.g., Codal-heavy = high impact for stocks/ETFs).
    """

    def __init__(self, config: Optional[Config] = None, db=None):
        self.config = config or CONFIG
        self.db = db or get_default_db(self.config.database_path, tenant_id=self.config.tenant_id)
        self.session = requests.Session()
        self.session.headers.update({"User-Agent": "SimpleTrader-Iran/1.0 (internal)"})

    def fetch_and_process(self, lookback_hours: int = 48) -> List[NewsItem]:
        """
        Fetch dedicated Iranian sources.
        Returns list of NewsItem ready for insertion (caller handles dedup).
        Emphasizes local perspective: Codal as primary signal source (like politician disclosures but for companies + gov influence).
        """
        items: List[NewsItem] = []
        now = now_ts()
        cutoff = now - (lookback_hours * 3600)

        feeds = self.config.iran_rss_feeds or IRAN_PRIMARY_RSS
        keywords = [k.lower() for k in self.config.iran_keywords] + IRAN_STRONG_KEYWORDS

        for feed_url in feeds:
            try:
                resp = self.session.get(feed_url, timeout=15)
                if resp.status_code != 200:
                    continue

                # Parse (reuse basic parser or feedparser)
                try:
                    import feedparser
                    feed = feedparser.parse(resp.text)
                    entries = feed.entries[:20]
                except Exception:
                    from .news_fetcher import NewsFetcher  # fallback
                    entries = NewsFetcher(self.config, self.db)._basic_rss_parse(resp.text)

                for entry in entries:
                    title = entry.get("title", "") or getattr(entry, "title", "")
                    link = entry.get("link", "") or getattr(entry, "link", "")
                    summary = entry.get("summary", "") or getattr(entry, "summary", "") or title
                    pub_ts = now  # rough; enhance with date parsing if needed

                    if pub_ts < cutoff:
                        continue

                    text = (title + " " + summary).lower()
                    if not any(kw in text for kw in keywords):
                        continue

                    # Iran-specific asset mapping (local view)
                    asset = self._map_iran_asset(text, title)

                    # Local "impact" flavor: Codal filings = high-alpha disclosure events
                    is_codal = "کدال" in text or "codal" in text or "افشا" in text or "گزارش" in text
                    content = summary[:300]
                    if is_codal:
                        content += " (Codal filing - primary local disclosure source, high signal for IRAN assets despite global events)"

                    item = NewsItem(
                        provider="iran_codal" if is_codal else "iran_bourse",
                        url=link,
                        title=f"IRAN: {title[:75]}",
                        content=content + " (Iranian perspective via Eghtesad/Codal - decoupled from global markets)",
                        published_at=pub_ts,
                        asset=asset,
                        raw_json={
                            "source": feed_url,
                            "is_codal": is_codal,
                            "local_keywords": [k for k in keywords if k in text],
                            "iran_specific_note": "Market may react positively to domestic resilience even during sanctions/war news."
                        },
                        fetched_at=now,
                    )
                    items.append(item)
                    logger.info("Iran dedicated signal: %s -> %s (codal=%s)", title[:45], asset, is_codal)

            except Exception as e:
                logger.debug("Iran news source error %s: %s (graceful, free source)", feed_url, e)

        return items

    def _map_iran_asset(self, text: str, title: str) -> str:
        text_l = text.lower()
        title_l = title.lower()

        if any(k in text_l for k in ["اوراق خزانه", "سخاب", "خزانه اسلامی", "treasury"]):
            return "IRAN_TREASURY"
        if any(k in text_l for k in ["صندوق درآمد ثابت", "درآمد ثابت", "fixed income fund"]):
            return "IRAN_FIXED_INCOME"
        if any(k in text_l for k in ["اوراق", "bond"]):
            return "IRAN_BOND"
        if "etf" in text_l or "صندوق etf" in text_l:
            return "IRAN_ETF"
        # Default to stock for bourse/سهام/فولاد etc.
        return "IRAN_STOCK"


class IranIndicators:
    """
    Iran-specific indicators and algorithms.
    Separate from global technicals because local market has different drivers:
    - Codal event strength (filing surprise = alpha)
    - Oil export resilience despite sanctions
    - Local inflation / rial pressure
    - "Sanctions resilience" score (positive reaction to bad global news sometimes)
    """

    @staticmethod
    def codal_event_strength(text: str, title: str) -> float:
        """0-1 score for how 'surprising' or high-impact a Codal filing/news is."""
        text_l = (text + " " + title).lower()
        score = 0.0
        if "کدال" in text_l or "codal" in text_l:
            score += 0.4
        if any(k in text_l for k in ["افشا", "گزارش", "سود", "زیان", "مجمع"]):
            score += 0.3
        if any(k in text_l for k in ["تورم", "تحریم", "صادرات"]):
            score += 0.2  # local macro overlay
        return min(1.0, score)

    @staticmethod
    def sanctions_resilience_score(news_text: str) -> float:
        """
        Positive score when Iran market/news shows resilience narrative
        (e.g. "despite sanctions, exports up", "local production strong").
        This is the key "different world view" algorithm.
        """
        text = news_text.lower()
        positive_local = ["با وجود تحریم", "صادرات افزایش", "تولید داخلی", "مقاومت", "رشد بورس"]
        negative_global = ["تحریم جدید", "جنگ", "کاهش صادرات"]

        pos = sum(1 for p in positive_local if p in text)
        neg = sum(1 for n in negative_global if n in text)

        if pos > neg:
            return min(0.8, 0.2 + (pos - neg) * 0.15)
        return 0.0

    @staticmethod
    def iran_oil_beta(text: str) -> float:
        """Local oil sensitivity (Iran is producer but sanctioned - nuanced beta)."""
        text_l = text.lower()
        if "نفت" in text_l or "oil" in text_l or "صادرات" in text_l:
            return 0.7 if "افزایش" in text_l or "رشد" in text_l else 0.4
        return 0.2


class IranMarketData:
    """
    Dedicated chart / OHLC sources for Iran.
    Separate because global providers (yfinance, CoinGecko) have almost nothing usable.
    Primary free sources: tsetmc.com, ifb.ir, codal.ir (for fundamentals more than price).
    """

    def __init__(self):
        self.session = requests.Session()
        self.session.headers.update({"User-Agent": "SimpleTrader-IranData/1.0"})

    def get_ohlc(self, symbol: str, lookback_days: int = 30) -> Optional[dict]:
        """
        Stub for Iranian price data.
        In practice:
        - Use public endpoints from tsetmc.com or ifb.ir (may require parsing tables/JS).
        - For bonds/treasury: central bank or bourse yield data.
        - Fallback: return None and let the system run pure news/Codal algorithm.
        """
        # Placeholder - real implementation would scrape or call public JSON if available.
        logger.info("IranMarketData: requesting OHLC for %s (free sources limited)", symbol)
        # Example future hook:
        # try:
        #     # url = f"https://tsetmc.com/..." or codal search
        #     pass
        # except: pass
        return None  # Signals will be news-driven with Iran-specific algorithms

    def get_codal_recent(self, keyword: str = "") -> List[dict]:
        """Lightweight public Codal search hook (for disclosure alpha)."""
        # In real use, one can implement requests to codal.ir search results.
        return []


# Convenience entry points used by the rest of the system
def fetch_iran_dedicated_news(config: Optional[Config] = None) -> List[NewsItem]:
    proc = IranNewsProcessor(config)
    return proc.fetch_and_process()


def get_iran_indicator_features(text: str, title: str = "") -> Dict[str, float]:
    """Returns Iran-specific features for scorer / decision_audit."""
    return {
        "iran_codal_strength": IranIndicators.codal_event_strength(text, title),
        "iran_sanctions_resilience": IranIndicators.sanctions_resilience_score(text),
        "iran_oil_beta": IranIndicators.iran_oil_beta(text),
    }


def get_iran_market_data(symbol: str):
    client = IranMarketData()
    return client.get_ohlc(symbol)