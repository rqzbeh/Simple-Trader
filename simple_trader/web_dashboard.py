"""
Internal Web Dashboard for the Secret Formula Investing System (beautiful, production-grade).

Run: uvicorn simple_trader.web_dashboard:app --host 0.0.0.0 --port 8080  (or via the service)

Major sections:
- Hero P&L metrics: Realized, Unrealized (MTM via live prices), Total est, Win Rate, Open positions + risk
- How trades are doing: Full equity / cumulative P&L line chart + recent trades P&L bars
- Rich trade log: Latest trades/positions with OPEN/CLOSED badges, entry, current/exit price, colored P/L, filterable (All/Open/Closed + symbol search)
- Risk allocation pie + per-asset bars (Core vs Alpha emphasis)
- Book risk + breaker status
- Urgent alerts + Whale/Politician (public disclosures) signals
- ML learnings (cause weights + regrets) showing the system is getting better

Self-contained HTML (Tailwind + Chart.js). Dark modern finance aesthetic. Auto-refresh. Action buttons.

Internal only. Perfect for VPS team access.
"""

from __future__ import annotations

import json
from datetime import datetime, timezone
from typing import Any, Dict, List

from fastapi import FastAPI, Request
from fastapi.responses import HTMLResponse, JSONResponse
from fastapi.staticfiles import StaticFiles
import uvicorn

from .config import CONFIG
from .db import get_default_db
from .portfolio_allocator import PortfolioAllocator
from .risk_engine import RiskEngine
from .hedge_manager import HedgeManager
from .knowledge_base import get_knowledge_for_prompt  # for potential future "explain" endpoint

app = FastAPI(title="Simple-Trader Internal Dashboard", docs_url=None, redoc_url=None)

# Simple in-memory for demo; in real service share DB/connections
db = get_default_db(CONFIG.database_path, tenant_id=CONFIG.tenant_id)
allocator = PortfolioAllocator(config=CONFIG, db=db)
risk_engine = RiskEngine(config=CONFIG, db=db)
hedge_manager = HedgeManager(config=CONFIG, db=db)

# --- Price helper for live MTM on open positions (beautiful dashboard needs "how much we are making now") ---
try:
    import yfinance as yf
    _HAS_YF = True
except Exception:
    _HAS_YF = False

import time as _time
_price_cache: dict[str, tuple[float, float]] = {}  # sym -> (price, ts)

def _get_current_price(symbol: str) -> float | None:
    """Best-effort current price for MTM. Uses yfinance (GC=F, BTC-USD, etc). Cached 30s. Graceful."""
    sym = (symbol or "").upper()
    now = _time.time()
    if sym in _price_cache:
        p, ts = _price_cache[sym]
        if now - ts < 30:
            return p
    ticker = sym
    if sym in ("GOLD", "XAU", "GC"): ticker = "GC=F"
    elif sym in ("SILVER", "SI"): ticker = "SI=F"
    elif sym in ("OIL", "CL", "CRUDE"): ticker = "CL=F"
    elif sym in ("BTC", "ETH", "SOL"): ticker = f"{sym}-USD"
    elif len(sym) == 6 and sym.endswith("USD"): ticker = sym  # e.g. EURUSD
    elif sym == "FOREX" or sym.startswith("USD"): ticker = "DX=F"  # rough dollar index
    try:
        if _HAS_YF:
            t = yf.Ticker(ticker)
            # fast path
            price = None
            try:
                price = float(t.fast_info.get("last_price") or t.fast_info.get("regular_market_price") or 0)
            except:
                pass
            if not price or price <= 0:
                hist = t.history(period="1d", interval="1m")
                if not hist.empty:
                    price = float(hist["Close"].iloc[-1])
            if price and price > 0:
                _price_cache[sym] = (price, now)
                return price
    except Exception:
        pass
    # Fallback for known: use coingecko simple for crypto if needed (no key)
    if sym in ("BTC", "ETH") and not _HAS_YF:
        try:
            import requests
            r = requests.get(f"https://api.coingecko.com/api/v3/simple/price?ids={ 'bitcoin' if sym=='BTC' else 'ethereum' }&vs_currencies=usd", timeout=4)
            if r.status_code == 200:
                p = r.json().get( 'bitcoin' if sym=='BTC' else 'ethereum', {}).get("usd")
                if p: 
                    _price_cache[sym] = (float(p), now)
                    return float(p)
        except:
            pass
    return None

def _compute_unrealized_pnl(pos: dict) -> float | None:
    """Rough MTM for display. Uses risk_amount as proxy for position risk; scales by price change."""
    try:
        entry = float(pos.get("entry_price") or pos.get("executed_price") or 0)
        if entry <= 0:
            return None
        current = _get_current_price(pos.get("symbol", ""))
        if not current:
            return None
        side_mult = 1 if (pos.get("side", "").lower() == "long") else -1
        # Approximate: price change % * risk (risk is what we stand to lose on stop, but good enough visual)
        pct = (current - entry) / entry * side_mult
        risk = float(pos.get("risk_amount") or 0)
        # Conservative: show est P/L as pct * risk (real position sizing would be better but matches existing risk model)
        return round(pct * risk, 2)
    except:
        return None

# Basic HTML template (self-contained, beautiful modern dark finance UI with Tailwind + Chart.js)
DASHBOARD_HTML = """
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Secret Formula • Internal Dashboard</title>
    <script src="https://cdn.tailwindcss.com"></script>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        @import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600&amp;family=Space+Grotesk:wght@500;600&amp;display=swap');
        :root { --accent: #34d399; }
        body { font-family: 'Inter', system_ui, sans-serif; }
        .font-display { font-family: 'Space Grotesk', 'Inter', sans-serif; }
        .card { 
            background: #0f172a; 
            border: 1px solid #1e2937; 
            transition: transform 0.1s ease, box-shadow 0.1s ease; 
        }
        .card:hover { box-shadow: 0 10px 15px -3px rgb(0 0 0 / 0.1); }
        .metric-big { font-size: 1.85rem; line-height: 1.1; font-weight: 700; font-family: 'Space Grotesk', sans-serif; }
        .section-title { font-size: 0.95rem; letter-spacing: -.015em; font-weight: 600; color: #64748b; }
        .trade-row { transition: background 0.05s; }
        .trade-row:hover { background: #1e2937; }
        .status-badge { font-size: 0.65rem; padding: 1px 8px; border-radius: 9999px; font-weight: 600; letter-spacing: .5px; }
        .pnl-positive { color: #4ade80; font-weight: 600; }
        .pnl-negative { color: #f87171; font-weight: 600; }
        .finance-table th { font-weight: 500; color: #64748b; font-size: 0.75rem; text-transform: uppercase; letter-spacing: .05em; }
        .nav-active { border-bottom: 2px solid #34d399; color: white; }
        .chart-container { position: relative; }
        .metric-label { font-size: 0.7rem; text-transform: uppercase; letter-spacing: .08em; color: #64748b; }
    </style>
</head>
<body class="bg-slate-950 text-slate-200">
    <div class="max-w-[1280px] mx-auto p-5">
        <!-- HEADER -->
        <div class="flex items-center justify-between mb-6">
            <div class="flex items-center gap-x-3">
                <div class="w-9 h-9 bg-emerald-500 rounded-2xl flex items-center justify-center text-slate-950 font-bold text-2xl tracking-tighter">SF</div>
                <div>
                    <div class="font-display text-3xl font-semibold tracking-tighter">Secret Formula</div>
                    <div class="text-[10px] text-emerald-400 -mt-1">INTERNAL • VPS</div>
                </div>
            </div>
            
            <div class="flex items-center gap-x-2">
                <div id="last-updated" class="text-xs text-slate-500 font-mono"></div>
                <button onclick="refreshAll()" 
                        class="px-4 py-1.5 text-sm bg-slate-900 hover:bg-slate-800 border border-slate-700 rounded-2xl flex items-center gap-x-2 text-slate-300 hover:text-white transition">
                    <svg xmlns="http://www.w3.org/2000/svg" class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.058 11H1M12 3v2m0 16v2m9-9H15m-6 0a8 8 0 01-.938-1.5" />
                    </svg>
                    <span>Refresh</span>
                </button>
                <button onclick="runAllocate()" class="px-4 py-1.5 text-sm bg-emerald-600 hover:bg-emerald-500 rounded-2xl text-white font-medium text-sm">Re-allocate</button>
                <button onclick="runHedge()" class="px-4 py-1.5 text-sm bg-amber-600 hover:bg-amber-500 rounded-2xl text-white font-medium text-sm">Hedge</button>
            </div>
        </div>

        <!-- HERO METRICS - How much we are making -->
        <div class="grid grid-cols-2 md:grid-cols-5 gap-3 mb-6">
            <div class="card rounded-3xl p-5">
                <div class="metric-label mb-1">Realized P&amp;L</div>
                <div id="realized-pnl" class="metric-big font-semibold tabular-nums">—</div>
                <div class="text-[10px] text-slate-500 mt-0.5">from closed trades</div>
            </div>
            <div class="card rounded-3xl p-5">
                <div class="metric-label mb-1">Unrealized (MTM)</div>
                <div id="unrealized-pnl" class="metric-big font-semibold tabular-nums">—</div>
                <div class="text-[10px] text-slate-500 mt-0.5">live mark-to-market</div>
            </div>
            <div class="card rounded-3xl p-5 border border-emerald-900/50">
                <div class="metric-label mb-1">Total P&amp;L (est)</div>
                <div id="total-pnl" class="metric-big font-semibold tabular-nums">—</div>
                <div id="total-pnl-sub" class="text-[10px] text-emerald-400 mt-0.5">Realized + Open</div>
            </div>
            <div class="card rounded-3xl p-5">
                <div class="metric-label mb-1">Win Rate</div>
                <div id="win-rate" class="metric-big font-semibold tabular-nums">—</div>
                <div id="trade-count" class="text-[10px] text-slate-500 mt-0.5">closed trades</div>
            </div>
            <div class="card rounded-3xl p-5">
                <div class="metric-label mb-1">Open Positions</div>
                <div id="open-count" class="metric-big font-semibold tabular-nums">—</div>
                <div id="open-risk" class="text-[10px] text-amber-400 mt-0.5">risk at stake</div>
            </div>
        </div>

        <!-- CHARTS: How trades are doing + Allocation -->
        <div class="grid grid-cols-1 lg:grid-cols-5 gap-3 mb-6">
            <!-- Equity Curve - the star "how are our trades doing" -->
            <div class="card rounded-3xl p-5 lg:col-span-3">
                <div class="flex items-center justify-between mb-3">
                    <div>
                        <div class="section-title">EQUITY &amp; CUMULATIVE P&amp;L</div>
                        <div class="text-xs text-slate-400">How our trades are performing over time</div>
                    </div>
                    <div class="text-[10px] px-2 py-0.5 bg-slate-900 rounded-xl text-emerald-400">LIVE</div>
                </div>
                <div class="chart-container h-56">
                    <canvas id="equityChart"></canvas>
                </div>
            </div>

            <!-- Allocation + Risk by Asset -->
            <div class="card rounded-3xl p-5 lg:col-span-2">
                <div class="section-title mb-3">RISK ALLOCATION</div>
                <div class="grid grid-cols-2 gap-4">
                    <div>
                        <canvas id="bucketChart" class="max-h-[138px]"></canvas>
                        <div class="mt-2 text-center text-xs">
                            <span class="text-emerald-400">Core</span> <span id="core-pct" class="font-mono font-semibold"></span><br>
                            <span class="text-amber-400">Alpha</span> <span id="alpha-pct" class="font-mono font-semibold"></span>
                        </div>
                    </div>
                    <div>
                        <div class="text-xs text-slate-400 mb-1.5">Risk by Asset Class</div>
                        <canvas id="assetChart" class="max-h-[130px]"></canvas>
                    </div>
                </div>
            </div>
        </div>

        <!-- Recent P&L bars -->
        <div class="card rounded-3xl p-5 mb-6">
            <div class="section-title mb-2">LATEST TRADES PERFORMANCE</div>
            <div class="chart-container h-40">
                <canvas id="recentPnlChart"></canvas>
            </div>
            <div class="text-[10px] text-slate-500 mt-1">Green = profit, Red = loss. Hover for details.</div>
        </div>

        <!-- TRADE LOG - Open / Closed with status and P/L -->
        <div class="card rounded-3xl p-5 mb-6">
            <div class="flex items-center justify-between mb-3">
                <div class="section-title">LATEST TRADES &amp; POSITIONS</div>
                <div class="flex items-center gap-x-1 text-xs">
                    <button onclick="filterTrades('all')" class="tab-btn px-3 py-1 rounded-2xl bg-slate-800 hover:bg-slate-700 active-tab" id="tab-all">All</button>
                    <button onclick="filterTrades('open')" class="tab-btn px-3 py-1 rounded-2xl bg-slate-800 hover:bg-slate-700" id="tab-open">Open</button>
                    <button onclick="filterTrades('closed')" class="tab-btn px-3 py-1 rounded-2xl bg-slate-800 hover:bg-slate-700" id="tab-closed">Closed</button>
                    <input id="trade-filter" onkeyup="filterTradesTable()" placeholder="Filter symbol..." 
                           class="ml-2 bg-slate-900 border border-slate-700 text-xs px-3 py-1 rounded-2xl w-40 focus:outline-none">
                </div>
            </div>
            
            <div class="overflow-x-auto">
                <table class="w-full finance-table text-sm">
                    <thead>
                        <tr class="border-b border-slate-700">
                            <th class="py-2 text-left">Time</th>
                            <th class="py-2 text-left">Symbol</th>
                            <th class="py-2">Side</th>
                            <th class="py-2 text-right">Entry</th>
                            <th class="py-2 text-right">Current / Exit</th>
                            <th class="py-2">Status</th>
                            <th class="py-2 text-right">P&amp;L (USD)</th>
                            <th class="py-2 text-right">RR</th>
                            <th class="py-2">Bucket</th>
                        </tr>
                    </thead>
                    <tbody id="trades-tbody" class="text-sm divide-y divide-slate-800"></tbody>
                </table>
            </div>
            <div class="text-[10px] text-slate-500 mt-2">Open positions show live MTM estimate. Closed = realized from trade log. Click refresh to update prices.</div>
        </div>

        <div class="grid grid-cols-1 lg:grid-cols-3 gap-3">
            <!-- Book Risk & Safety -->
            <div class="card rounded-3xl p-5">
                <div class="section-title mb-3">BOOK RISK &amp; SAFETY</div>
                <div id="risk-metrics" class="grid grid-cols-2 gap-y-4 text-sm"></div>
                <div id="breaker-status" class="mt-3 text-xs"></div>
            </div>

            <!-- Urgent + Special (Politician Disclosures) -->
            <div class="card rounded-3xl p-5">
                <div class="section-title mb-2">URGENT OPPORTUNITIES / RISKS</div>
                <div id="urgent-list" class="text-xs max-h-[138px] overflow-auto space-y-1.5 font-mono"></div>
                
                <div class="mt-4 pt-3 border-t border-slate-800">
                    <div class="section-title mb-1.5 flex items-center gap-x-2">
                        <span>WHALE &amp; POLITICIAN SIGNALS</span>
                        <span class="text-[9px] px-1.5 py-px bg-violet-900/60 text-violet-300 rounded">PUBLIC</span>
                    </div>
                    <div id="special-list" class="text-xs max-h-[92px] overflow-auto space-y-1"></div>
                </div>
            </div>

            <!-- ML + System -->
            <div class="card rounded-3xl p-5">
                <div class="section-title mb-2">ML LEARNINGS — SYSTEM GETS BETTER</div>
                <div id="ml-list" class="text-xs bg-slate-950 p-3 rounded-2xl font-mono max-h-[148px] overflow-auto"></div>
                <div class="text-[10px] text-emerald-400/70 mt-2">Every loss attributes causes → negative weight → future signals penalized + veto possible. Review with tags to retrain.</div>
            </div>

            <!-- NEW: System Health Indicators -->
            <div class="card rounded-3xl p-5">
                <div class="section-title mb-2">SYSTEM HEALTH INDICATORS</div>
                <div id="health-list" class="text-xs font-mono"></div>
                <div class="text-[10px] text-slate-500 mt-1">Signal rate, cause trends, regret-veto pressure. Rising 'insufficient_hedge' or high veto = tune hedge rules.</div>
            </div>
        </div>

        <div class="mt-6 flex items-center justify-between text-[10px] text-slate-500">
            <div>
                Allocator + RiskEngine + HedgeManager + Knowledge (Buffett/Simons) + Public Disclosures active.<br>
                Paper mode by default • Full audit trail • Run as systemd service.
            </div>
            <div class="text-right">
                <button onclick="runRiskReport()" class="underline hover:no-underline">Risk Report</button> • 
                <button onclick="alert('Use CLI: python main.py ml-status  or  review-mistakes --tag ...')" class="underline hover:no-underline">ML Tools (CLI)</button>
            </div>
        </div>
    </div>

<script>
let bucketChart, assetChart, equityChart, recentPnlChart;
let allTrades = [];

function tailwindInit() {
    // Tailwind script already loaded via CDN
}

async function fetchJSON(url) {
    const r = await fetch(url);
    if (!r.ok) throw new Error('fetch failed');
    return r.json();
}

function formatMoney(n) {
    if (n === null || n === undefined) return '—';
    const sign = n >= 0 ? '' : '-';
    const abs = Math.abs(n);
    return sign + '$' + abs.toLocaleString(undefined, {maximumFractionDigits: 0});
}

function updateLastUpdated() {
    const el = document.getElementById('last-updated');
    if (el) el.textContent = new Date().toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'}) + ' • live';
}

async function refreshAll() {
    updateLastUpdated();
    try {
        const [alloc, risk, perf, positions, urgent, trades, equity] = await Promise.all([
            fetchJSON('/api/allocation'),
            fetchJSON('/api/risk'),
            fetchJSON('/api/performance'),
            fetchJSON('/api/positions'),
            fetchJSON('/api/urgent'),
            fetchJSON('/api/trades?limit=30'),
            fetchJSON('/api/equity_curve')
        ]);

        // Hero metrics
        document.getElementById('realized-pnl').innerHTML = `<span class="${perf.realized_pnl >= 0 ? 'text-emerald-400' : 'text-red-400'}">${formatMoney(perf.realized_pnl)}</span>`;
        document.getElementById('unrealized-pnl').innerHTML = `<span class="${perf.unrealized_pnl_est >= 0 ? 'text-emerald-400' : 'text-red-400'}">${formatMoney(perf.unrealized_pnl_est)}</span>`;
        document.getElementById('total-pnl').innerHTML = `<span class="${perf.total_pnl_est >= 0 ? 'text-emerald-400' : 'text-red-400'}">${formatMoney(perf.total_pnl_est)}</span>`;
        document.getElementById('win-rate').innerHTML = perf.win_rate_pct + '<span class="text-base align-super">%</span>';
        document.getElementById('trade-count').innerHTML = `${perf.num_closed_trades} closed • ${perf.wins}W ${perf.losses}L`;
        document.getElementById('open-count').innerHTML = perf.open_positions;
        document.getElementById('open-risk').innerHTML = formatMoney(perf.open_risk_usd) + ' risk';

        // Charts
        updateBucketChart(alloc);
        updateAssetChart(alloc);
        updateEquityCurve(equity);
        updateRecentPnlChart(trades);

        // Risk
        updateRiskMetrics(risk);

        // Trade log (store + render)
        allTrades = trades || [];
        renderTradesTable(allTrades);

        // Other sections
        updateUrgent(urgent);
        updateSpecial();
        updateML();
        updateHealth();

    } catch (e) {
        console.error('Dashboard refresh error', e);
    }
}

function updateBucketChart(data) {
    const ctx = document.getElementById('bucketChart');
    const core = Math.round((data.core_risk_pct || 0) * 100);
    const alpha = Math.round((data.alpha_risk_pct || 0) * 100);
    document.getElementById('core-pct').innerText = core + '%';
    document.getElementById('alpha-pct').innerText = alpha + '%';

    if (bucketChart) bucketChart.destroy();
    bucketChart = new Chart(ctx, {
        type: 'doughnut',
        data: {
            labels: ['Core (Gold/Silver)', 'Alpha (Crypto/Forex/Oil)'],
            datasets: [{
                data: [core, alpha],
                backgroundColor: ['#10b981', '#f59e0b'],
                borderColor: '#0f172a',
                borderWidth: 3
            }]
        },
        options: { 
            responsive: true, 
            cutout: '68%',
            plugins: { legend: { display: false } } 
        }
    });
}

function updateAssetChart(data) {
    const ctx = document.getElementById('assetChart');
    const per = data.per_asset || {};
    const labels = Object.keys(per);
    const vals = labels.map(l => Math.round((per[l] || 0) * 100));

    if (assetChart) assetChart.destroy();
    assetChart = new Chart(ctx, {
        type: 'bar',
        data: {
            labels: labels,
            datasets: [{ 
                label: '% Risk', 
                data: vals, 
                backgroundColor: '#64748b',
                borderRadius: 4
            }]
        },
        options: { 
            responsive: true, 
            scales: { y: { beginAtZero: true, max: 60, grid: { color: '#1e2937' } }, x: { grid: { color: '#1e2937' } } },
            plugins: { legend: { display: false } }
        }
    });
}

let equityChartInstance = null;
function updateEquityCurve(curveData) {
    const ctx = document.getElementById('equityChart');
    const labels = curveData.map(p => {
        const d = new Date(p.ts);
        return d.toLocaleDateString([], {month:'short', day:'numeric'}) + ' ' + d.toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'});
    });
    const values = curveData.map(p => p.cum_pnl);

    if (equityChartInstance) equityChartInstance.destroy();
    equityChartInstance = new Chart(ctx, {
        type: 'line',
        data: {
            labels: labels,
            datasets: [{
                label: 'Cumulative P&L',
                data: values,
                borderColor: '#34d399',
                borderWidth: 2.5,
                fill: true,
                backgroundColor: 'rgba(52, 211, 153, 0.08)',
                tension: 0.2,
                pointRadius: 0,
                pointHoverRadius: 3
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            scales: {
                x: { grid: { color: '#1e2937' }, ticks: { color: '#64748b', font: { size: 10 } } },
                y: { grid: { color: '#1e2937' }, ticks: { color: '#64748b', font: { size: 10 } } }
            },
            plugins: { legend: { display: false } }
        }
    });
}

function updateRecentPnlChart(trades) {
    const ctx = document.getElementById('recentPnlChart');
    const recent = (trades || []).filter(t => t.pnl != null).slice(0, 12).reverse();
    
    const labels = recent.map(t => t.symbol || '?');
    const data = recent.map(t => t.pnl);
    const colors = data.map(v => v >= 0 ? '#4ade80' : '#f87171');

    if (recentPnlChart) recentPnlChart.destroy();
    recentPnlChart = new Chart(ctx, {
        type: 'bar',
        data: {
            labels: labels,
            datasets: [{ data: data, backgroundColor: colors, borderRadius: 3 }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            scales: { y: { grid: { color: '#1e2937' } }, x: { grid: { display: false } } },
            plugins: { legend: { display: false }, tooltip: { callbacks: { label: (ctx) => '$' + ctx.raw } } }
        }
    });
}

function updateRiskMetrics(risk) {
    const el = document.getElementById('risk-metrics');
    const ddClass = (risk.current_drawdown_pct || 0) > 0.05 ? 'text-red-400' : 'text-emerald-400';
    el.innerHTML = `
        <div><span class="text-slate-400 text-xs">Total Risk</span><div class="font-semibold tabular-nums text-lg">${(risk.total_risk_usd || 0).toLocaleString()}</div></div>
        <div><span class="text-slate-400 text-xs">Notional Exposure</span><div class="font-semibold tabular-nums text-lg">${(risk.total_notional_usd || 0).toLocaleString()}</div></div>
        <div><span class="text-slate-400 text-xs">Current Drawdown</span><div class="font-semibold tabular-nums text-lg ${ddClass}">${((risk.current_drawdown_pct||0)*100).toFixed(1)}%</div></div>
        <div><span class="text-slate-400 text-xs">Open Signals</span><div class="font-semibold tabular-nums text-lg">${risk.open_signals || 0}</div></div>
    `;
    const br = document.getElementById('breaker-status');
    if (risk.breaker_active) {
        br.innerHTML = `<span class="inline-block px-2.5 py-0.5 text-xs rounded-2xl bg-red-950 text-red-400 border border-red-900">⚠ BREAKER: ${risk.breaker_reason || 'active'}</span>`;
    } else {
        br.innerHTML = `<span class="text-emerald-400 text-xs">✓ All limits healthy</span>`;
    }
}

function renderTradesTable(trades) {
    const tbody = document.getElementById('trades-tbody');
    tbody.innerHTML = '';
    
    (trades || []).forEach(trade => {
        const tr = document.createElement('tr');
        tr.className = 'trade-row border-b border-slate-800';
        
        const status = trade.status || 'unknown';
        const isOpen = status === 'open';
        const pnl = trade.pnl;
        const pnlClass = pnl == null ? '' : (pnl >= 0 ? 'pnl-positive' : 'pnl-negative');
        const pnlText = pnl == null ? '—' : formatMoney(pnl);
        
        const statusHTML = isOpen 
            ? `<span class="status-badge bg-amber-400/10 text-amber-400 border border-amber-400/30">OPEN</span>` 
            : `<span class="status-badge bg-emerald-400/10 text-emerald-400 border border-emerald-400/30">CLOSED</span>`;
        
        const currentExit = trade.current_or_exit ? Number(trade.current_or_exit).toFixed(2) : (trade.exit_price ? Number(trade.exit_price).toFixed(2) : '—');
        const entry = trade.entry ? Number(trade.entry).toFixed(2) : '—';
        
        tr.innerHTML = `
            <td class="py-2.5 text-xs font-mono text-slate-400">${(trade.time || '').slice(5,16)}</td>
            <td class="py-2.5 font-semibold">${trade.symbol}</td>
            <td class="py-2.5"><span class="uppercase text-xs px-1.5 py-px rounded ${trade.side === 'long' ? 'bg-emerald-900/60 text-emerald-300' : 'bg-red-900/60 text-red-300'}">${trade.side || ''}</span></td>
            <td class="py-2.5 text-right font-mono text-xs">${entry}</td>
            <td class="py-2.5 text-right font-mono text-xs">${currentExit}</td>
            <td class="py-2.5">${statusHTML}</td>
            <td class="py-2.5 text-right font-semibold tabular-nums ${pnlClass}">${pnlText}</td>
            <td class="py-2.5 text-right font-mono text-xs">${(trade.rr || 0).toFixed(1)}</td>
            <td class="py-2.5"><span class="text-xs px-2 py-0.5 rounded-xl ${trade.bucket === 'CORE' ? 'bg-emerald-900/60 text-emerald-300' : 'bg-amber-900/60 text-amber-300'}">${trade.bucket || '-'}</span></td>
        `;
        tbody.appendChild(tr);
    });
}

function filterTrades(mode) {
    // Highlight active tab
    document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active-tab', 'bg-emerald-700'));
    const active = document.getElementById('tab-' + mode);
    if (active) active.classList.add('active-tab', 'bg-emerald-700');

    let filtered = allTrades;
    if (mode === 'open') filtered = allTrades.filter(t => t.status === 'open');
    if (mode === 'closed') filtered = allTrades.filter(t => t.status === 'closed');
    
    renderTradesTable(filtered);
}

function filterTradesTable() {
    const q = (document.getElementById('trade-filter').value || '').toUpperCase();
    const filtered = allTrades.filter(t => (t.symbol || '').toUpperCase().includes(q));
    renderTradesTable(filtered);
}

async function updateSpecial() {
    try {
        const specials = await fetchJSON('/api/special');
        const el = document.getElementById('special-list');
        if (!el) return;
        if (!specials || !specials.length) {
            el.innerHTML = '<div class="text-slate-500 text-xs">No recent public whale or politician disclosure signals.</div>';
            return;
        }
        el.innerHTML = specials.map(s => {
            const isPol = (s.type || '').includes('POLITICS') || (s.type || '').includes('DISCLOS');
            return `<div class="flex gap-x-2 text-xs">
                <span class="shrink-0 text-violet-400 font-medium">${s.type}</span> 
                <span class="text-slate-300">${s.asset}:</span> 
                <span class="truncate text-slate-400">${s.title}</span>
            </div>`;
        }).join('');
    } catch(e) {}
}

async function updateML() {
    try {
        const ml = await fetchJSON('/api/ml');
        const el = document.getElementById('ml-list');
        let html = '';
        if (ml.cause_weights && Object.keys(ml.cause_weights).length) {
            html += `<div class="mb-1">Active penalties: <span class="font-semibold">${JSON.stringify(ml.cause_weights)}</span></div>`;
        }
        if (ml.regrets && ml.regrets.length) {
            html += ml.regrets.slice(0,2).map(r => `• ${r.outcome} ${r.symbol} <span class="text-red-400">${r.causes}</span>`).join('<br>');
        }
        el.innerHTML = html || '<span class="text-slate-500">No mistakes logged yet — system learning from every outcome.</span>';
    } catch(e) {
        console.debug('ml optional', e);
    }
}

async function updateHealth() {
    try {
        const h = await fetchJSON('/api/system_health');
        const el = document.getElementById('health-list');
        if (!el) return;
        let html = `24h signals: ${h.signals_24h || 0} (Alpha ${h.alpha_signals_24h || 0})<br>`;
        if (h.top_cause_trends) {
            html += `Top causes: ${JSON.stringify(h.top_cause_trends)}<br>`;
        }
        html += `Veto pressure: ${h.regret_veto_pressure || 0}`;
        el.innerHTML = html;
    } catch(e) {
        console.debug('health optional', e);
    }
}

async function updateUrgent(urgent) {
    const el = document.getElementById('urgent-list');
    if (!urgent || !urgent.length) {
        el.innerHTML = '<div class="text-slate-500">No urgent signals in last 24h.</div>';
        return;
    }
    el.innerHTML = urgent.map(u => 
        `<div class="px-2 py-1 rounded-xl bg-slate-900 text-xs border-l-2 border-red-500">${u.time?.slice(5,16)} <strong>${u.type}</strong> ${u.message}</div>`
    ).join('');
}

async function runAllocate() {
    const res = await fetch('/api/run-allocate', {method:'POST'});
    const data = await res.json();
    alert('Allocator: ' + JSON.stringify(data.suggestions?.slice(0,2) || data));
    refreshAll();
}

async function runHedge() {
    const res = await fetch('/api/risk');
    const data = await res.json();
    alert('Current book risk: ' + JSON.stringify({total: data.total_risk_usd, dd: data.current_drawdown_pct}));
    // In full system we would call hedge suggestions here
    refreshAll();
}

async function runRiskReport() {
    const res = await fetch('/api/risk');
    const data = await res.json();
    alert('Risk Report:\\n' + JSON.stringify(data, null, 2));
    refreshAll();
}

// Boot
tailwindInit();
setInterval(refreshAll, 28000);
window.onload = () => {
    refreshAll();
    // initial tab highlight
    const allTab = document.getElementById('tab-all');
    if (allTab) allTab.classList.add('bg-emerald-700', 'active-tab');
};
</script>
</body>
</html>
"""

@app.get("/", response_class=HTMLResponse)
async def dashboard():
    return HTMLResponse(DASHBOARD_HTML)

@app.get("/api/allocation")
async def api_allocation():
    # Pull live from allocator + risk
    book = risk_engine.compute_book_risk()
    total = max(book.total_risk_usd, 1.0)
    
    # Approximate Core/Alpha split using asset classes
    core_risk = book.per_class_risk.get("GOLD", 0) + book.per_class_risk.get("SILVER", 0)
    alpha_risk = book.total_risk_usd - core_risk
    
    per_asset = {k: v/total for k,v in book.per_class_risk.items()}
    
    return {
        "core_risk_pct": core_risk / total if total > 0 else CONFIG.core_bucket_target_pct,
        "alpha_risk_pct": alpha_risk / total if total > 0 else CONFIG.alpha_bucket_target_pct,
        "per_asset": per_asset,
        "total_risk_usd": book.total_risk_usd,
    }

@app.get("/api/risk")
async def api_risk():
    book = risk_engine.compute_book_risk()
    open_count = len(db.get_open_signals(limit=1000)) if hasattr(db, 'get_open_signals') else 0
    return {
        "total_risk_usd": book.total_risk_usd,
        "total_notional_usd": book.total_notional_usd,
        "current_drawdown_pct": book.current_drawdown_pct,
        "breaker_active": book.breaker_active,
        "breaker_reason": book.breaker_reason,
        "open_signals": open_count,
        "per_class": book.per_class_risk,
    }

@app.get("/api/positions")
async def api_positions():
    rows = db.execute_custom(
        "SELECT id, symbol, side, risk_amount, rr, bucket, decision_audit, entry_price, created_at FROM signals WHERE status = 'open' ORDER BY created_at DESC LIMIT 20"
    )
    out = []
    for r in (rows or []):
        d = dict(r)
        d["current_price"] = _get_current_price(d.get("symbol"))
        d["unrealized_pnl_est"] = _compute_unrealized_pnl(d)
        out.append(d)
    return out

@app.get("/api/urgent")
async def api_urgent():
    # Pull recent urgent events. In real system we would have a dedicated urgent_events table.
    # For now, synthesize from recent high-impact signals or use a simple log.
    # Placeholder: return recent "strong" signals as opportunities.
    rows = db.execute_custom(
        "SELECT symbol, side, created_at, decision_audit FROM signals WHERE status='open' AND created_at > datetime('now', '-24 hours') ORDER BY created_at DESC LIMIT 10"
    )
    events = []
    for r in (rows or []):
        try:
            audit = json.loads(r.get("decision_audit") or "{}")
            if audit.get("score", 0) > 0.75 or "strong" in str(audit).lower():
                events.append({
                    "time": r["created_at"],
                    "type": "OPPORTUNITY",
                    "message": f"High conviction {r['side']} {r['symbol']} (score high). Consider sizing per allocator."
                })
        except:
            pass
    return events

@app.get("/api/special")
async def api_special():
    """Recent whale and politics signals (free on-chain + public disclosures)."""
    rows = db.execute_custom(
        "SELECT title, content, asset, provider, created_at FROM news "
        "WHERE provider IN ('whale', 'whale_btc', 'whale_eth', 'politics', 'politics_disclosure', 'politician_disclosure_public') "
        "AND created_at > datetime('now', '-48 hours') ORDER BY created_at DESC LIMIT 10"
    )
    specials = []
    for r in (rows or []):
        specials.append({
            "time": r.get("created_at"),
            "type": r.get("provider", "special").upper(),
            "asset": r.get("asset"),
            "title": r.get("title", "")[:80],
            "content": (r.get("content") or "")[:120],
        })
    return specials

@app.get("/api/ml")
async def api_ml():
    """ML status for dashboard: regrets, cause weights (the small persisted mapping), scorer health. Shows the 'gets better' loop."""
    regrets = db.get_regrets(limit=8) if hasattr(db, "get_regrets") else []
    cw = db.get_cause_weights() if hasattr(db, "get_cause_weights") else {}
    # Simple scorer status
    scorer_info = {"sklearn": "unknown", "trained": False, "cause_persister_loaded": False}
    try:
        # Best effort: if signal_manager in scope or via orchestrator not here, just report DB side
        scorer_info = {
            "sklearn": "avail-if-partial_fit-works",
            "trained": len(cw) > 0,
            "cause_persister_loaded": bool(cw),
            "note": "Cause weights from regret_table outcomes + review tags drive future scoring and vetoes (Simons online + Buffett avoid-repeat)."
        }
    except:
        pass
    return {
        "regrets": regrets,
        "cause_weights": cw,
        "scorer": scorer_info,
        "explanation": "Every loss/timeout attributes causes from decision_audit (e.g. insufficient_hedge on a POLITICS disclosure signal). Weights persist, penalize future similar signals, auto-retrain on tags, regret veto in gate. System improves."
    }

@app.get("/api/performance")
async def api_performance():
    """Overall P&L, win stats, how the book is performing."""
    # Realized from trades
    closed_rows = db.execute_custom(
        "SELECT pnl, outcome FROM trades WHERE pnl IS NOT NULL"
    ) or []
    realized = 0.0
    wins = 0
    losses = 0
    for r in closed_rows:
        p = float(r["pnl"] or 0)
        realized += p
        if p > 0 or (r.get("outcome") or "").lower() == "win":
            wins += 1
        else:
            losses += 1
    total_closed = wins + losses
    win_rate = (wins / total_closed * 100) if total_closed > 0 else 0.0

    # Open positions count + rough sum risk
    open_rows = db.execute_custom("SELECT risk_amount FROM signals WHERE status='open'") or []
    open_count = len(open_rows)
    open_risk = sum(float(r["risk_amount"] or 0) for r in open_rows)

    # Est unrealized (best effort)
    unrealized = 0.0
    for r in (db.execute_custom("SELECT symbol, side, risk_amount, entry_price FROM signals WHERE status='open'") or []):
        est = _compute_unrealized_pnl(dict(r))
        if est is not None:
            unrealized += est

    total_pnl = realized + unrealized

    return {
        "realized_pnl": round(realized, 2),
        "unrealized_pnl_est": round(unrealized, 2),
        "total_pnl_est": round(total_pnl, 2),
        "num_closed_trades": total_closed,
        "wins": wins,
        "losses": losses,
        "win_rate_pct": round(win_rate, 1),
        "open_positions": open_count,
        "open_risk_usd": round(open_risk, 2),
        "note": "Unrealized is mark-to-market estimate using live prices (yfinance/coingecko). Realized from executed trades table."
    }

@app.get("/api/trades")
async def api_trades(limit: int = 25, status: str = "all"):
    """Latest trades/positions for the beautiful trade log: open + closed with P/L and status."""
    items = []

    # Closed / realized trades
    if status in ("all", "closed"):
        closed = db.execute_custom(
            "SELECT t.id, t.signal_id, t.executed_at as time, t.executed_price as entry, t.exit_price as exit_price, "
            "t.pnl, t.outcome, t.notes, s.symbol, s.side, s.bucket, s.rr "
            "FROM trades t LEFT JOIN signals s ON t.signal_id = s.id "
            "WHERE t.pnl IS NOT NULL ORDER BY t.exit_at DESC, t.executed_at DESC LIMIT ?",
            (limit,)
        ) or []
        for r in closed:
            d = dict(r)
            d["status"] = "closed"
            d["pnl"] = d.get("pnl")
            d["current_or_exit"] = d.get("exit_price")
            items.append(d)

    # Open positions
    if status in ("all", "open"):
        opens = db.execute_custom(
            "SELECT id as signal_id, symbol, side, entry_price as entry, risk_amount, rr, bucket, decision_audit, created_at as time "
            "FROM signals WHERE status='open' ORDER BY created_at DESC LIMIT ?",
            (limit,)
        ) or []
        for r in opens:
            d = dict(r)
            d["status"] = "open"
            d["pnl"] = None  # will be est below
            d["exit_price"] = None
            d["current_or_exit"] = _get_current_price(d.get("symbol"))
            est = _compute_unrealized_pnl(d)
            d["unrealized_pnl_est"] = est
            # rough "pnl" for display
            d["pnl"] = est
            items.append(d)

    # Sort newest first, trim
    items.sort(key=lambda x: str(x.get("time") or ""), reverse=True)
    items = items[:limit]

    # Enrich a bit
    for it in items:
        it["symbol"] = it.get("symbol") or "?"
        it["side"] = (it.get("side") or "long").lower()
        if it.get("pnl") is not None:
            it["pnl"] = round(float(it["pnl"]), 2)

    return items

@app.get("/api/equity_curve")
async def api_equity_curve(limit: int = 60):
    """Cumulative P&L over time for the main 'how are our trades doing' chart. From closed trades + latest MTM."""
    rows = db.execute_custom(
        "SELECT COALESCE(exit_at, executed_at) as ts, pnl FROM trades WHERE pnl IS NOT NULL ORDER BY ts ASC"
    ) or []
    curve = []
    cum = 0.0
    for r in rows[-limit:]:
        cum += float(r["pnl"] or 0)
        curve.append({"ts": r["ts"], "cum_pnl": round(cum, 2)})

    # Append a live point if we have open MTM
    if curve:
        now_iso = datetime.now(timezone.utc).isoformat()
        # quick est total unrealized
        unrl = 0.0
        for r in (db.execute_custom("SELECT symbol, side, risk_amount, entry_price FROM signals WHERE status='open'") or []):
            e = _compute_unrealized_pnl(dict(r))
            if e: unrl += e
        curve.append({"ts": now_iso, "cum_pnl": round(cum + unrl, 2), "live": True})

    return curve

@app.post("/api/run-allocate")
async def api_run_allocate():
    # Trigger allocator suggestions (could be used by UI button)
    rows = db.execute_custom("SELECT symbol, risk_amount FROM signals WHERE status='open'")
    current = {}
    for r in (rows or []):
        current[r["symbol"]] = current.get(r["symbol"], 0.0) + float(r.get("risk_amount") or 0)
    suggestions = allocator.suggest_rebalance(current)
    return {"suggestions": [s.__dict__ for s in suggestions]}

# Health
@app.get("/health")
async def health():
    return {"status": "ok", "time": datetime.now(timezone.utc).isoformat()}

@app.get("/api/system_health")
async def api_system_health():
    """Lightweight system health 'indicators' for ops/monitoring (signal rate, cause trends, veto pressure)."""
    try:
        # Recent signals
        sigs = db.execute_custom("SELECT created_at, bucket FROM signals WHERE created_at > datetime('now', '-24 hours')") or []
        alpha_24h = sum(1 for s in sigs if (s.get("bucket") or "").upper() == "ALPHA")
        core_24h = len(sigs) - alpha_24h

        # Cause trends from regrets
        regrets = db.get_regrets(limit=50) if hasattr(db, "get_regrets") else []
        cause_trend = {}
        veto_pressure = 0
        for r in regrets:
            try:
                cs = json.loads(r.get("causes", "[]") or "[]")
                for c in cs:
                    cause_trend[c] = cause_trend.get(c, 0) + 1
                if len(cs) >= 2:
                    veto_pressure += 1
            except:
                pass

        # Rough veto rate
        total_recent = len(regrets)
        veto_rate = (veto_pressure / max(1, total_recent)) if total_recent else 0.0

        return {
            "signals_24h": len(sigs),
            "alpha_signals_24h": alpha_24h,
            "core_signals_24h": core_24h,
            "top_cause_trends": dict(sorted(cause_trend.items(), key=lambda x: -x[1])[:5]),
            "regret_veto_pressure": round(veto_rate, 3),
            "note": "High veto_pressure or rising 'insufficient_hedge' trend = review sizing / hedge rules.",
        }
    except Exception as e:
        return {"error": str(e)}

if __name__ == "__main__":
    # For direct run: uvicorn simple_trader.web_dashboard:app --host 0.0.0.0 --port 8080
    uvicorn.run(app, host="0.0.0.0", port=8080)
