import React, { useState, useEffect, useCallback } from 'react';
import {
  TrendingUp,
  TrendingDown,
  Target,
  ShieldAlert,
  Percent,
  DollarSign,
  Award,
  RefreshCw,
  BarChart2,
} from 'lucide-react';

interface SignalSummary {
  profile?: string;
  closed: number;
  wins: number;
  losses: number;
  flat: number;
  win_rate: number;
  avg_win_pct: number;
  avg_loss_pct: number;
  payoff_ratio: number;
  expectancy_pct: number;
  total_pnl_usd: number;
  stop_out_rate: number;
  tp1_hit_rate: number;
}

interface PerformanceSummaryProps {
  profile?: string;
}

const PROFILES: { label: string; value: string }[] = [
  { label: 'All', value: '' },
  { label: 'Crypto', value: 'CRYPTO' },
  { label: 'Commodity', value: 'COMMODITY' },
];

export const PerformanceSummary: React.FC<PerformanceSummaryProps> = ({ profile = '' }) => {
  const [selectedProfile, setSelectedProfile] = useState<string>(profile);
  const [summary, setSummary] = useState<SignalSummary | null>(null);
  const [loading, setLoading] = useState<boolean>(true);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  useEffect(() => {
    setSelectedProfile(profile);
  }, [profile]);

  const fetchSummary = useCallback(async (prof: string) => {
    try {
      setLoading(true);
      setErrorMsg(null);
      const url = `/api/v1/signals/summary?profile=${encodeURIComponent(prof)}`;
      const res = await fetch(url);
      if (!res.ok) {
        throw new Error(`HTTP ${res.status}: Failed to load performance summary`);
      }
      const data: SignalSummary = await res.json();
      setSummary(data);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Error fetching summary';
      setErrorMsg(msg);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchSummary(selectedProfile);
  }, [selectedProfile, fetchSummary]);

  return (
    <div className="p-4 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 shadow-sm space-y-4">
      {/* Header and Profile Selector */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <div className="p-1.5 rounded-lg bg-sky-500/10 text-sky-500">
            <BarChart2 className="w-4 h-4" />
          </div>
          <div>
            <h3 className="text-sm font-bold tracking-tight text-slate-900 dark:text-slate-100">
              Signal Performance Summary
            </h3>
            <p className="text-[11px] text-slate-500 dark:text-slate-400 font-mono">
              Closed position statistics & expectancy metrics
            </p>
          </div>
        </div>

        <div className="flex items-center gap-2">
          {/* Profile Toggle */}
          <div className="flex items-center p-0.5 rounded-lg bg-slate-100 dark:bg-slate-800/80 border border-slate-200/60 dark:border-slate-700/60">
            {PROFILES.map((p) => {
              const active = selectedProfile === p.value;
              return (
                <button
                  key={p.value}
                  type="button"
                  onClick={() => setSelectedProfile(p.value)}
                  className={`px-2.5 py-1 rounded-md text-xs font-semibold font-mono transition-colors ${
                    active
                      ? 'bg-white dark:bg-slate-900 text-sky-600 dark:text-sky-400 shadow-xs'
                      : 'text-slate-500 hover:text-slate-800 dark:hover:text-slate-200'
                  }`}
                >
                  {p.label}
                </button>
              );
            })}
          </div>

          <button
            type="button"
            onClick={() => fetchSummary(selectedProfile)}
            disabled={loading}
            aria-label="Refresh summary"
            className="p-1.5 rounded-lg border border-slate-200 dark:border-slate-800 hover:bg-slate-50 dark:hover:bg-slate-800 text-slate-500 transition-colors disabled:opacity-50"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>

      {/* Error state */}
      {errorMsg && (
        <div className="p-2.5 rounded-lg bg-rose-500/10 border border-rose-500/20 text-rose-600 dark:text-rose-400 text-xs font-mono">
          {errorMsg}
        </div>
      )}

      {/* Loading Skeleton */}
      {loading ? (
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-9 gap-2.5" aria-hidden="true">
          {Array.from({ length: 9 }).map((_, idx) => (
            <div
              key={idx}
              className="p-3 rounded-lg border border-slate-200/60 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/40 animate-pulse space-y-2"
            >
              <div className="h-3 w-14 rounded bg-slate-200 dark:bg-slate-700" />
              <div className="h-5 w-18 rounded bg-slate-200 dark:bg-slate-700" />
            </div>
          ))}
        </div>
      ) : !summary || summary.closed === 0 ? (
        /* Empty State */
        <div className="p-8 text-center rounded-xl border border-slate-200 dark:border-slate-800 bg-slate-50/50 dark:bg-slate-800/20 text-slate-400 text-xs font-mono">
          No closed trade signals recorded for {selectedProfile ? `${selectedProfile} profile` : 'selected criteria'}.
        </div>
      ) : (
        /* Performance Cards Grid */
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-9 gap-2.5">
          {/* 1. Closed */}
          <div className="p-3 rounded-lg bg-slate-50 dark:bg-slate-800/60 border border-slate-200/60 dark:border-slate-700/60 flex flex-col justify-between">
            <span className="text-[10px] uppercase font-mono tracking-wider text-slate-500 dark:text-slate-400">
              Closed
            </span>
            <div className="mt-1">
              <span className="text-base font-bold font-mono text-slate-900 dark:text-slate-100">
                {summary.closed}
              </span>
              <span className="block text-[10px] text-slate-400 font-mono">
                {summary.wins}W / {summary.losses}L{summary.flat > 0 ? ` / ${summary.flat}F` : ''}
              </span>
            </div>
          </div>

          {/* 2. Win rate % */}
          <div className="p-3 rounded-lg bg-slate-50 dark:bg-slate-800/60 border border-slate-200/60 dark:border-slate-700/60 flex flex-col justify-between">
            <span className="text-[10px] uppercase font-mono tracking-wider text-slate-500 dark:text-slate-400 flex items-center gap-1">
              <Percent className="w-2.5 h-2.5 text-sky-400" /> Win Rate
            </span>
            <div className="mt-1">
              <span
                className={`text-base font-bold font-mono ${
                  summary.win_rate >= 0.5 ? 'text-emerald-500' : 'text-slate-700 dark:text-slate-200'
                }`}
              >
                {(summary.win_rate * 100).toFixed(1)}%
              </span>
              <span className="block text-[10px] text-slate-400 font-mono">
                {(summary.win_rate * summary.closed).toFixed(0)} of {summary.closed}
              </span>
            </div>
          </div>

          {/* 3. Avg win % */}
          <div className="p-3 rounded-lg bg-slate-50 dark:bg-slate-800/60 border border-slate-200/60 dark:border-slate-700/60 flex flex-col justify-between">
            <span className="text-[10px] uppercase font-mono tracking-wider text-slate-500 dark:text-slate-400 flex items-center gap-1">
              <TrendingUp className="w-2.5 h-2.5 text-emerald-500" /> Avg Win
            </span>
            <div className="mt-1">
              <span className="text-base font-bold font-mono text-emerald-500">
                +{summary.avg_win_pct.toFixed(2)}%
              </span>
              <span className="block text-[10px] text-slate-400 font-mono">Per winning trade</span>
            </div>
          </div>

          {/* 4. Avg loss % */}
          <div className="p-3 rounded-lg bg-slate-50 dark:bg-slate-800/60 border border-slate-200/60 dark:border-slate-700/60 flex flex-col justify-between">
            <span className="text-[10px] uppercase font-mono tracking-wider text-slate-500 dark:text-slate-400 flex items-center gap-1">
              <TrendingDown className="w-2.5 h-2.5 text-rose-500" /> Avg Loss
            </span>
            <div className="mt-1">
              <span className="text-base font-bold font-mono text-rose-500">
                {summary.avg_loss_pct.toFixed(2)}%
              </span>
              <span className="block text-[10px] text-slate-400 font-mono">Per losing trade</span>
            </div>
          </div>

          {/* 5. Payoff */}
          <div className="p-3 rounded-lg bg-slate-50 dark:bg-slate-800/60 border border-slate-200/60 dark:border-slate-700/60 flex flex-col justify-between">
            <span className="text-[10px] uppercase font-mono tracking-wider text-slate-500 dark:text-slate-400 flex items-center gap-1">
              <Award className="w-2.5 h-2.5 text-amber-500" /> Payoff
            </span>
            <div className="mt-1">
              <span className="text-base font-bold font-mono text-amber-500">
                {summary.payoff_ratio.toFixed(2)}
              </span>
              <span className="block text-[10px] text-slate-400 font-mono">Win / Loss ratio</span>
            </div>
          </div>

          {/* 6. Expectancy % */}
          <div className="p-3 rounded-lg bg-slate-50 dark:bg-slate-800/60 border border-slate-200/60 dark:border-slate-700/60 flex flex-col justify-between">
            <span className="text-[10px] uppercase font-mono tracking-wider text-slate-500 dark:text-slate-400">
              Expectancy
            </span>
            <div className="mt-1">
              <span
                className={`text-base font-bold font-mono ${
                  summary.expectancy_pct >= 0 ? 'text-emerald-500' : 'text-rose-500'
                }`}
              >
                {summary.expectancy_pct >= 0 ? '+' : ''}
                {summary.expectancy_pct.toFixed(2)}%
              </span>
              <span className="block text-[10px] text-slate-400 font-mono">Edge per trade</span>
            </div>
          </div>

          {/* 7. Total PnL USD */}
          <div className="p-3 rounded-lg bg-slate-50 dark:bg-slate-800/60 border border-slate-200/60 dark:border-slate-700/60 flex flex-col justify-between">
            <span className="text-[10px] uppercase font-mono tracking-wider text-slate-500 dark:text-slate-400 flex items-center gap-1">
              <DollarSign className="w-2.5 h-2.5 text-sky-400" /> Total PnL
            </span>
            <div className="mt-1">
              <span
                className={`text-base font-bold font-mono ${
                  summary.total_pnl_usd >= 0 ? 'text-emerald-500' : 'text-rose-500'
                }`}
              >
                {summary.total_pnl_usd >= 0 ? '+' : ''}$
                {summary.total_pnl_usd.toLocaleString(undefined, {
                  minimumFractionDigits: 2,
                  maximumFractionDigits: 2,
                })}
              </span>
              <span className="block text-[10px] text-slate-400 font-mono">Net realized USD</span>
            </div>
          </div>

          {/* 8. Stop-out rate % */}
          <div className="p-3 rounded-lg bg-slate-50 dark:bg-slate-800/60 border border-slate-200/60 dark:border-slate-700/60 flex flex-col justify-between">
            <span className="text-[10px] uppercase font-mono tracking-wider text-slate-500 dark:text-slate-400 flex items-center gap-1">
              <ShieldAlert className="w-2.5 h-2.5 text-rose-500" /> Stop-Out
            </span>
            <div className="mt-1">
              <span className="text-base font-bold font-mono text-slate-800 dark:text-slate-200">
                {(summary.stop_out_rate * 100).toFixed(1)}%
              </span>
              <span className="block text-[10px] text-slate-400 font-mono">Hit stop loss</span>
            </div>
          </div>

          {/* 9. TP1 hit % */}
          <div className="p-3 rounded-lg bg-slate-50 dark:bg-slate-800/60 border border-slate-200/60 dark:border-slate-700/60 flex flex-col justify-between">
            <span className="text-[10px] uppercase font-mono tracking-wider text-slate-500 dark:text-slate-400 flex items-center gap-1">
              <Target className="w-2.5 h-2.5 text-emerald-500" /> TP1 Hit
            </span>
            <div className="mt-1">
              <span className="text-base font-bold font-mono text-emerald-500">
                {(summary.tp1_hit_rate * 100).toFixed(1)}%
              </span>
              <span className="block text-[10px] text-slate-400 font-mono">Reached target 1</span>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
