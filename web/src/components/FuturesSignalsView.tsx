import React, { useState, useEffect } from 'react';
import { FuturesTradeSignal } from '../types';
import { Zap, TrendingUp, TrendingDown, ShieldAlert, Target, DollarSign, Clock, RefreshCw, XCircle } from 'lucide-react';

interface FuturesSignalsViewProps {
  apiBaseUrl?: string;
  currentPrice?: number;
}

export const FuturesSignalsView: React.FC<FuturesSignalsViewProps> = ({ apiBaseUrl = '', currentPrice = 0 }) => {
  const [signals, setSignals] = useState<FuturesTradeSignal[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [evaluating, setEvaluating] = useState<boolean>(false);
  const [evaluatingAll, setEvaluatingAll] = useState<boolean>(false);
  const [selectedSymbol, setSelectedSymbol] = useState<string>('BTC/USDT');
  const [statusFilter, setStatusFilter] = useState<'ACTIVE' | 'CLOSED'>('ACTIVE');
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  const fetchSignals = async () => {
    try {
      setLoading(true);
      setErrorMsg(null);
      const res = await fetch(`${apiBaseUrl}/api/v1/signals/futures?status=${statusFilter}&limit=30`);
      if (!res.ok) {
        throw new Error(`HTTP ${res.status}: Failed to fetch signals`);
      }
      const data = await res.json();
      setSignals(Array.isArray(data) ? data : []);
    } catch (err: any) {
      setErrorMsg(err.message || 'Error loading signals');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchSignals();
  }, [statusFilter]);

  const handleEvaluate = async () => {
    try {
      setEvaluating(true);
      setErrorMsg(null);
      const res = await fetch(`${apiBaseUrl}/api/v1/signals/futures/decide`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          symbol: selectedSymbol,
          bucket: 'ALPHA',
        }),
      });
      const data = await res.json();
      if (data.status === 'HOLD') {
        alert(`AI Decision: HOLD for ${selectedSymbol}\n\n${data.message}`);
      } else if (data.id) {
        fetchSignals();
      }
    } catch (err: any) {
      setErrorMsg(err.message || 'Failed to trigger signal evaluation');
    } finally {
      setEvaluating(false);
    }
  };

  const handleEvaluateAll = async () => {
    try {
      setEvaluatingAll(true);
      setErrorMsg(null);
      const res = await fetch(`${apiBaseUrl}/api/v1/signals/futures/decide-all`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ bucket: 'ALL' }),
      });
      const data = await res.json();
      fetchSignals();
      if (data.signals_count > 0) {
        alert(`Scan Complete: Evaluated ${data.scanned_count} assets simultaneously. Generated ${data.signals_count} actionable signals!`);
      } else {
        alert(`Scan Complete: Evaluated ${data.scanned_count} assets simultaneously. No breaking catalyst found (HOLD).`);
      }
    } catch (err: any) {
      setErrorMsg(err.message || 'Failed to trigger batch evaluation');
    } finally {
      setEvaluatingAll(false);
    }
  };

  const handleCloseSignal = async (id: number, symbol: string) => {
    if (!confirm(`Are you sure you want to close signal #${id} (${symbol}) at market price?`)) return;
    try {
      const exitPrice = (selectedSymbol === symbol && currentPrice > 0) ? currentPrice : 0;
      const res = await fetch(`${apiBaseUrl}/api/v1/signals/futures/${id}/close`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          exit_price: exitPrice,
          exit_reason: 'MANUAL_CLOSE',
        }),
      });
      if (!res.ok) throw new Error('Failed to close signal');
      fetchSignals();
    } catch (err: any) {
      alert(`Close failed: ${err.message}`);
    }
  };

  return (
    <div className="space-y-6">
      {/* Header Controls & Catalyst Evaluation Trigger */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 p-4 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 shadow-sm">
        <div>
          <h2 className="text-base font-bold flex items-center gap-2">
            <Zap className="w-5 h-5 text-amber-500" />
            <span>Two-Sided Futures Trade Signals</span>
          </h2>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
            Strict News-First Catalysts • Isolated Leverage (1x–10x) • Max 2% Single-Trade Equity Risk • Min 1:1.5 R:R
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2 w-full sm:w-auto">
          <select
            value={selectedSymbol}
            onChange={(e) => setSelectedSymbol(e.target.value)}
            className="px-3 py-1.5 rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800 text-xs font-mono font-medium focus:outline-none focus:ring-2 focus:ring-sky-500"
          >
            <option value="BTC/USDT">BTC/USDT</option>
            <option value="ETH/USDT">ETH/USDT</option>
            <option value="SOL/USDT">SOL/USDT</option>
            <option value="AVAX/USDT">AVAX/USDT</option>
            <option value="DOGE/USDT">DOGE/USDT</option>
            <option value="SUI/USDT">SUI/USDT</option>
            <option value="PAXG/USDT">PAXG/USDT (Tokenized Gold)</option>
            <option value="XAU/USDT">XAU/USDT (Gold Futures)</option>
            <option value="XAG/USDT">XAG/USDT (Silver Futures)</option>
            <option value="BNB/USDT">BNB/USDT</option>
            <option value="XRP/USDT">XRP/USDT</option>
            <option value="LINK/USDT">LINK/USDT</option>
          </select>

          <button
            onClick={handleEvaluateAll}
            disabled={evaluatingAll || evaluating}
            className="px-3.5 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white text-xs font-semibold shadow-sm flex items-center gap-1.5 transition-all disabled:opacity-50 font-mono"
            title="Scan breaking news catalysts across all crypto assets simultaneously"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${evaluatingAll ? 'animate-spin' : ''}`} />
            <span>{evaluatingAll ? 'Scanning All Universe...' : 'Scan All Assets'}</span>
          </button>

          <button
            onClick={handleEvaluate}
            disabled={evaluating || evaluatingAll}
            className="px-3.5 py-1.5 rounded-lg bg-gradient-to-r from-amber-500 to-orange-600 hover:from-amber-600 hover:to-orange-700 text-white text-xs font-semibold shadow-sm flex items-center gap-1.5 transition-all disabled:opacity-50"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${evaluating ? 'animate-spin' : ''}`} />
            <span>{evaluating ? 'Evaluating...' : `Scan ${selectedSymbol}`}</span>
          </button>
        </div>
      </div>

      {/* Filter Tabs */}
      <div className="flex items-center space-x-2 border-b border-slate-200 dark:border-slate-800 pb-2">
        <button
          onClick={() => setStatusFilter('ACTIVE')}
          className={`px-3 py-1.5 rounded-lg text-xs font-semibold font-mono transition-colors ${
            statusFilter === 'ACTIVE'
              ? 'bg-sky-500/10 text-sky-600 dark:text-sky-400 border border-sky-500/20'
              : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'
          }`}
        >
          Active Signals ({statusFilter === 'ACTIVE' ? signals.length : '•'})
        </button>
        <button
          onClick={() => setStatusFilter('CLOSED')}
          className={`px-3 py-1.5 rounded-lg text-xs font-semibold font-mono transition-colors ${
            statusFilter === 'CLOSED'
              ? 'bg-sky-500/10 text-sky-600 dark:text-sky-400 border border-sky-500/20'
              : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'
          }`}
        >
          Closed History ({statusFilter === 'CLOSED' ? signals.length : '•'})
        </button>
      </div>

      {errorMsg && (
        <div className="p-3 rounded-lg bg-rose-500/10 border border-rose-500/20 text-rose-600 dark:text-rose-400 text-xs font-mono">
          {errorMsg}
        </div>
      )}

      {/* Signals Cards Grid */}
      {loading ? (
        <div className="p-8 text-center text-xs font-mono text-slate-500">
          Loading institutional futures trade signals...
        </div>
      ) : signals.length === 0 ? (
        <div className="p-8 text-center rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 text-slate-400 text-xs font-mono">
          No {statusFilter.toLowerCase()} futures trade signals recorded. Click "Analyze Breaking News & Signals" to evaluate market catalysts.
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {signals.map((sig) => {
            const isLong = sig.direction === 'LONG';
            return (
              <div
                key={sig.id}
                className="p-4 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 shadow-sm flex flex-col justify-between space-y-4 relative overflow-hidden"
              >
                {/* Direction Badge Ribbon */}
                <div
                  className={`absolute top-0 right-0 px-3 py-1 rounded-bl-lg text-[10px] font-mono font-bold tracking-wider uppercase flex items-center gap-1 ${
                    isLong
                      ? 'bg-emerald-500/15 text-emerald-500 border-b border-l border-emerald-500/20'
                      : 'bg-rose-500/15 text-rose-500 border-b border-l border-rose-500/20'
                  }`}
                >
                  {isLong ? <TrendingUp className="w-3 h-3" /> : <TrendingDown className="w-3 h-3" />}
                  <span>{sig.direction} {sig.leverage}x</span>
                </div>

                <div>
                  <div className="flex items-center gap-2">
                    <span className="font-bold text-sm tracking-tight">{sig.symbol}</span>
                    <span className="text-[10px] px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-slate-500 font-mono">
                      #{sig.id}
                    </span>
                  </div>

                  {/* News Catalyst Priority Box */}
                  <div className="mt-2.5 p-2.5 rounded-lg bg-slate-50 dark:bg-slate-800/60 border border-slate-200/60 dark:border-slate-700/60 text-xs">
                    <div className="flex items-center gap-1 text-[10px] font-semibold text-slate-400 uppercase tracking-wider mb-1">
                      <Zap className="w-3 h-3 text-amber-500" />
                      <span>Primary News Catalyst</span>
                    </div>
                    <p className="text-slate-700 dark:text-slate-300 font-medium text-xs leading-relaxed italic">
                      "{sig.catalyst_headline}"
                    </p>
                  </div>
                </div>

                {/* Pricing & Protective Bounds */}
                <div className="grid grid-cols-3 gap-2 py-2 border-y border-slate-100 dark:border-slate-800/80 text-xs font-mono">
                  <div>
                    <span className="text-[10px] text-slate-400 block">Entry</span>
                    <span className="font-semibold">${sig.entry_price.toLocaleString()}</span>
                  </div>
                  <div>
                    <span className="text-[10px] text-rose-400 flex items-center gap-0.5">
                      <ShieldAlert className="w-2.5 h-2.5" /> SL
                    </span>
                    <span className="font-semibold text-rose-500">${sig.stop_loss.toLocaleString()}</span>
                  </div>
                  <div>
                    <span className="text-[10px] text-emerald-400 flex items-center gap-0.5">
                      <Target className="w-2.5 h-2.5" /> TP
                    </span>
                    <span className="font-semibold text-emerald-500">${sig.take_profit_1.toLocaleString()}</span>
                  </div>
                </div>

                {/* Capital Allocation & Risk-Reward */}
                <div className="space-y-1.5 text-xs font-mono">
                  <div className="flex items-center justify-between text-slate-500 dark:text-slate-400">
                    <span className="flex items-center gap-1">
                      <DollarSign className="w-3 h-3 text-sky-400" /> Capital Allocated:
                    </span>
                    <span className="font-semibold text-slate-900 dark:text-slate-100">
                      ${sig.allocated_capital_usd.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })} ({sig.allocated_capital_pct.toFixed(2)}%)
                    </span>
                  </div>
                  <div className="flex items-center justify-between text-slate-500 dark:text-slate-400">
                    <span>Risk-to-Reward:</span>
                    <span className="font-semibold text-amber-500 font-bold">
                      1 : {sig.risk_reward_ratio.toFixed(2)}
                    </span>
                  </div>

                  {sig.status === 'CLOSED' && (
                    <div className="mt-2 pt-2 border-t border-slate-100 dark:border-slate-800">
                      <div className="flex items-center justify-between">
                        <span className="text-slate-400">Exit Price:</span>
                        <span className="font-semibold">${sig.exit_price?.toLocaleString()}</span>
                      </div>
                      <div className="flex items-center justify-between mt-1">
                        <span className="text-slate-400">Realized ROI:</span>
                        <span
                          className={`font-bold ${
                            (sig.realized_roi_pct || 0) >= 0 ? 'text-emerald-500' : 'text-rose-500'
                          }`}
                        >
                          {sig.realized_roi_pct && sig.realized_roi_pct > 0 ? '+' : ''}
                          {sig.realized_roi_pct?.toFixed(2)}% (${sig.realized_pnl_usd?.toFixed(2)})
                        </span>
                      </div>
                    </div>
                  )}
                </div>

                {/* Footer Actions / Timestamps */}
                <div className="pt-2 flex items-center justify-between border-t border-slate-100 dark:border-slate-800/80">
                  <span className="text-[10px] text-slate-400 font-mono flex items-center gap-1">
                    <Clock className="w-3 h-3" />
                    {new Date(sig.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                  </span>

                  {sig.status === 'ACTIVE' && (
                    <button
                      onClick={() => handleCloseSignal(sig.id, sig.symbol)}
                      className="px-2.5 py-1 rounded bg-rose-500/10 hover:bg-rose-500/20 text-rose-500 text-[11px] font-semibold transition-colors flex items-center gap-1"
                    >
                      <XCircle className="w-3 h-3" />
                      <span>Manual Exit</span>
                    </button>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
};
