"""
Portfolio management helpers for Simple-Trader.

For an investment company, individual signals are not enough.
This module provides:
- Current portfolio exposure / risk summary
- Simple position sizing adjustments across the book (risk parity style)
- Basic correlation-naive diversification suggestions
- A hook that can be called before accepting new signals

Leaders (AQR, Bridgewater, quant crypto funds) use:
- Full portfolio optimization (HRP, risk-parity, mean-variance with views)
- Factor risk models
- Dynamic risk budgeting by regime
- Drawdown / tail-risk overlays (e.g. reduce all size when VIX or realized vol spikes)

This is a pragmatic starting point you can evolve into a real book risk system.
Integrate it in SignalManager before creating signals, and in your execution layer.
"""

from __future__ import annotations

import logging
from dataclasses import dataclass
from typing import Dict, List, Optional, Tuple

import numpy as np

from .config import CONFIG, Config
from .db import Database, get_default_db

logger = logging.getLogger("simple_trader.portfolio")


@dataclass
class PortfolioSnapshot:
    open_signals: int
    total_risk_usd: float
    total_notional_usd: float
    symbols: List[str]
    per_symbol_risk: Dict[str, float]
    max_single_risk_pct_of_book: float


class PortfolioManager:
    """
    Lightweight portfolio risk manager / allocator.
    """

    def __init__(self, config: Optional[Config] = None, db: Optional[Database] = None):
        self.config = config or CONFIG
        self.db = db or get_default_db(self.config.database_path, tenant_id=self.config.tenant_id)

    def get_current_exposure(self) -> PortfolioSnapshot:
        """Query open signals and aggregate risk/notional."""
        try:
            rows = self.db.execute_custom(
                "SELECT symbol, side, risk_amount, position_size, entry_price, leverage FROM signals WHERE status = 'open'"
            )
        except Exception:
            logger.exception("Failed to load open signals for portfolio exposure")
            rows = []

        total_risk = 0.0
        total_notional = 0.0
        per_symbol: Dict[str, float] = {}
        symbols = set()

        for r in rows or []:
            sym = (r.get("symbol") or "UNKNOWN").upper()
            symbols.add(sym)
            risk = float(r.get("risk_amount") or 0.0)
            lev = float(r.get("leverage") or 1.0)
            pos_size = float(r.get("position_size") or 0.0)
            entry = float(r.get("entry_price") or 0.0)

            total_risk += risk
            notional = pos_size * entry * max(1.0, lev)
            total_notional += notional
            per_symbol[sym] = per_symbol.get(sym, 0.0) + risk

        account = self.config.account_balance_usd or 100000.0
        max_single_pct = max((v / total_risk for v in per_symbol.values()), default=0.0) if total_risk > 0 else 0.0

        return PortfolioSnapshot(
            open_signals=len(rows or []),
            total_risk_usd=total_risk,
            total_notional_usd=total_notional,
            symbols=sorted(symbols),
            per_symbol_risk=per_symbol,
            max_single_risk_pct_of_book=max_single_pct,
        )

    def check_new_signal_allowed(self, symbol: str, proposed_risk_usd: float, max_total_risk_pct: float = 0.06) -> Tuple[bool, str]:
        """
        Return (allowed, reason).
        Enforces:
        - Total book risk cap
        - Per-symbol concentration (e.g. no single name > 25-30% of current risk)
        """
        snap = self.get_current_exposure()
        account = self.config.account_balance_usd or 100000.0
        max_book_risk = account * max_total_risk_pct

        if snap.total_risk_usd + proposed_risk_usd > max_book_risk:
            return False, f"Would exceed total portfolio risk budget ({max_total_risk_pct*100:.1f}% of account)"

        sym_risk = snap.per_symbol_risk.get(symbol.upper(), 0.0)
        proposed_sym_total = sym_risk + proposed_risk_usd
        if snap.total_risk_usd > 0 and (proposed_sym_total / (snap.total_risk_usd + proposed_risk_usd)) > 0.30:
            return False, f"Would concentrate >30% of book risk in {symbol}"

        # Simple drawdown guard hook (extend with a stored equity curve peak/trough if you track it)
        return True, "ok"

    def suggest_risk_parity_weights(self, min_weight: float = 0.05) -> Dict[str, float]:
        """
        Very simple risk-parity style suggestion for current open positions.
        (Assumes equal risk contribution target; in reality you would use vol + correlation matrix.)
        Returns symbol -> suggested fraction of *current total risk budget*.
        """
        snap = self.get_current_exposure()
        if not snap.symbols or snap.total_risk_usd <= 0:
            return {}

        n = len(snap.symbols)
        # naive equal risk contribution
        target_per = 1.0 / max(1, n)
        weights: Dict[str, float] = {}
        for s in snap.symbols:
            current = snap.per_symbol_risk.get(s, 0.0) / snap.total_risk_usd
            # suggest scaling factor
            weights[s] = max(min_weight, target_per)
        # normalize
        s = sum(weights.values())
        if s > 0:
            weights = {k: v / s for k, v in weights.items()}
        return weights


# Convenience
def get_portfolio_manager(config: Optional[Config] = None, db: Optional[Database] = None) -> PortfolioManager:
    return PortfolioManager(config=config, db=db)
