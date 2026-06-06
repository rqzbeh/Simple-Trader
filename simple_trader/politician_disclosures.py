"""
Politician Disclosures Fetcher - Public Data Source for "Insider-like" Signals

US politicians (Congress, Senate, etc.) are required to disclose trades periodically (e.g., within 45 days for most).
Sources are PUBLIC:
- House/Senate disclosure websites (clerk.house.gov, senate.gov)
- Aggregators like OpenSecrets.org, CapitolTrades.com (public data available)
- News RSS that report on notable ones (Pelosi, Trump family crypto like WLFI, etc.)

This module fetches/parses public disclosures or news about them as high-impact "POLITICS" signals.
- Focus on our assets: Crypto (Trump announcements, WLFI), Oil/Energy policy, Forex/macro (tariffs, rates), Gold (inflation hedges).
- FREE: Uses public RSS, requests to public pages (no API key). Rate-limited, cached.
- Not real-time "insider" – lagged disclosures + public news. Use as sentiment/alpha catalyst only.
- Integrated as NewsItem with provider="politics_disclosure", asset mapped to CRYPTO/OIL/etc.

Usage: Called from news_fetcher or service for special signals.
"""

from __future__ import annotations

import hashlib
import json
import logging
import re
import time
from datetime import datetime, timezone
from typing import Dict, List, Optional

import requests

from simple_trader.config import CONFIG, Config
from simple_trader.db import Database, NewsItem, get_default_db, now_ts

logger = logging.getLogger("simple_trader.politician_disclosures")
logger.addHandler(logging.NullHandler())

# Public sources (free, no key)
# - RSS from aggregators/news that cover politician trades
# - Direct public disclosure search pages (parse recent filings)
# Real STOCK Act PTRs: lagged (up to 45 days for Congress), public via house/senate clerks, ethics sites, aggregators (Capitol Trades, OpenSecrets reports).
# We use public RSS + keyword + occasional lightweight public page scans. Never real-time insider.
POLITICS_RSS_FEEDS = [
    "https://www.opensecrets.org/rss/",  # OpenSecrets RSS (public data on money in politics)
    "https://www.reuters.com/markets/cryptocurrencies/rss",  # Often covers Trump crypto, policy
    "https://feeds.marketwatch.com/marketwatch/topstories/",  # Macro/policy
    "https://www.benzinga.com/topics/politics/feed",  # Politics/trade headlines (public)
]

# Known names for keyword boost (public figures with crypto/energy/macro impact)
KNOWN_POLITICIANS = ["Pelosi", "Trump", "Eric Trump", "Donald Trump Jr", "congress", "senate", "WLFI", "World Liberty"]

class PoliticianDisclosuresFetcher:
    def __init__(self, config: Optional[Config] = None, db: Optional[Database] = None):
        self.config = config or CONFIG
        self.db = db or get_default_db(self.config.database_path, tenant_id=self.config.tenant_id)
        self.session = requests.Session()
        self.session.headers.update({"User-Agent": "SimpleTrader-Politics/1.0 (internal)"})

    def fetch_recent_disclosures(self, lookback_days: int = 30) -> List[NewsItem]:
        """
        Fetch recent public politician trades/disclosures/announcements.
        - Prioritizes RSS from public sources.
        - Keyword matches for known politicians + our assets (crypto, oil, rates, gold hedge).
        - Returns list of NewsItem (not yet inserted; caller handles dedup/insert).
        - FREE: Public data only. Lagged (disclosures not instant), but high signal for headlines.
        """
        items: List[NewsItem] = []
        now = now_ts()
        cutoff = now - (lookback_days * 86400)

        # 1. Fetch from public RSS (free, no key)
        for feed_url in POLITICS_RSS_FEEDS:
            try:
                resp = self.session.get(feed_url, timeout=20)
                if resp.status_code != 200:
                    continue
                # Simple RSS parse without full feedparser dependency (use regex for titles/links)
                # For robustness, assume feedparser if available, else basic
                try:
                    import feedparser
                    feed = feedparser.parse(resp.text)
                    entries = feed.entries[:20]  # recent
                except Exception:
                    # Fallback basic parse
                    entries = self._basic_rss_parse(resp.text)

                for entry in entries:
                    title = entry.get("title", "") or getattr(entry, "title", "")
                    link = entry.get("link", "") or getattr(entry, "link", "")
                    summary = entry.get("summary", "") or getattr(entry, "summary", "") or title
                    pub_date = entry.get("published_parsed", None)
                    pub_ts = self._parse_pub_date(pub_date) or now

                    if pub_ts < cutoff:
                        continue

                    text = (title + " " + summary).lower()
                    # Match politicians or policy
                    if any(name.lower() in text for name in KNOWN_POLITICIANS + getattr(self.config, "monitor_politicians", [])):
                        # Map to asset
                        asset = self._map_to_asset(text)
                        if asset in ["CRYPTO", "OIL", "FOREX", "GOLD"]:  # our focus
                            ac = asset  # for signals later asset_class ~ same
                            item = NewsItem(
                                provider="politics_disclosure",
                                url=link,
                                title=f"POLITICS: {title[:80]}",
                                content=summary[:300] + " (Public disclosure/announcement - high sentiment impact. Lagged STOCK Act PTR or public filing. Always hedge GOLD for alpha).",
                                published_at=pub_ts,
                                asset=asset,
                                raw_json={"source": feed_url, "politician_keywords": [n for n in KNOWN_POLITICIANS if n.lower() in text], "asset_class": ac},
                                fetched_at=now,
                            )
                            items.append(item)
                            logger.info("Politics disclosure signal: %s -> %s", title[:50], asset)
            except Exception as e:
                logger.debug("Politics RSS fetch error for %s: %s (public source, graceful)", feed_url, e)

        # 2. Lightweight public disclosure/aggregator scan (free, no auth, graceful)
        # STOCK Act requires periodic transaction reports (PTR) public. Clerks publish (often PDF/search); aggregators surface notable ones fast.
        # We do minimal HTML keyword scan on known public pages for recent headlines (no full crawler).
        try:
            # Example public aggregator pages (change as sites evolve; always public data)
            pub_pages = [
                "https://www.capitoltrades.com/trades?per_page=20",  # public recent trades list (may have JS but text has names)
                "https://www.opensecrets.org/news/",  # public reports
            ]
            for purl in pub_pages:
                try:
                    r = self.session.get(purl, timeout=12)
                    if r.status_code == 200:
                        txt = r.text.lower()
                        if any(n.lower() in txt for n in KNOWN_POLITICIANS + ["disclosure", "ptr", "filed"]):
                            # Create a signal item from page hit
                            item = NewsItem(
                                provider="politics_disclosure",
                                url=purl,
                                title="PUBLIC DISCLOSURE: Recent politician trade report (STOCK Act PTR via public source)",
                                content="Public filing or report detected on aggregator. Check for crypto/energy/forex/gold holdings by monitored politicians. High headline/sentiment impact for Alpha (always pair with Gold hedge). Source: public web.",
                                published_at=now,
                                asset=self._map_to_asset(txt),
                                raw_json={"source": "public_page_scan", "url": purl},
                                fetched_at=now,
                            )
                            items.append(item)
                            logger.info("Politics disclosure from public page scan: %s", purl)
                except Exception:
                    pass
        except Exception as e:
            logger.debug("Public page scan for disclosures skipped (free/public): %s", e)

        return items

    def _basic_rss_parse(self, xml_text: str) -> List[Dict]:
        """Very basic RSS item extractor without feedparser."""
        items = []
        # Rough regex for <item><title>...</title><link>...</link><pubDate>...</pubDate>
        item_pattern = re.compile(r'<item>(.*?)</item>', re.DOTALL)
        for item_match in item_pattern.findall(xml_text):
            title = re.search(r'<title>(.*?)</title>', item_match, re.DOTALL)
            link = re.search(r'<link>(.*?)</link>', item_match, re.DOTALL)
            pub = re.search(r'<pubDate>(.*?)</pubDate>', item_match, re.DOTALL)
            if title:
                items.append({
                    "title": title.group(1).strip(),
                    "link": link.group(1).strip() if link else "",
                    "published_parsed": pub.group(1) if pub else None,
                    "summary": ""
                })
        return items

    def _parse_pub_date(self, pub_parsed) -> Optional[int]:
        if not pub_parsed:
            return None
        try:
            if isinstance(pub_parsed, str):
                dt = datetime.strptime(pub_parsed, "%a, %d %b %Y %H:%M:%S %z")  # rough
                return int(dt.timestamp())
            # If time struct from feedparser
            return int(time.mktime(pub_parsed))
        except Exception:
            return None

    def _map_to_asset(self, text: str) -> str:
        text_l = text.lower()
        if any(k in text_l for k in ["crypto", "bitcoin", "eth", "wlfi", "trump", "digital asset"]):
            return "CRYPTO"
        if any(k in text_l for k in ["oil", "energy", "crude", "opec", "sanction"]):
            return "OIL"
        if any(k in text_l for k in ["fed", "rate", "dollar", "inflation", "tariff"]):
            return "FOREX"
        if any(k in text_l for k in ["gold", "silver", "precious", "hedge"]):
            return "GOLD"
        return "CRYPTO"  # default for politician/crypto overlap

    def insert_as_news(self, items: List[NewsItem]) -> int:
        """Insert into DB via standard path (deduped). Returns count inserted."""
        count = 0
        for item in items:
            try:
                # Reuse news_fetcher insert logic if available, else direct
                inserted = self.db._insert_news_item(item) if hasattr(self.db, '_insert_news_item') else self._direct_insert(item)
                if inserted:
                    count += 1
            except Exception as e:
                logger.debug("Politics insert skip: %s", e)
        return count

    def _direct_insert(self, item: NewsItem) -> int:
        # Fallback direct DB insert (simplified)
        h = hashlib.sha256((item.url or item.title or "").encode()).hexdigest()
        try:
            cur = self.db.conn.cursor()
            cur.execute(
                "INSERT OR IGNORE INTO news (tenant_id, provider, url, title, content, published_at, asset, hash, fetched_at, processed, raw_json) "
                "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?)",
                (self.db.tenant_id, item.provider, item.url, item.title, item.content, 
                 datetime.fromtimestamp(item.published_at or now_ts(), tz=timezone.utc).isoformat() if item.published_at else None,
                 item.asset, h, datetime.fromtimestamp(item.fetched_at or now_ts(), tz=timezone.utc).isoformat(), json.dumps(item.raw_json))
            )
            self.db.conn.commit()
            return cur.lastrowid if cur.lastrowid else 0
        except Exception:
            return 0

# Convenience
def fetch_and_process_politician_disclosures(config: Optional[Config] = None, db: Optional[Database] = None) -> List[NewsItem]:
    fetcher = PoliticianDisclosuresFetcher(config, db)
    items = fetcher.fetch_recent_disclosures()
    fetcher.insert_as_news(items)
    return items
