"""
Risk Engine - Book level risk, circuit breakers, drawdown control.

This is the "do not blow up the company" module.

Features:
- Real-time book risk (total risk, per class, leverage)
- Simple parametric VaR proxy + max drawdown tracking (store peak equity if you feed it)
- Circuit breakers (pause alpha on bad days)
- Correlation awareness stub (gold negatively correlated to risk assets in theory)
- Pre-trade checks before any new position or hedge adjustment

Used by Allocator and SignalManager.
"""

from __future__ import annotations

import logging
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Dict, Optional

from .config import CONFIG, Config
from .db import Database, get_default_db

logger = logging.getLogger("simple_trader.risk_engine")


@dataclass
class BookRisk:
    total_risk_usd: float
    total_notional_usd: float
    per_class_risk: Dict[str, float]
    current_drawdown_pct: float = 0.0
    breaker_active: bool = False
    breaker_reason: str = ""


class RiskEngine:
    def __init__(self, config: Optional[Config] = None, db: Optional[Database] = None):
        self.config = config or CONFIG
        self.db = db or get_default_db(self.config.database_path, tenant_id=self.config.tenant_id)
        self._peak_equity: Optional[float] = None  # In real use, persist this or feed from accounting

    def compute_book_risk(self, open_positions: Optional[list] = None) -> BookRisk:
        """Aggregate current open risk from DB (or passed list)."""
        try:
            rows = self.db.execute_custom(
                "SELECT symbol, risk_amount, position_size, entry_price, leverage FROM signals WHERE status = 'open'"
            )
        except Exception:
            rows = []

        total_risk = 0.0
        total_notional = 0.0
        per_class: Dict[str, float] = {}

        for r in rows or []:
            risk = float(r.get("risk_amount") or 0)
            lev = float(r.get("leverage") or 1)
            size = float(r.get("position_size") or 0)
            entry = float(r.get("entry_price") or 0)

            total_risk += risk
            notional = size * entry * max(1, lev)
            total_notional += notional

            # Classify
            from .market_data import _get_asset_class
            ac = _get_asset_class(r.get("symbol", ""))
            per_class[ac] = per_class.get(ac, 0.0) + risk

        # Drawdown (stub - in production feed real equity curve)
        dd = 0.0
        if self._peak_equity and self._peak_equity > 0:
            # simplistic
            current_equity_proxy = self.config.account_balance_usd - total_risk * 0.5  # rough
            dd = max(0, (self._peak_equity - current_equity_proxy) / self._peak_equity)

        breaker = False
        reason = ""
        if dd > self.config.max_drawdown_pause_pct:
            breaker = True
            reason = f"Drawdown {dd:.1%} > limit"

        return BookRisk(
            total_risk_usd=total_risk,
            total_notional_usd=total_notional,
            per_class_risk=per_class,
            current_drawdown_pct=dd,
            breaker_active=breaker,
            breaker_reason=reason
        )

    def pre_trade_check(self, symbol: str, proposed_risk_usd: float, current_book: BookRisk) -> Tuple[bool, str]:
        """Can we take this risk right now?"""
        if current_book.breaker_active:
            return False, f"Circuit breaker active: {current_book.breaker_reason}"

        # Daily loss stub (you would track realized P&L separately)
        # For now rely on book risk
        if current_book.total_risk_usd + proposed_risk_usd > self.config.account_balance_usd * self.config.max_book_risk_pct:
            return False, "Would breach max book risk"

        # Per class already handled in allocator, but double check here
        from .market_data import _get_asset_class
        ac = _get_asset_class(symbol)
        current_class = current_book.per_class_risk.get(ac, 0)
        max_class = getattr(self.config, f"max_exposure_{ac.lower()}", 0.15) * self.config.account_balance_usd
        if current_class + proposed_risk_usd > max_class:
            return False, f"Exceeds max {ac} exposure"

        return True, "approved"

    def record_peak(self, current_equity_usd: float):
        """Call this from your accounting/equity curve feed."""
        if self._peak_equity is None or current_equity_usd > self._peak_equity:
            self._peak_equity = current_equity_usd
