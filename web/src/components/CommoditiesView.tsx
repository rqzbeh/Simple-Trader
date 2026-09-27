import React, { useState, useEffect, useCallback } from 'react';
import { Fuel, Clock, Ban, ShieldAlert, Target, RefreshCw } from 'lucide-react';
import { FuturesTradeSignal, formatSignalPrice } from '../types';
import { PerformanceSummary } from './PerformanceSummary';

interface CommodityStatus {
  profile: string;
  weekend_flat: boolean;
  in_weekend_gap: boolean;
  blocked_events: string[];
  horizon_min: number;
  horizon_max: number;
  freshness_halflife_min: number;
}

// CommoditiesView is the dedicated asset-class section (spec 012 US4,
// FR-015): commodity-profile signal list, horizon/blackout status banner
// and an asset-class-separated performance block.
export const CommoditiesView: React.FC = () => {
  const [status, setStatus] = useState<CommodityStatus | null>(null);
  const [signals, setSignals] = useState<FuturesTradeSignal[]>([]);
  const [statusFilter, setStatusFilter] = useState<'ACTIVE' | 'CLOSED'>('ACTIVE');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchAll = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const [stRes, sigRes] = await Promise.all([
        fetch('/api/v1/commodities/status'),
        fetch(`/api/v1/signals/futures?profile=COMMODITY&status=${statusFilter}&limit=50`),
      ]);
      if (!stRes.ok) throw new Error(`status: HTTP ${stRes.status}`);
      setStatus(await stRes.json());
      const sigData = sigRes.ok ? await sigRes.json() : [];
      setSignals(Array.isArray(sigData) ? sigData : []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'failed to load commodities');
    } finally {
      setLoading(false);
    }
  }, [statusFilter]);

  useEffect(() => {
    fetchAll();
  }, [fetchAll]);

  return (
    <div className="space-y-4">
      {/* Horizon & session status banner (FR-014) */}
      <div className="rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 p-4 shadow-sm">
        <div className="flex items-center justify-between pb-2 border-b border-slate-100 dark:border-slate-800">
          <div className="flex items-center gap-2">
            <Fuel className="w-4 h-4 text-orange-500" />
            <h3 className="font-semibold text-sm tracking-tight text-slate-800 dark:text-slate-100">
              Commodities Session
            </h3>
          </div>
          <button
            onClick={fetchAll}
            className="p-1.5 rounded-lg border border-slate-200 dark:border-slate-800 text-slate-500 hover:text-slate-800 dark:hover:text-slate-200"
            aria-label="Refresh commodities"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>

        {error && (
          <div className="mt-2 p-2 rounded bg-rose-500/10 border border-rose-500/20 text-rose-600 dark:text-rose-400 text-xs font-mono">
            {error}
          </div>
        )}

        <div className="mt-2 flex flex-wrap gap-2 text-xs font-mono">
          <span className="px-2 py-1 rounded bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300 flex items-center gap-1">
            <Clock className="w-3 h-3" />
            horizon {status ? `${status.horizon_min}-${status.horizon_max}m` : '—'}
          </span>
          <span
            className={`px-2 py-1 rounded border flex items-center gap-1 ${
              status?.in_weekend_gap
                ? 'bg-amber-500/10 border-amber-500/30 text-amber-600 dark:text-amber-400'
                : 'bg-emerald-500/10 border-emerald-500/30 text-emerald-600 dark:text-emerald-400'
            }`}
          >
            {status?.in_weekend_gap ? 'WEEKEND GAP — FLAT' : 'MARKET OPEN'}
          </span>
          {status?.blocked_events?.map((ev) => (
            <span
              key={ev}
              className="px-2 py-1 rounded bg-rose-500/10 border border-rose-500/30 text-rose-600 dark:text-rose-400 flex items-center gap-1"
            >
              <Ban className="w-3 h-3" /> {ev} BLACKOUT
            </span>
          ))}
          {status?.weekend_flat && !status.in_weekend_gap && !(status.blocked_events?.length) ? (
            <span className="px-2 py-1 rounded bg-slate-100 dark:bg-slate-800 text-slate-500">
              weekend flat armed
            </span>
          ) : null}
        </div>
      </div>

      {/* Commodity signal list */}
      <div className="rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 p-4 shadow-sm">
        <div className="flex items-center justify-between pb-2 border-b border-slate-100 dark:border-slate-800">
          <div className="flex items-center gap-2">
            <Fuel className="w-4 h-4 text-orange-500" />
            <h3 className="font-semibold text-sm tracking-tight text-slate-800 dark:text-slate-100">
              Commodity Signals
            </h3>
            <span className="text-[10px] px-1.5 py-0.5 rounded font-mono bg-slate-100 dark:bg-slate-800 text-slate-500">
              {signals.length}
            </span>
          </div>
          <div className="flex items-center gap-1 p-0.5 rounded-lg bg-slate-100 dark:bg-slate-800/80 text-xs font-mono">
            {(['ACTIVE', 'CLOSED'] as const).map((f) => (
              <button
                key={f}
                onClick={() => setStatusFilter(f)}
                className={`px-2.5 py-1 rounded-md text-[11px] font-semibold transition-colors ${
                  statusFilter === f
                    ? 'bg-white dark:bg-slate-900 text-sky-600 dark:text-sky-400 shadow-xs'
                    : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'
                }`}
              >
                {f === 'ACTIVE' ? 'Active' : 'History'}
              </button>
            ))}
          </div>
        </div>

        {loading ? (
          <div className="py-8 text-center text-xs font-mono text-slate-400">loading…</div>
        ) : signals.length === 0 ? (
          <div className="py-8 text-center text-xs font-mono text-slate-400">
            no {statusFilter.toLowerCase()} commodity signals
          </div>
        ) : (
          <div className="divide-y divide-slate-100 dark:divide-slate-800">
            {signals.map((sig) => (
              <div key={sig.id} className="py-2.5 flex flex-col gap-1.5">
                <div className="flex items-center justify-between text-xs">
                  <span className="font-bold font-mono text-slate-800 dark:text-slate-200">
                    {sig.symbol}
                  </span>
                  <span
                    className={`px-1.5 py-0.5 rounded text-[10px] font-mono font-bold uppercase ${
                      sig.direction === 'LONG'
                        ? 'bg-emerald-500/15 text-emerald-500'
                        : 'bg-rose-500/15 text-rose-500'
                    }`}
                  >
                    {sig.direction}
                  </span>
                </div>
                <div className="grid grid-cols-4 gap-1 font-mono text-[11px]">
                  <div>
                    <span className="block text-[10px] text-slate-400">Entry</span>
                    <span className="font-semibold">{formatSignalPrice(sig.entry_price)}</span>
                  </div>
                  <div>
                    <span className="block text-[10px] text-rose-400 flex items-center gap-0.5">
                      <ShieldAlert className="w-2.5 h-2.5" /> SL
                    </span>
                    <span className="font-semibold text-rose-500">{formatSignalPrice(sig.stop_loss)}</span>
                  </div>
                  <div>
                    <span className="block text-[10px] text-emerald-400 flex items-center gap-0.5">
                      <Target className="w-2.5 h-2.5" /> TP1
                    </span>
                    <span className="font-semibold text-emerald-500">{formatSignalPrice(sig.take_profit_1)}</span>
                  </div>
                  <div>
                    <span className="block text-[10px] text-emerald-300">TP2</span>
                    <span className="font-semibold text-emerald-400">
                      {sig.take_profit_2 ? formatSignalPrice(sig.take_profit_2) : '--'}
                    </span>
                  </div>
                </div>
                {sig.decay_state && sig.decay_state !== 'NONE' ? (
                  <span className="self-start px-1.5 py-0.5 rounded text-[10px] font-mono bg-amber-500/10 text-amber-500">
                    {sig.decay_state}
                  </span>
                ) : null}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Asset-class-separated performance (FR-015 / SC-007) */}
      <PerformanceSummary profile="COMMODITY" />
    </div>
  );
};
