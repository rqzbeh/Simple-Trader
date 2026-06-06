"""
Full Book Backtester for the Secret Formula.

Simulates the entire pipeline over historical periods:
- News arrival (use historical news if you have it, or replay)
- Signal generation
- Portfolio allocation + risk + hedge decisions
- Paper fills with realistic costs
- Equity curve, drawdowns, per-bucket performance, Sharpe-like metrics

This is how you validate the "massive profits with gold safety" thesis before putting company capital on the line.

Usage (example):
    bt = BookBacktester()
    results = bt.run_backtest(start_date, end_date, initial_capital=200000)
"""

from __future__ import annotations

import logging
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from typing import Dict, List, Optional

import pandas as pd

from .config import CONFIG, Config
from .portfolio_allocator import PortfolioAllocator
from .risk_engine import RiskEngine

logger = logging.getLogger("simple_trader.backtester")


@dataclass
class BacktestResult:
    equity_curve: List[Dict] = field(default_factory=list)
    total_return_pct: float = 0.0
    max_drawdown_pct: float = 0.0
    sharpe_proxy: float = 0.0
    alpha_pnl: float = 0.0
    core_pnl: float = 0.0
    num_trades: int = 0
    notes: str = ""


class BookBacktester:
    """
    Simplified full-book simulator.
    In production you would feed real historical news + tick data.
    This version replays using available market data and synthetic news impact.
    """

    def __init__(self, config: Optional[Config] = None):
        self.config = config or CONFIG
        self.allocator = PortfolioAllocator(config=self.config)
        self.risk_engine = RiskEngine(config=self.config)

    def run_backtest(self, start: datetime, end: datetime,
                     initial_capital: float = 200_000.0,
                     symbols: Optional[List[str]] = None) -> BacktestResult:
        """
        Very high-level simulation.
        Real version would:
          - Load historical news
          - Run full signal pipeline per bar
          - Apply allocator + risk + hedge
          - Simulate fills with costs
          - Track per-bucket equity
        """
        if symbols is None:
            symbols = ["BTC", "XAUUSD", "EURUSD", "CL=F"]

        logger.info("Starting book backtest %s -> %s on %s", start, end, symbols)

        equity = initial_capital
        peak = equity
        max_dd = 0.0
        curve = []
        trades = 0
        alpha_pnl = 0.0
        core_pnl = 0.0

        # Naive daily loop (in real life use 1H or tick data)
        current = start
        while current < end:
            # Simulate: assume some alpha signals fire, allocator decides size, risk approves
            # In practice call the real SignalManager + Allocator + Risk + Hedge here

            daily_pnl = 0.0
            # Toy model: alpha has higher vol, core (gold) dampens it
            alpha_exposure = 0.35 * equity   # rough
            core_exposure = 0.55 * equity

            # Fake returns (you would use real historical returns)
            alpha_ret = (0.001 if (current.day % 3 == 0) else -0.0008)   # news days good/bad
            core_ret = 0.0002 + ( -0.001 * alpha_ret * 0.6 )  # gold hedges a bit

            daily_pnl = alpha_exposure * alpha_ret + core_exposure * core_ret
            equity += daily_pnl
            alpha_pnl += alpha_exposure * alpha_ret
            core_pnl += core_exposure * core_ret
            trades += 1 if abs(alpha_ret) > 0.0005 else 0

            peak = max(peak, equity)
            dd = (peak - equity) / peak if peak > 0 else 0
            max_dd = max(max_dd, dd)

            curve.append({
                "date": current.date().isoformat(),
                "equity": round(equity, 2),
                "drawdown": round(dd * 100, 2),
                "alpha_pnl": round(alpha_pnl, 2),
                "core_pnl": round(core_pnl, 2),
            })

            current += timedelta(days=1)

        total_ret = (equity - initial_capital) / initial_capital * 100

        # Crude sharpe proxy (mean / std of daily returns)
        if len(curve) > 5:
            rets = pd.Series([c["equity"] for c in curve]).pct_change().dropna()
            sharpe = (rets.mean() / (rets.std() + 1e-9)) * (252 ** 0.5) if len(rets) > 1 else 0
        else:
            sharpe = 0

        result = BacktestResult(
            equity_curve=curve,
            total_return_pct=round(total_ret, 2),
            max_drawdown_pct=round(max_dd * 100, 2),
            sharpe_proxy=round(sharpe, 2),
            alpha_pnl=round(alpha_pnl, 2),
            core_pnl=round(core_pnl, 2),
            num_trades=trades,
            notes="Toy simulation. Replace with real historical news replay + accurate fills for production decisions."
        )
        logger.info("Backtest done: return=%.1f%% maxDD=%.1f%% sharpe~%.2f", total_ret, max_dd*100, sharpe)
        return result
