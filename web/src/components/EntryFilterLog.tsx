import React, { useState, useEffect, useCallback } from 'react';
import { Clock, RefreshCw, ShieldAlert } from 'lucide-react';

interface EntryFilterLogEntry {
  id: number;
  created_at: string;
  symbol: string;
  direction: string;
  catalyst_event_id?: number | null;
  rule: string;
  detail: unknown;
}

interface EntryFilterLogProps {
  limit?: number;
}

const getRuleChipClass = (rule: string): string => {
  switch (rule) {
    case 'CHASE_BLOCKED':
    case 'POLARIZED':
      return 'bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20';
    case 'NO_VOLUME':
    case 'NO_TREND':
      return 'bg-slate-500/10 text-slate-600 dark:text-slate-400 border border-slate-500/20';
    case 'OI_TRAP':
    case 'LIQ_BUFFER':
    case 'EVENT_BLACKOUT':
    case 'STALE_NEWS':
    case 'NO_NEWS':
    default:
      return 'bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/20';
  }
};

const formatRelativeTime = (dateStr: string): string => {
  const date = new Date(dateStr);
  const now = new Date();
  const diffSec = Math.floor((now.getTime() - date.getTime()) / 1000);
  if (isNaN(diffSec) || diffSec < 5) return 'just now';
  if (diffSec < 60) return `${diffSec}s ago`;
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHour = Math.floor(diffMin / 60);
  if (diffHour < 24) return `${diffHour}h ago`;
  const diffDay = Math.floor(diffHour / 24);
  return `${diffDay}d ago`;
};

const parseDetail = (detail: unknown): { chips: string[]; raw?: string } => {
  if (detail === null || detail === undefined) {
    return { chips: [] };
  }
  let obj: unknown = detail;
  if (typeof detail === 'string') {
    try {
      obj = JSON.parse(detail);
    } catch {
      return { chips: [], raw: detail };
    }
  }
  if (typeof obj === 'object' && obj !== null && !Array.isArray(obj)) {
    const chips = Object.entries(obj).map(([k, v]) => {
      const valStr = typeof v === 'object' && v !== null ? JSON.stringify(v) : String(v);
      return `${k}=${valStr}`;
    });
    return { chips };
  }
  return { chips: [], raw: String(obj) };
};

export const EntryFilterLog: React.FC<EntryFilterLogProps> = ({ limit = 20 }) => {
  const [entries, setEntries] = useState<EntryFilterLogEntry[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  const fetchFilters = useCallback(async () => {
    try {
      setLoading(true);
      setErrorMsg(null);
      const res = await fetch(`/api/v1/signals/filters?limit=${limit}`);
      if (!res.ok) {
        throw new Error(`HTTP ${res.status}: Failed to fetch entry filter log`);
      }
      const data = await res.json();
      setEntries(Array.isArray(data) ? data : []);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Error fetching filter log';
      setErrorMsg(msg);
    } finally {
      setLoading(false);
    }
  }, [limit]);

  useEffect(() => {
    fetchFilters();
  }, [fetchFilters]);

  return (
    <div className="p-4 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 shadow-sm space-y-3">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <div className="p-1.5 rounded-lg bg-rose-500/10 text-rose-500">
            <ShieldAlert className="w-4 h-4" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h3 className="text-sm font-bold tracking-tight text-slate-900 dark:text-slate-100">
                Entry Filter Log
              </h3>
              <span className="text-[10px] px-1.5 py-0.5 rounded font-mono bg-slate-100 dark:bg-slate-800 text-slate-500">
                {entries.length}
              </span>
            </div>
            <p className="text-[11px] text-slate-500 dark:text-slate-400 font-mono">
              Rejected trade candidates filtered by entry gate rules
            </p>
          </div>
        </div>

        <button
          type="button"
          onClick={fetchFilters}
          disabled={loading}
          aria-label="Refresh filter log"
          className="p-1.5 rounded-lg border border-slate-200 dark:border-slate-800 hover:bg-slate-50 dark:hover:bg-slate-800 text-slate-500 transition-colors disabled:opacity-50"
        >
          <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
        </button>
      </div>

      {/* Error state */}
      {errorMsg && (
        <div className="p-2.5 rounded-lg bg-rose-500/10 border border-rose-500/20 text-rose-600 dark:text-rose-400 text-xs font-mono">
          {errorMsg}
        </div>
      )}

      {/* Loading state */}
      {loading ? (
        <div className="space-y-2 py-1" aria-hidden="true">
          {[0, 1, 2].map((i) => (
            <div
              key={i}
              className="p-3 rounded-lg border border-slate-100 dark:border-slate-800/80 bg-slate-50/60 dark:bg-slate-800/40 animate-pulse space-y-2"
            >
              <div className="flex items-center justify-between">
                <div className="h-3.5 w-24 rounded bg-slate-200 dark:bg-slate-700" />
                <div className="h-3 w-16 rounded bg-slate-200 dark:bg-slate-700" />
              </div>
              <div className="h-3 w-48 rounded bg-slate-200/70 dark:bg-slate-700/70" />
            </div>
          ))}
        </div>
      ) : entries.length === 0 ? (
        /* Empty State */
        <div className="p-8 text-center rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 text-slate-400 text-xs font-mono">
          No rejected entries
        </div>
      ) : (
        /* Scrollable List */
        <div className="divide-y divide-slate-100 dark:divide-slate-800/80 max-h-96 overflow-y-auto pr-1">
          {entries.map((entry) => {
            const isLong = entry.direction?.toUpperCase() === 'LONG';
            const { chips, raw } = parseDetail(entry.detail);

            return (
              <div
                key={entry.id}
                className="py-2.5 first:pt-1 last:pb-1 flex flex-col sm:flex-row sm:items-center justify-between gap-2"
              >
                {/* Left side: Symbol, Direction, Rule */}
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-bold text-xs tracking-tight font-mono text-slate-900 dark:text-slate-100">
                    {entry.symbol}
                  </span>

                  {/* Direction Badge */}
                  <span
                    className={`px-1.5 py-0.5 rounded text-[10px] font-mono font-bold uppercase tracking-wider ${
                      isLong
                        ? 'bg-emerald-500/15 text-emerald-500 border border-emerald-500/20'
                        : 'bg-rose-500/15 text-rose-500 border border-rose-500/20'
                    }`}
                  >
                    {entry.direction}
                  </span>

                  {/* Rule Chip */}
                  <span
                    className={`px-2 py-0.5 rounded text-[10px] font-mono font-semibold tracking-wide ${getRuleChipClass(
                      entry.rule
                    )}`}
                  >
                    {entry.rule}
                  </span>

                  {/* Catalyst Event ID if available */}
                  {entry.catalyst_event_id && (
                    <span className="text-[10px] px-1 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-slate-400 font-mono">
                      evt #{entry.catalyst_event_id}
                    </span>
                  )}
                </div>

                {/* Right side: Detail chips and Relative Time */}
                <div className="flex flex-wrap items-center gap-2 sm:justify-end">
                  {chips.length > 0 && (
                    <div className="flex flex-wrap gap-1 items-center">
                      {chips.map((chip, idx) => (
                        <span
                          key={idx}
                          className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-mono bg-slate-100 dark:bg-slate-800/80 text-slate-600 dark:text-slate-300 border border-slate-200/60 dark:border-slate-700/60"
                        >
                          {chip}
                        </span>
                      ))}
                    </div>
                  )}

                  {raw && (
                    <span className="text-[10px] font-mono text-slate-500 dark:text-slate-400 break-all">
                      {raw}
                    </span>
                  )}

                  {/* Relative Time */}
                  <span className="text-[10px] text-slate-400 font-mono flex items-center gap-1 shrink-0 ml-1">
                    <Clock className="w-3 h-3 text-slate-400" />
                    {formatRelativeTime(entry.created_at)}
                  </span>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
};
