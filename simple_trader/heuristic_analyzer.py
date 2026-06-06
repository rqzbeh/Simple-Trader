"""
Heuristic Analyzer - Zero API Key "LLM" Replacement

Uses the rich knowledge_base + pattern signals + news keywords + asset rules
to generate high-quality direction, confidence, summary, and leverage suggestions.

This allows the entire system to run with **zero paid API keys** (only optional Telegram for alerts).

It is deliberately conservative and knowledge-driven, mimicking what a good junior analyst would say based on the embedded expertise.
"""

from __future__ import annotations

import logging
import re
from typing import Optional

from .config import CONFIG, Config
from .db import NewsItem
from .knowledge_base import ASSET_KNOWLEDGE, PROFESSIONAL_RULES, is_risk_off_regime, get_knowledge_for_prompt

# Lazy import to keep heuristic usable even if pandas not installed (pure free mode)
try:
    from .pattern_detector import PatternMatch
except Exception:
    PatternMatch = None  # type: ignore

logger = logging.getLogger("simple_trader.heuristic_analyzer")

class HeuristicAnalyzer:
    """
    Rule + knowledge based analyzer. No external calls.
    Produces output compatible with LLMSummary.
    """

    def __init__(self, config: Optional[Config] = None):
        self.config = config or CONFIG

    def analyze_news(self, news_item: NewsItem, patterns: list[PatternMatch]) -> dict:
        """
        Analyze a news item + detected patterns using deep knowledge.
        Returns a dict that can be turned into LLMSummary.
        """
        text = (news_item.title or "") + " " + (news_item.content or "")
        text_lower = text.lower()
        asset = news_item.asset or "UNKNOWN"
        ac = self._classify_asset(asset)

        # Base from patterns
        pattern_direction = None
        pattern_conf = 0.0
        if patterns and PatternMatch is not None:
            try:
                best = max(patterns, key=lambda p: getattr(p, 'confidence', 0))
                pattern_direction = best.direction
                pattern_conf = best.confidence
            except Exception:
                pass

        # Keyword-based sentiment from knowledge (init early to avoid UnboundLocal in boosts)
        direction = "neutral"
        confidence = 0.55
        impact = 0.0
        summary_parts = []

        # Special boost for whale and politics (public signals for alpha) - politician disclosures are lagged but high-signal sentiment (STOCK Act public)
        provider = getattr(news_item, 'provider', '') or ''
        is_whale = 'whale' in provider.lower() or 'whale' in (news_item.title or '').lower() or 'large' in (news_item.title or '').lower() and ('btc' in (news_item.title or '').lower() or 'eth' in (news_item.title or '').lower())
        is_politics = 'politics' in provider.lower() or 'disclosure' in provider.lower() or any(p.lower() in (news_item.title or '').lower() + (news_item.content or '').lower() for p in ['trump', 'pelosi', 'congress', 'senate', 'disclosure', 'wlfi', 'stock act', 'ptr'])

        if is_whale:
            ac = 'WHALE'
            confidence = max(confidence, 0.72)
        if is_politics:
            ac = 'POLITICS'
            confidence = max(confidence, 0.68)  # public disclosure -> solid alpha timing catalyst (cross with other data)
            # Buffett: always consider hedge moat
            summary_parts.append("Public politician disclosure (lagged PTR): high sentiment for Alpha bucket; size small + Gold hedge per knowledge.")

        k = ASSET_KNOWLEDGE.get(ac, {})

        # Strong override for gold titles (common in free RSS)
        if ac in ('GOLD', 'SILVER') or 'gold' in (news_item.title or '').lower() or 'xau' in asset.lower():
            text_l = (news_item.title or '' + ' ' + (news_item.content or '')).lower()
            if any(x in text_l for x in ['risk-off', 'risk off', 'safe haven', 'dovish', 'rate cut', 'inflation hedge']):
                direction = 'long'
                confidence = 0.78
                summary_parts.append('Strong gold bullish from risk-off / dovish / safe haven keywords (knowledge override).')
            elif any(x in text_l for x in ['hawkish', 'strong dollar']):
                direction = 'short'
                confidence = 0.62

        # Risk-off / hedge triggers - asset specific first (gold loves risk-off)
        risk_off_keywords = ["recession", "inflation", "war", "geopolitic", "fed hawkish", "rate hike", "crisis", "risk off", "risk-off", "selloff", "safe haven"]
        is_risk_off = any(kw in text_lower for kw in risk_off_keywords)

        if ac in ("GOLD", "SILVER"):
            if is_risk_off or any(x in text_lower for x in ["dovish", "rate cut", "weaker dollar", "inflation hedge"]):
                direction = "long"
                confidence = 0.78
                impact = 0.6
                summary_parts.append("Risk-off / dovish / safe haven - bullish for gold/silver (core knowledge).")
            elif any(x in text_lower for x in ["strong dollar", "hawkish"]):
                direction = "short"
                confidence = 0.62
                summary_parts.append("Hawkish / strong USD - bearish for gold/silver.")
        elif is_risk_off:
            direction = "short"
            confidence = 0.65
            impact = -0.5
            summary_parts.append("Risk-off signals - bearish pressure on risk assets (crypto/oil/forex).")

        # Whale and Politics special handling (public signals for alpha)
        if ac == "WHALE" or is_whale:
            if "buy" in text_lower or "accumulation" in text_lower or "cold" in text_lower:
                direction = "long"
                confidence = max(confidence, 0.72)
                summary_parts.append("Whale accumulation (public on-chain) - bullish crypto alpha signal.")
            else:
                direction = "short"
                confidence = max(confidence, 0.68)
                summary_parts.append("Whale distribution or large exchange move - caution for crypto.")
        if ac == "POLITICS" or is_politics:
            if "trump" in text_lower or "pro-crypto" in text_lower or "wlfi" in text_lower:
                direction = "long"
                confidence = max(confidence, 0.70)
                summary_parts.append("Pro-crypto politician announcement - high impact bullish for crypto alpha.")
            elif "pelosi" in text_lower or "disclosure" in text_lower:
                direction = "neutral"
                confidence = max(confidence, 0.60)
                summary_parts.append("Politician disclosure - headline volatility; use for timing but hedge.")

        # Asset specific keyword rules
        if ac == "GOLD":
            if any(x in text_lower for x in ["rate cut", "dovish", "weaker dollar", "inflation hedge", "risk off", "safe haven"]):
                direction = "long"
                confidence = max(confidence, 0.78)
                summary_parts.append("Positive for gold: lower real rates / USD weakness / risk-off per knowledge base.")
            if any(x in text_lower for x in ["strong dollar", "hawkish fed", "higher rates"]):
                direction = "short"
                confidence = max(confidence, 0.65)
                summary_parts.append("Negative: strong USD or higher rates pressure gold.")

        elif ac == "OIL":
            if any(x in text_lower for x in ["opec cut", "supply disruption", "geopolitic", "middle east"]):
                direction = "long"
                confidence = max(confidence, 0.72)
                summary_parts.append("Bullish supply shock or OPEC action.")
            if any(x in text_lower for x in ["demand weakness", "china slowdown", "recession"]):
                direction = "short"
                confidence = max(confidence, 0.68)

        elif ac == "CRYPTO":
            if any(x in text_lower for x in ["etf inflow", "approval", "adoption", "halving"]):
                direction = "long"
                confidence = max(confidence, 0.7)
            if any(x in text_lower for x in ["ban", "sec", "hack", "regulation"]):
                direction = "short"
                confidence = max(confidence, 0.65)

        elif ac == "FOREX":
            # Very basic carry + data reaction
            if "usd" in text_lower and "strong" in text_lower:
                # Depends on pair, but simplify
                direction = "short" if "eur" in text_lower or "gbp" in text_lower else "long"
                confidence = 0.6

        # Blend with pattern if present
        if pattern_direction and pattern_conf > 0.6:
            if direction == "neutral":
                direction = pattern_direction
                confidence = max(confidence, pattern_conf * 0.9)
            elif direction == pattern_direction:
                confidence = max(confidence, (confidence + pattern_conf) / 2)
            else:
                # Conflict - lower confidence
                confidence = min(confidence, 0.55)

        # Regime adjustment
        if is_risk_off_regime():
            if ac in ("GOLD", "SILVER"):
                confidence = min(0.92, confidence + 0.1)
            else:
                confidence = max(0.3, confidence - 0.15)

        # Conservative cap
        confidence = max(0.35, min(0.88, confidence))

        # Final strong override for obvious gold risk-off headlines (free RSS common case)
        title_l = (news_item.title or '').lower()
        if 'gold' in title_l and ('risk' in title_l or 'dovish' in title_l or 'safe' in title_l):
            direction = 'long'
            confidence = 0.78

        # Buffett value overlay (margin of safety, fundamentals for Core)
        if ac in ("GOLD", "SILVER"):
            # Simple: high value when risk-off or dovish (real rates low)
            if is_risk_off or "dovish" in text_lower or "inflation" in text_lower:
                confidence = min(0.9, confidence + 0.1)  # margin of safety boost
                summary_parts.append("Buffett value: High margin of safety for gold as inflation/risk hedge.")
        if ac in ("OIL", "CRYPTO"):
            # Simons quant: sequence + data edge
            if is_whale or is_politics:
                confidence = min(0.85, confidence + 0.05)
                summary_parts.append("Simons edge: Public whale/politician data as statistical pattern booster.")

        # Build summary
        summary = ". ".join(summary_parts) if summary_parts else f"Heuristic analysis for {ac} based on embedded professional knowledge and current patterns."
        summary += f" Key drivers from knowledge: {k.get('key_drivers', ['macro', 'news'])}"

        # Recommended leverage (conservative)
        rec_lev = 1
        if confidence > 0.75 and ac in ("CRYPTO", "OIL"):
            rec_lev = 2
        elif confidence > 0.7 and ac == "FOREX":
            rec_lev = 3
        elif ac in ("GOLD", "SILVER"):
            rec_lev = 1  # Core is lower leverage

        # Cap by config
        max_lev = self.config.max_leverage_crypto if ac == "CRYPTO" else self.config.max_leverage_forex if ac in ("FOREX", "OIL") else 3
        rec_lev = min(rec_lev, max_lev)

        return {
            "direction": direction,
            "confidence": round(confidence, 2),
            "impact_score": round(impact, 2),
            "summary": summary[:450],
            "asset": asset,
            "recommended_leverage": rec_lev,
            "provider": "heuristic_knowledge",
            "raw": {"rules_matched": len(summary_parts), "regime_risk_off": is_risk_off_regime()}
        }

    def _classify_asset(self, symbol: str) -> str:
        s = str(symbol).upper().replace('/', '').replace('-', '').replace('=F', '').replace('=X', '')
        if s in ('XAUUSD', 'XAU', 'GC', 'GOLD'):
            return 'GOLD'
        if s in ('XAGUSD', 'XAG', 'SI', 'SILVER'):
            return 'SILVER'
        if s in ('CL', 'USOIL', 'WTI', 'OIL', 'CRUDE'):
            return 'OIL'
        if any(c in s for c in ('BTC', 'ETH', 'SOL')):
            return 'CRYPTO'
        if len(s) >= 6 and s[:3].isalpha() and s[3:6].isalpha():
            return 'FOREX'
        return 'OTHER'
