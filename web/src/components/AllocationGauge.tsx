import React from 'react';
import { PortfolioSummary } from '../types';
import { ShieldCheck, AlertOctagon, Percent } from 'lucide-react';

interface AllocationGaugeProps {
  summary: PortfolioSummary;
}

export const AllocationGauge: React.FC<AllocationGaugeProps> = ({ summary }) => {
  const totalAllocated = summary.coreEquity + summary.alphaEquity;
  const corePct = totalAllocated > 0 ? (summary.coreEquity / totalAllocated) * 100 : 60;
  const alphaPct = totalAllocated > 0 ? (summary.alphaEquity / totalAllocated) * 100 : 40;

  return (
    <div className="p-5 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 shadow-sm flex flex-col justify-between">
      <div>
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center space-x-2">
            <Percent className="w-4 h-4 text-sky-500" />
            <h3 className="font-semibold text-sm tracking-tight text-slate-800 dark:text-slate-200">
              Core & Alpha Allocation Gauge
            </h3>
          </div>
          {summary.circuitBreakerHalted ? (
            <span className="flex items-center text-xs font-bold px-2 py-1 rounded bg-rose-500/10 text-rose-500 border border-rose-500/30 animate-pulse">
              <AlertOctagon className="w-3.5 h-3.5 mr-1" />
              CIRCUIT BREAKER HALTED
            </span>
          ) : (
            <span className="flex items-center text-xs font-medium px-2 py-0.5 rounded bg-emerald-500/10 text-emerald-500 border border-emerald-500/20">
              <ShieldCheck className="w-3.5 h-3.5 mr-1" />
              Risk Engine Normal
            </span>
          )}
        </div>

        {/* Dual Progress Bar */}
        <div className="space-y-1.5 mb-4">
          <div className="h-3 w-full bg-slate-100 dark:bg-slate-800 rounded-full overflow-hidden flex">
            <div
              style={{ width: `${corePct}%` }}
              className="bg-amber-500 h-full transition-all duration-500"
              title={`Core: ${corePct.toFixed(1)}%`}
            />
            <div
              style={{ width: `${alphaPct}%` }}
              className="bg-indigo-500 h-full transition-all duration-500"
              title={`Alpha: ${alphaPct.toFixed(1)}%`}
            />
          </div>
          <div className="flex justify-between text-xs font-mono">
            <span className="text-amber-600 dark:text-amber-400 font-medium">
              Core: {corePct.toFixed(1)}% (Target: 60%)
            </span>
            <span className="text-indigo-600 dark:text-indigo-400 font-medium">
              Alpha: {alphaPct.toFixed(1)}% (Target: 40%)
            </span>
          </div>
        </div>
      </div>

      {/* Sub-stats grid */}
      <div className="grid grid-cols-3 gap-2 pt-3 border-t border-slate-100 dark:border-slate-800/80 text-xs">
        <div>
          <span className="text-slate-500 dark:text-slate-400 block">Available Cash</span>
          <span className="font-mono font-bold text-slate-800 dark:text-slate-200">
            ${summary.cash.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
          </span>
        </div>
        <div>
          <span className="text-slate-500 dark:text-slate-400 block">Peak-to-Trough DD</span>
          <span className={`font-mono font-bold ${summary.drawdownPct > 5 ? 'text-rose-500' : 'text-emerald-500'}`}>
            -{summary.drawdownPct.toFixed(2)}%
          </span>
        </div>
        <div>
          <span className="text-slate-500 dark:text-slate-400 block">Peak Portfolio</span>
          <span className="font-mono font-bold text-slate-800 dark:text-slate-200">
            ${summary.peakEquity.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
          </span>
        </div>
      </div>
    </div>
  );
};
