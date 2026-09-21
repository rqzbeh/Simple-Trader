import React, { useState, useEffect } from 'react';
import { FuturesTradeSignal } from '../types';
import {
  Zap,
  TrendingUp,
  TrendingDown,
  ShieldAlert,
  Target,
  DollarSign,
  RefreshCw,
  XCircle,
  Sparkles,
} from 'lucide-react';

interface AISignalFeedProps {
  selectedSymbol?: string;
  apiBaseUrl?: string;
  currentPrice?: number;
}

export const AISignalFeed: React.FC<AISignalFeedProps> = ({
  selectedSymbol = 'BTC/USDT',
  apiBaseUrl = '',
  currentPrice = 0,
}) => {
  const [signals, setSignals] = useState<FuturesTradeSignal[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [evaluating, setEvaluating] = useState<boolean>(false);
  const [statusFilter, setStatusFilter] = useState<'ACTIVE' | 'CLOSED'>('ACTIVE');
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  const fetchSignals = async () => {
    try {
      setLoading(true);
      setErrorMsg(null);
      const res = await fetch(`${apiBaseUrl}/api/v1/signals/futures?status=${statusFilter}&limit=20`);
      if (!res.ok) throw new Error(`HTTP ${res.status}: Failed to load signals`);
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
      setErrorMsg(err.message || 'Failed to trigger evaluation');
    } finally {
      setEvaluating(false);
    }
  };

  const handleCloseSignal = async (id: number, exitPrice: number = 0) => {
    if (!confirm(`Confirm market close for signal #${id}?`)) return;
    try {
      const res = await fetch(`${apiBaseUrl}/api/v1/signals/futures/${id}/close`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          exit_price: exitPrice > 0 ? exitPrice : 0,
          exit_reason: 'MANUAL_CLOSE',
        }),
      });
      if (!res.ok) throw new Error('Close failed');
      fetchSignals();
    } catch (err: any) {
      alert(`Close failed: ${err.message}`);
    }
  };

  return (
    <div className="rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 p-4 shadow-sm flex flex-col h-full space-y-3">
      {/* Header with Title & Action */}
      <div className="flex items-center justify-between pb-2.5 border-b border-slate-100 dark:border-slate-800">
        <div className="flex items-center space-x-2">
          <Zap className="w-4 h-4 text-amber-500" />
          <h3 className="font-semibold text-sm tracking-tight text-slate-800 dark:text-slate-100">
            Actionable Trade Signals
          </h3>
        </div>

        <button
          onClick={handleEvaluate}
          disabled={evaluating}
          className="px-2.5 py-1 rounded-lg bg-amber-500/10 hover:bg-amber-500/20 text-amber-600 dark:text-amber-400 border border-amber-500/30 text-xs font-semibold flex items-center gap-1 transition-all disabled:opacity-50"
          title={`Scan breaking news & generate signal for ${selectedSymbol}`}
        >
          <RefreshCw className={`w-3 h-3 ${evaluating ? 'animate-spin' : ''}`} />
          <span className="hidden sm:inline font-mono">
            {evaluating ? 'Analyzing...' : `Scan ${selectedSymbol}`}
          </span>
        </button>
      </div>

      {/* Filter Tabs & Counter */}
      <div className="flex items-center justify-between text-xs font-mono">
        <div className="flex items-center space-x-1 bg-slate-100 dark:bg-slate-800/80 p-0.5 rounded-lg">
          <button
            onClick={() => setStatusFilter('ACTIVE')}
            className={`px-2.5 py-1 rounded-md text-[11px] font-semibold transition-colors ${
              statusFilter === 'ACTIVE'
                ? 'bg-white dark:bg-slate-900 text-sky-600 dark:text-sky-400 shadow-xs'
                : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'
            }`}
          >
            Active ({statusFilter === 'ACTIVE' ? signals.length : '•'})
          </button>
          <button
            onClick={() => setStatusFilter('CLOSED')}
            className={`px-2.5 py-1 rounded-md text-[11px] font-semibold transition-colors ${
              statusFilter === 'CLOSED'
                ? 'bg-white dark:bg-slate-900 text-sky-600 dark:text-sky-400 shadow-xs'
                : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'
            }`}
          >
            History ({statusFilter === 'CLOSED' ? signals.length : '•'})
          </button>
        </div>

        <span className="text-[10px] text-slate-400 flex items-center gap-1 font-mono">
          <Sparkles className="w-3 h-3 text-indigo-400" />
          News Catalyst Driven
        </span>
      </div>

      {errorMsg && (
        <div className="p-2 rounded-lg bg-rose-500/10 border border-rose-500/20 text-rose-500 text-xs font-mono">
          {errorMsg}
        </div>
      )}

      {/* Signals List */}
      <div className="space-y-3 overflow-y-auto max-h-[380px] pr-1">
        {loading ? (
          <div className="p-6 text-center text-xs font-mono text-slate-400">
            Loading trade signals...
          </div>
        ) : signals.length === 0 ? (
          <div className="p-6 text-center text-xs font-mono text-slate-400 rounded-lg border border-dashed border-slate-200 dark:border-slate-800">
            No {statusFilter.toLowerCase()} signals. Click "Scan {selectedSymbol}" to evaluate breaking news catalysts.
          </div>
        ) : (
          signals.map((sig) => {
            const isLong = sig.direction === 'LONG';
            return (
              <div
                key={sig.id}
                className="p-3 rounded-lg border border-slate-200 dark:border-slate-800 bg-slate-50/50 dark:bg-slate-800/30 text-xs space-y-2 relative overflow-hidden"
              >
                {/* Header: Symbol & Direction Badge */}
                <div className="flex items-center justify-between">
                  <div className="flex items-center space-x-2">
                    <span className="font-mono font-bold text-slate-900 dark:text-slate-100 text-sm">
                      {sig.symbol}
                    </span>
                    <span
                      className={`px-2 py-0.5 rounded font-bold text-[10px] flex items-center space-x-1 ${
                        isLong
                          ? 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20'
                          : 'bg-rose-500/15 text-rose-600 dark:text-rose-400 border border-rose-500/20'
                      }`}
                    >
                      {isLong ? (
                        <TrendingUp className="w-3 h-3 mr-0.5" />
                      ) : (
                        <TrendingDown className="w-3 h-3 mr-0.5" />
                      )}
                      <span>
                        {sig.direction} {sig.leverage}x
                      </span>
                    </span>
                  </div>

                  <span className="text-[10px] text-slate-400 font-mono">
                    #{sig.id} • {new Date(sig.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                  </span>
                </div>

                {/* News Catalyst Priority Box */}
                {sig.catalyst_headline && (
                  <div className="p-2 rounded bg-amber-500/5 dark:bg-amber-500/10 border border-amber-500/20 text-xs space-y-1">
                    <div className="text-[10px] font-semibold text-amber-500 uppercase flex items-center gap-1">
                      <Zap className="w-3 h-3" /> Breaking Catalyst
                      {sig.catalyst_source && <span className="text-slate-400">({sig.catalyst_source})</span>}
                    </div>
                    <p className="text-slate-700 dark:text-slate-200 italic font-medium leading-tight">
                      "{sig.catalyst_headline}"
                    </p>
                  </div>
                )}

                {/* Key Execution Levels: Entry, SL, TP */}
                <div className="grid grid-cols-3 gap-1 py-1.5 border-y border-slate-200/60 dark:border-slate-800 font-mono text-[11px]">
                  <div>
                    <span className="text-[10px] text-slate-400 block">Entry</span>
                    <span className="font-semibold text-slate-800 dark:text-slate-200">
                      ${sig.entry_price.toLocaleString()}
                    </span>
                  </div>
                  <div>
                    <span className="text-[10px] text-rose-400 flex items-center gap-0.5">
                      <ShieldAlert className="w-2.5 h-2.5" /> SL
                    </span>
                    <span className="font-semibold text-rose-500">
                      ${sig.stop_loss.toLocaleString()}
                    </span>
                  </div>
                  <div>
                    <span className="text-[10px] text-emerald-400 flex items-center gap-0.5">
                      <Target className="w-2.5 h-2.5" /> TP
                    </span>
                    <span className="font-semibold text-emerald-500">
                      ${sig.take_profit_1.toLocaleString()}
                    </span>
                  </div>
                </div>

                {/* Risk Parameters & Capital Sizing */}
                <div className="flex items-center justify-between text-[11px] font-mono text-slate-500 dark:text-slate-400">
                  <span className="flex items-center gap-1">
                    <DollarSign className="w-3 h-3 text-sky-400" />
                    <span>
                      ${sig.allocated_capital_usd?.toFixed(0)} ({sig.allocated_capital_pct?.toFixed(1)}%)
                    </span>
                  </span>
                  <span>
                    R:R <strong className="text-amber-500">1:{sig.risk_reward_ratio?.toFixed(2)}</strong>
                  </span>
                </div>

                {/* Closed Trade Result */}
                {sig.status === 'CLOSED' && (
                  <div className="p-2 rounded bg-slate-100 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700 text-xs font-mono flex items-center justify-between">
                    <span className="text-slate-400">Realized ROI:</span>
                    <span
                      className={`font-bold ${
                        (sig.realized_roi_pct || 0) >= 0 ? 'text-emerald-500' : 'text-rose-500'
                      }`}
                    >
                      {(sig.realized_roi_pct || 0) >= 0 ? '+' : ''}
                      {sig.realized_roi_pct?.toFixed(2)}% (${sig.realized_pnl_usd?.toFixed(2)})
                    </span>
                  </div>
                )}

                {/* Manual Exit Button */}
                {sig.status === 'ACTIVE' && (
                  <div className="pt-1 flex justify-end">
                    <button
                      onClick={() => handleCloseSignal(sig.id, currentPrice)}
                      className="px-2 py-0.5 rounded bg-rose-500/10 hover:bg-rose-500/20 text-rose-500 text-[10px] font-semibold transition-colors flex items-center gap-1"
                    >
                      <XCircle className="w-3 h-3" />
                      <span>Close Position</span>
                    </button>
                  </div>
                )}
              </div>
            );
          })
        )}
      </div>
    </div>
  );
};
