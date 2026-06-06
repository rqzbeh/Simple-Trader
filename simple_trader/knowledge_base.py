"""
Finance & Trading Knowledge Base - Deep Embedded Expertise for the Secret Formula

This module provides structured, professional knowledge from institutional trading, hedge funds, and quant practices.

Used to:
- Augment LLM prompts with expert context (no hallucinations on basics).
- Influence scoring, sizing, hedging decisions with proven rules.
- Provide "judgment" heuristics for the ML/tuner to reference when learning from mistakes.
- Educate the system on asset-specific behaviors for GOLD/SILVER (Core hedge), CRYPTO/FOREX/OIL (Alpha).

Core Philosophy:
- Alpha for asymmetric upside on news catalysts (short-term, high conviction).
- Core (Gold/Silver) as ballast: negative beta to risk assets during stress, inflation protection, liquidity.
- Never let Alpha risk overwhelm the ability of Core to stabilize the book.
- Learn from every outcome: tag "mistakes" (e.g., ignored hedge, over-levered on low-conviction news) and adjust.

Key Concepts & Formulas (use these in logic and prompts):
"""

from __future__ import annotations

import math
from typing import Dict, Any, Optional

# --- Position Sizing & Risk ---
def kelly_fraction(win_rate: float, avg_win: float, avg_loss: float) -> float:
    """
    Kelly Criterion for optimal bet size.
    f = (bp - q) / b   where b=odds (win/loss ratio), p=win prob, q=1-p
    We use fractional Kelly (0.25-0.5) in practice for safety (drawdown control).
    Returns recommended fraction of bankroll.
    """
    if avg_loss <= 0 or win_rate <= 0 or win_rate >= 1:
        return 0.0
    b = avg_win / abs(avg_loss)  # net odds
    p = win_rate
    q = 1 - p
    kelly = (b * p - q) / b
    return max(0.0, min(1.0, kelly))

def fractional_kelly(win_rate: float, avg_win: float, avg_loss: float, fraction: float = 0.25) -> float:
    """Conservative Kelly used by pros (e.g. 1/4 Kelly to survive variance)."""
    return kelly_fraction(win_rate, avg_win, avg_loss) * fraction

def vol_target_position_size(account: float, target_vol_pct: float, current_vol_pct: float, 
                             base_risk_pct: float) -> float:
    """
    Volatility targeting: scale position so expected vol contribution ~ target.
    Common at funds like AQR, Bridgewater style risk parity.
    """
    if current_vol_pct <= 0:
        return base_risk_pct
    scale = target_vol_pct / current_vol_pct
    return min(base_risk_pct * scale, base_risk_pct * 2.0)  # cap leverage

# --- Portfolio & Hedging ---
def risk_parity_weight(vols: Dict[str, float]) -> Dict[str, float]:
    """
    Simple risk parity: inverse volatility weights (normalized).
    For multi-asset: Gold low vol -> higher weight in Core; Crypto high vol -> smaller in Alpha.
    """
    inv_vols = {k: 1.0 / max(v, 1e-6) for k, v in vols.items()}
    total = sum(inv_vols.values())
    return {k: v / total for k, v in inv_vols.items()}

def hedge_ratio_calc(alpha_risk: float, gold_beta_to_risk: float = -0.4, target_net_beta: float = 0.2) -> float:
    """
    Approximate hedge: how much gold to hold to achieve desired net portfolio beta.
    Gold often has negative correlation to equities/risk assets (especially in crises).
    """
    if alpha_risk <= 0:
        return 0.0
    # Solve: alpha_risk * 1.0 + hedge * gold_beta = target_net * (alpha + hedge)
    # Simplified linear: hedge_amount = (alpha_risk * (1 - target_net)) / (1 - gold_beta) approx
    hedge = alpha_risk * (1.0 - target_net_beta) / max(1.0 - gold_beta_to_risk, 0.1)
    return max(0.0, hedge)

# --- Asset Specific Knowledge ---
ASSET_KNOWLEDGE: Dict[str, Dict[str, Any]] = {
    "GOLD": {
        "role": "Core ballast and hedge. Negative beta to USD strength, equities, and risk sentiment.",
        "key_drivers": ["real rates (inverse)", "USD (inverse)", "geopolitics/inflation", "central bank buying"],
        "news_sensitivity": "Medium. Flight-to-safety on bad news for risk assets. Strong on Fed, CPI, wars.",
        "vol_regime": "Lower vol than crypto/oil. Good for sizing up in Core when VIX or risk-off.",
        "hedge_behavior": "Buy gold (or increase long) when Alpha (crypto/oil) risk is elevated or news turns risk-off.",
        "common_mistake": "Over-hedging in strong USD bull markets or ignoring opportunity cost.",
    },
    "SILVER": {
        "role": "Higher beta version of gold. Industrial + monetary. More volatile than gold, used for modest Core satellite.",
        "key_drivers": ["gold + industrial demand (solar, electronics)", "USD", "inflation"],
        "news_sensitivity": "Higher than gold on economic data.",
    },
    "OIL": {
        "role": "Alpha. Geopolitical + supply shock driven. High short-term alpha from news (OPEC, wars, inventories, China demand).",
        "key_drivers": ["geopolitics (Middle East)", "OPEC decisions", "US inventories (EIA)", "global growth/China", "USD (inverse)"],
        "news_sensitivity": "Very high. Headlines move it fast. Good for 2H-24H trades.",
        "risks": "High vol, contango/backwardation (futures), storage costs. Use futures-aware sizing.",
        "common_mistake": "Chasing headlines without checking positioning (COT reports) or technicals.",
    },
    "CRYPTO": {
        "role": "Highest alpha potential + highest risk. News (regulation, ETF flows, macro correlation, on-chain) drives short-term moves.",
        "key_drivers": ["ETF flows (spot Bitcoin)", "regulation/Fed", "halving cycles (longer)", "on-chain metrics (exchange flows)", "risk sentiment"],
        "news_sensitivity": "Extreme on regulatory/Fed news. Often leads or lags equities.",
        "vol_regime": "Very high. Use aggressive vol targeting and tight stops or wide for swings.",
        "hedge": "Reduce or hedge with gold on risk-off crypto news (e.g. ETF outflow + bad macro).",
    },
    "FOREX": {
        "role": "Alpha with carry + event-driven. Major pairs (EURUSD, GBPUSD, USDJPY) have good liquidity for short-term news trades.",
        "key_drivers": ["interest rate differentials (carry)", "central bank speakers", "CPI/PPI data", "risk sentiment (safe havens: JPY, CHF)"],
        "news_sensitivity": "High around data releases and FOMC/ECB. Use session awareness (London/NY overlap best).",
        "risks": "Leverage amplifies quickly. Watch for weekend gaps on majors.",
    },
    "WHALE": {
        "role": "High-impact on-chain sentiment for CRYPTO (and sometimes OIL via correlated flows). Large transfers to exchanges often signal distribution (bearish), accumulation or cold wallet moves bullish.",
        "key_drivers": ["exchange inflows/outflows", "large single tx (>1000 BTC or 10k ETH)", "known whale wallets (e.g. public Trump family or institutional)", "timing relative to news"],
        "news_sensitivity": "Very high for crypto alpha. Whale buys during dips can precede pumps; sells before dumps.",
        "risks": "Can be noise or coordinated. Always combine with volume, funding rates, and other signals. Not 'insider' but public data.",
        "hedge": "If large whale accumulation in crypto while Core gold is low, consider increasing hedge or alpha size cautiously.",
    },
    "POLITICS": {
        "role": "Sentiment and potential 'smart money' or headline driver for CRYPTO (Trump family ventures like WLFI), OIL (energy policy, sanctions), FOREX (tariffs, geopolitics), GOLD (inflation hedges via policy).",
        "key_drivers": ["US Congress/House/Senate periodic disclosures (45-day lag)", "Trump/Trump Jr/Eric Trump crypto announcements or holdings", "Nancy Pelosi or other prominent members' trades (often tracked publicly)", "bills, hearings, executive orders on crypto regulation, oil, rates"],
        "news_sensitivity": "Extremely high for short-term alpha on announcements. Disclosures can move markets if 'timed' well (use as sentiment, not advice).",
        "risks": "Delayed reporting, not real-time insider. Media hype around 'Pelosi trades' or 'Trump crypto' can create volatility. Cross-reference with on-chain and news.",
        "hedge": "Politician pro-crypto stance (e.g. Trump) bullish for crypto alpha, may warrant reducing gold hedge temporarily. Geopolitical (oil) or rate policy shifts affect all.",
    },
}

# --- Common Professional Rules & Pitfalls ---
PROFESSIONAL_RULES = [
    "Always size positions so that a single adverse move (2-3x ATR) does not breach daily loss limit.",
    "Core bucket (Gold/Silver) should act as ballast: when Alpha draws down >2-3%, Core should be flat or positive.",
    "News edge decays fast. Highest conviction in first 1-4 hours after major catalyst. Use 2H-24H horizon.",
    "Never average down on a losing Alpha position without new information. Use the hedge instead.",
    "Track 'regret' trades: positions you sized too big/small or didn't hedge. Log them for the tuner.",
    "Liquidity matters: Crypto and Oil can gap; size accordingly. Gold/Forex majors are more forgiving.",
    "Correlation is not static. Gold-beta to crypto/oil increases in crises.",
    "Kelly is theoretical maximum; pros use 1/4 to 1/2 Kelly to survive the path (drawdowns kill accounts before edge does).",
    "Whale trades and politician disclosures are PUBLIC sentiment/on-chain signals only — never treat as guaranteed 'insider' alpha. Always cross with multiple sources, volume, and strict risk management.",
    "Large exchange inflows from whales often precede short-term selling pressure in crypto; accumulation in cold wallets can be bullish but confirm with other data.",
    "Politician trades (e.g. Trump family crypto, Pelosi disclosures) create headlines and volatility — use for timing alpha entries in CRYPTO/OIL/FOREX but size small and hedge with GOLD.",
    # Buffett level: Long-term value, margin of safety, economic moats/fundamentals
    "Buffett: Buy quality assets (strong 'moat' like gold as inflation hedge or oil with supply constraints) at margin of safety (when fear high, e.g. risk-off for gold). Ignore short-term noise; focus on intrinsic value drivers (real rates for gold, geopolitics for oil).",
    "Buffett: Economic indicators (Fed policy, inflation, USD strength) as primary; use for Core bucket sizing (increase gold when value high).",
    # Simons level: Data-driven quant, patterns in noise, ML edges, statistical rigor
    "Simons: Find small edges in vast data (whale flows + politician sentiment + news sequences + price patterns); many weak signals compound. Use ML to detect non-obvious correlations (e.g., whale buy + low hedge -> higher loss prob).",
    "Simons: Avoid overfitting with out-of-sample (backtests), regime awareness, position sizing based on edge confidence. Online learning from every outcome (win/loss causes). High frequency of small decisions, but here adapted to 2H-24H with strict risk.",
]

def get_knowledge_for_prompt(asset_classes: list[str] = None) -> str:
    """Generate rich context string for LLM prompts or decision explanations."""
    knowledge = "You are an expert institutional trader running a multi-strategy book focused on GOLD, SILVER (Core/preservation + hedge), CRYPTO, FOREX, OIL (Alpha/news-driven short term).\n\n"
    knowledge += "Key principles:\n" + "\n".join(f"- {r}" for r in PROFESSIONAL_RULES) + "\n\n"
    
    if asset_classes:
        for ac in asset_classes:
            if ac in ASSET_KNOWLEDGE:
                k = ASSET_KNOWLEDGE[ac]
                knowledge += f"### {ac} specifics:\n"
                for key, val in k.items():
                    knowledge += f"- {key}: {val}\n"
                knowledge += "\n"
    return knowledge

def get_decision_rationale(asset: str, current_bucket_risk: float, proposed_risk: float, 
                           llm_conf: float, pattern_conf: float, news_impact: str = "") -> str:
    """Generate expert-style justification for a decision (used in audit logs and Telegram)."""
    ac = asset.upper()
    k = ASSET_KNOWLEDGE.get(ac, {})
    rationale = f"Decision for {asset} (class={ac}, bucket={k.get('role', 'Alpha')}):\n"
    rationale += f"- Current {ac} risk in book: {current_bucket_risk:.1%} of target.\n"
    rationale += f"- Proposed addition: {proposed_risk:.0f} USD risk.\n"
    rationale += f"- Conviction: LLM {llm_conf:.2f}, Pattern {pattern_conf:.2f}. "
    if news_impact:
        rationale += f"News catalyst strength: {news_impact}. "
    rationale += "\n"
    if ac in ["GOLD", "SILVER"]:
        rationale += "- Core logic: Increasing defensive ballast / hedge ratio per risk-parity and negative beta rules.\n"
    else:
        rationale += "- Alpha logic: High short-term edge from news + pattern alignment. Size with vol target + fractional Kelly. Hedge with Gold if book beta rises.\n"
    rationale += "Professional check: " + " | ".join(PROFESSIONAL_RULES[:2]) + "\n"
    return rationale

# Simple regime stub (can be expanded with real VIX or realized vol)
def is_risk_off_regime(volatility_proxy: float = 0.0, news_sentiment: str = "neutral") -> bool:
    """Heuristic: boost hedges in risk-off."""
    if volatility_proxy > 0.025 or "risk-off" in news_sentiment.lower() or "hawkish" in news_sentiment.lower():
        return True
    return False
