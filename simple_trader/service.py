"""
Production Service Runner for Linux VPS (systemd).

This is the main long-running process:
- Starts the periodic signal runner (news monitor + alpha/core logic).
- Starts the urgent opportunity/risk monitor (sends special Telegram messages).
- Starts the internal web dashboard (FastAPI on port 8080).
- Proper logging, graceful shutdown, health checks.

Run as systemd service (see simple-trader.service example).

Usage inside service:
    python -m simple_trader.service

Or directly:
    python -m simple_trader.service --interval 300 --dashboard-port 8080
"""

from __future__ import annotations

import argparse
import logging
import os
import signal
import sys
import threading
import time
from datetime import datetime, timezone

from .config import CONFIG
from .main import Orchestrator  # reuse the existing good orchestrator
from .web_dashboard import app as dashboard_app
from .telegram import TelegramNotifier
from .risk_engine import RiskEngine
from .portfolio_allocator import PortfolioAllocator
from .knowledge_base import is_risk_off_regime, get_decision_rationale, get_knowledge_for_prompt

import uvicorn

LOG = logging.getLogger("simple_trader.service")

class ServiceRunner:
    def __init__(self, interval_seconds: int = 300, dashboard_port: int = 8080):
        self.interval = interval_seconds
        self.dashboard_port = dashboard_port
        self.orchestrator = Orchestrator(config=CONFIG)
        self.telegram = TelegramNotifier(config=CONFIG, db=self.orchestrator.db)
        self.risk = RiskEngine(config=CONFIG, db=self.orchestrator.db)
        self.allocator = PortfolioAllocator(config=CONFIG, db=self.orchestrator.db)
        self._stop = threading.Event()
        self.dashboard_thread = None

    def _start_dashboard(self):
        """Run FastAPI in a background thread."""
        def run():
            LOG.info("Starting internal dashboard on port %s", self.dashboard_port)
            uvicorn.run(dashboard_app, host="0.0.0.0", port=self.dashboard_port, log_level="warning")
        self.dashboard_thread = threading.Thread(target=run, daemon=True)
        self.dashboard_thread.start()

    def _urgent_monitor_once(self):
        """Check for immediate opportunities or risks and blast Telegram."""
        try:
            book = self.risk.compute_book_risk()
            open_signals = self.orchestrator.db.get_open_signals(limit=50) or []

            # Risk breach
            if book.breaker_active or book.current_drawdown_pct > CONFIG.max_daily_loss_pct * 0.8:
                msg = f"🚨 URGENT RISK: Drawdown {book.current_drawdown_pct*100:.1f}% or breaker active. Book risk: {book.total_risk_usd:,.0f}. Consider reducing Alpha exposure / increasing Gold hedge."
                self.telegram.send_message(msg)
                LOG.warning(msg)

            # High opportunity: strong recent signals in under-allocated Alpha when regime allows
            alpha_risk = sum(r.get("risk_amount", 0) for r in open_signals 
                             if self.allocator.classify_symbol(r.get("symbol", ""))[1] == "ALPHA")
            target_alpha = book.total_risk_usd * CONFIG.alpha_bucket_target_pct if book.total_risk_usd > 0 else 0

            if alpha_risk < target_alpha * 0.6 and len(open_signals) < CONFIG.open_positions_limit:
                # Look for very recent high-conviction using knowledge
                for sig in open_signals[-5:]:
                    if "strong" in str(sig.get("decision_audit", "")).lower() or (sig.get("rr", 0) or 0) > 4.0:
                        rationale = get_decision_rationale(
                            sig.get("symbol"), alpha_risk, sig.get("risk_amount", 0),
                            llm_conf=0.85, pattern_conf=0.8, news_impact="high"
                        )
                        msg = f"💰 HIGH OPPORTUNITY (Alpha under-allocated): {sig['side'].upper()} {sig['symbol']} RR={sig.get('rr')}. {rationale[:220]}"
                        self.telegram.send_message(msg)
                        LOG.info("Sent urgent opportunity alert for %s", sig['symbol'])
                        break

            # Regime shift -> hedge suggestion
            if is_risk_off_regime(volatility_proxy=0.03):  # stub - improve with real VIX later
                msg = "🛡️ RISK-OFF REGIME DETECTED. HedgeManager recommends increasing Gold/Silver overlay immediately."
                self.telegram.send_message(msg)

        except Exception:
            LOG.exception("Urgent monitor cycle failed")

    def run(self):
        LOG.info("Simple-Trader Service starting. Interval=%ss, Dashboard port=%s", self.interval, self.dashboard_port)

        # Start dashboard
        self._start_dashboard()

        # Graceful shutdown
        def _handler(signum, frame):
            LOG.info("Shutdown signal received")
            self._stop.set()
        signal.signal(signal.SIGINT, _handler)
        signal.signal(signal.SIGTERM, _handler)

        last_urgent = 0

        while not self._stop.is_set():
            try:
                start = time.time()

                # Run the normal fetch/process/close/tune cycle (now with full Allocator/Risk/Hedge/Knowledge layers)
                self.orchestrator.fetch_news_once()
                self.orchestrator.process_news_once(max_news=50)
                self.orchestrator.close_expired_once()
                self.orchestrator.monitor_and_tune_once()

                # Urgent monitor every cycle or every ~5 min
                now = time.time()
                if now - last_urgent > 60:
                    self._urgent_monitor_once()
                    last_urgent = now

                # Log book health
                book = self.risk.compute_book_risk()
                LOG.info("Book health: risk=%.0f notional=%.0f dd=%.1f%% open=%d",
                         book.total_risk_usd, book.total_notional_usd, book.current_drawdown_pct*100, len(self.orchestrator.db.get_open_signals(limit=10) or []))

                elapsed = time.time() - start
                to_sleep = max(10, self.interval - elapsed)
                LOG.debug("Cycle took %.1fs, sleeping %.0fs", elapsed, to_sleep)

                # Interruptible sleep
                for _ in range(int(to_sleep)):
                    if self._stop.is_set():
                        break
                    time.sleep(1)

            except Exception:
                LOG.exception("Service cycle error")
                time.sleep(30)

        LOG.info("Service stopped cleanly")

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--interval", type=int, default=300)
    parser.add_argument("--dashboard-port", type=int, default=8080)
    args = parser.parse_args()

    # File logging for systemd journal + file
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s [%(levelname)s] %(name)s - %(message)s",
        handlers=[
            logging.StreamHandler(sys.stdout),
            logging.FileHandler("/var/log/simple-trader/service.log", mode="a") if os.path.exists("/var/log") else logging.NullHandler()
        ]
    )

    runner = ServiceRunner(interval_seconds=args.interval, dashboard_port=args.dashboard_port)
    runner.run()

if __name__ == "__main__":
    main()
