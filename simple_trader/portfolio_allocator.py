"""
Portfolio Allocator - The secret sauce for bucket management.

Core Bucket (Gold + Silver): Capital preservation, inflation hedge, risk-off anchor.
Alpha Bucket (Crypto + Forex + Oil): High-conviction, news-driven short-term trades for alpha/profit.

This module:
- Computes current vs target allocations (in risk terms, not notional).
- Suggests rebalancing trades (increase gold hedge when alpha is hot, trim alpha on risk-off).
- Provides per-symbol target risk given current book state.
- Works with the RiskEngine and HedgeManager.

For the company: This is how we "secure our assets" while still swinging for massive profits on the news edge.
"""

from __future__ import annotations

import logging
from dataclasses import dataclass
from typing import Dict, List, Optional, Tuple

from .config import CONFIG, Config
from .db import Database, get_default_db

logger = logging.getLogger("simple_trader.portfolio_allocator")


@dataclass
class AllocationTarget:
    symbol: str
    asset_class: str
    bucket: str  # "CORE" or "ALPHA"
    target_risk_usd: float
    current_risk_usd: float
    action: str  # "HOLD", "INCREASE", "DECREASE", "HEDGE"
    reason: str


class PortfolioAllocator:
    def __init__(self, config: Optional[Config] = None, db: Optional[Database] = None):
        self.config = config or CONFIG
        self.db = db or get_default_db(self.config.database_path, tenant_id=self.config.tenant_id)

    def get_bucket_targets(self, current_book_risk_usd: float) -> Dict[str, float]:
        """Return ideal risk USD for CORE and ALPHA buckets."""
        total = current_book_risk_usd or (self.config.account_balance_usd * 0.05)  # fallback
        core_target = total * self.config.core_bucket_target_pct
        alpha_target = total * self.config.alpha_bucket_target_pct
        return {"CORE": core_target, "ALPHA": alpha_target}

    def classify_symbol(self, symbol: str) -> Tuple[str, str]:
        """Return (asset_class, bucket)."""
        asset_class = "OTHER"
        s = symbol.upper()
        if any(x in s for x in ["XAU", "GOLD", "GC=F"]):
            asset_class = "GOLD"
        elif any(x in s for x in ["XAG", "SILVER", "SI=F"]):
            asset_class = "SILVER"
        elif any(x in s for x in ["CL=", "OIL", "WTI", "USOIL"]):
            asset_class = "OIL"
        elif any(x in s for x in ["BTC", "ETH", "SOL", "CRYPTO"]):
            asset_class = "CRYPTO"
        elif len(s) >= 6 and s[:3].isalpha() and s[3:6].isalpha():  # rough forex
            asset_class = "FOREX"

        bucket = self.config.asset_class_to_bucket.get(asset_class, "ALPHA")
        return asset_class, bucket

    def compute_desired_risk(self, symbol: str, proposed_risk_usd: float,
                             current_per_class: Dict[str, float],
                             current_total_risk: float) -> Tuple[float, str]:
        """
        Given a new signal's proposed risk, return (allowed_risk_usd, reason).
        Respects per-class caps and bucket balance.
        """
        asset_class, bucket = self.classify_symbol(symbol)
        account = self.config.account_balance_usd

        # Hard per-class cap
        max_for_class = getattr(self.config, f"max_exposure_{asset_class.lower()}", 0.15) * account
        current_class = current_per_class.get(asset_class, 0.0)
        room_in_class = max(0.0, max_for_class - current_class)

        # Bucket balance
        targets = self.get_bucket_targets(current_total_risk)
        bucket_key = bucket
        current_bucket = sum(r for ac, r in current_per_class.items()
                             if self.config.asset_class_to_bucket.get(ac, "ALPHA") == bucket_key)
        room_in_bucket = max(0.0, targets.get(bucket_key, 0) - current_bucket)

        allowed = min(proposed_risk_usd, room_in_class, room_in_bucket)

        reason = "ok"
        if allowed < proposed_risk_usd * 0.5:
            reason = f"Bucket/class limits hit for {asset_class} ({bucket})"

        return allowed, reason

    def suggest_rebalance(self, current_risks: Dict[str, float]) -> List[AllocationTarget]:
        """High-level suggestions for the team (what to add/reduce in gold vs alpha)."""
        suggestions: List[AllocationTarget] = []
        total = sum(current_risks.values()) or 1.0
        targets = self.get_bucket_targets(total)

        for sym, risk in current_risks.items():
            ac, bucket = self.classify_symbol(sym)
            target_risk = targets.get(bucket, 0) * (risk / total) if total > 0 else 0  # naive equal within bucket

            if risk > target_risk * 1.3:
                action = "DECREASE"
                reason = f"Overweight in {bucket} bucket"
            elif risk < target_risk * 0.7:
                action = "INCREASE"
                reason = f"Underweight in {bucket} - good entry opportunity?"
            else:
                action = "HOLD"
                reason = "Within tolerance"

            suggestions.append(AllocationTarget(
                symbol=sym, asset_class=ac, bucket=bucket,
                target_risk_usd=target_risk, current_risk_usd=risk,
                action=action, reason=reason
            ))

        # Special: if alpha is very hot and we have low gold, suggest increasing hedge
        alpha_risk = sum(r for ac, r in current_risks.items() if self.config.asset_class_to_bucket.get(ac) == "ALPHA")
        core_risk = sum(r for ac, r in current_risks.items() if self.config.asset_class_to_bucket.get(ac) == "CORE")
        if alpha_risk > core_risk * 1.8 and self.config.enable_auto_hedge:
            suggestions.append(AllocationTarget(
                symbol="GOLD_HEDGE", asset_class="GOLD", bucket="CORE",
                target_risk_usd=alpha_risk * self.config.hedge_ratio,
                current_risk_usd=core_risk,
                action="HEDGE",
                reason="Alpha risk significantly higher than core - increase gold hedge"
            ))

        return suggestions
