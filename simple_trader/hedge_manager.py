"""
Hedge Manager - Use Gold & Silver to protect the Alpha book.

Core idea for the company:
- When we take big short-term bets in Crypto/Oil/Forex (news alpha), we automatically
  put on a gold/silver overlay to reduce net directional risk to "risk-off" events.
- This lets us "gain massive profits" on the aggressive side while "securing our assets".

Simple linear hedge for now (can be upgraded to beta/option overlays later).
"""

from __future__ import annotations

import logging
from dataclasses import dataclass
from typing import Dict, List, Optional, Tuple

from .config import CONFIG, Config
from .db import Database, get_default_db

logger = logging.getLogger("simple_trader.hedge_manager")


@dataclass
class HedgeAction:
    hedge_symbol: str
    direction: str  # "long" for gold when hedging short risk assets
    target_risk_usd: float
    current_hedge_risk: float
    action: str
    reason: str


class HedgeManager:
    def __init__(self, config: Optional[Config] = None, db: Optional[Database] = None):
        self.config = config or CONFIG
        self.db = db or get_default_db(self.config.database_path, tenant_id=self.config.tenant_id)

    def compute_required_hedge(self, alpha_risk_usd: float, current_core_risk: float) -> float:
        """How much gold/silver risk we should have on to hedge the current alpha."""
        if not self.config.enable_auto_hedge:
            return 0.0
        desired = alpha_risk_usd * self.config.hedge_ratio
        # Don't over-hedge beyond reasonable core allocation
        max_core = self.config.core_bucket_target_pct * (self.config.account_balance_usd * 0.08)  # rough
        return min(desired, max_core)

    def suggest_hedge_actions(self, current_risks: Dict[str, float]) -> List[HedgeAction]:
        """Return list of recommended hedge adjustments."""
        actions: List[HedgeAction] = []

        alpha_risk = sum(
            r for sym, r in current_risks.items()
            if self.config.asset_class_to_bucket.get(
                self._classify(sym), "ALPHA"
            ) == "ALPHA"
        )
        core_risk = sum(
            r for sym, r in current_risks.items()
            if self.config.asset_class_to_bucket.get(
                self._classify(sym), "ALPHA"
            ) == "CORE"
        )

        desired_gold = self.compute_required_hedge(alpha_risk, core_risk)
        current_gold = current_risks.get("GOLD", 0) + current_risks.get("XAUUSD", 0) + current_risks.get("GC=F", 0)

        drift = abs(desired_gold - current_gold) / max(1, desired_gold) if desired_gold > 0 else 0

        if drift > self.config.hedge_rebalance_threshold:
            action = "INCREASE_HEDGE" if desired_gold > current_gold else "REDUCE_HEDGE"
            actions.append(HedgeAction(
                hedge_symbol="GOLD",
                direction="long",
                target_risk_usd=desired_gold,
                current_hedge_risk=current_gold,
                action=action,
                reason=f"Alpha risk {alpha_risk:.0f}, desired hedge {desired_gold:.0f}, current {current_gold:.0f}"
            ))

        return actions

    def _classify(self, sym: str) -> str:
        from .market_data import _get_asset_class
        return _get_asset_class(sym)
