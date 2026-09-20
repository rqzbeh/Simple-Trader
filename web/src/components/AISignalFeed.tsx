import React from 'react';
import { AISignal } from '../types';
import { Bot, ArrowUpRight, ArrowDownRight, Minus, Sparkles } from 'lucide-react';

interface AISignalFeedProps {
  signals: AISignal[];
}

export const AISignalFeed: React.FC<AISignalFeedProps> = ({ signals }) => {
  return (
    <div className="rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 p-4 shadow-sm flex flex-col h-full">
      <div className="flex items-center justify-between pb-3 border-b border-slate-100 dark:border-slate-800 mb-3">
        <div className="flex items-center space-x-2">
          <Bot className="w-4 h-4 text-indigo-500" />
          <h3 className="font-semibold text-sm tracking-tight text-slate-800 dark:text-slate-200">
            Real-Time AI Signals & Confluence
          </h3>
        </div>
        <span className="flex items-center text-[11px] font-medium text-indigo-500 bg-indigo-500/10 px-2 py-0.5 rounded border border-indigo-500/20">
          <Sparkles className="w-3 h-3 mr-1" />
          Multi-Indicator Confluence
        </span>
      </div>

      <div className="space-y-3 overflow-y-auto max-h-[380px] pr-1">
        {signals.map((sig) => {
          const isBuy = sig.direction === 'BUY';
          const isSell = sig.direction === 'SELL';
          const confluencePct = Math.round(sig.confluenceScore * 100);

          return (
            <div
              key={sig.id}
              className="p-3 rounded-lg border border-slate-200 dark:border-slate-800 bg-slate-50/50 dark:bg-slate-800/30 text-xs space-y-2"
            >
              <div className="flex items-center justify-between">
                <div className="flex items-center space-x-2">
                  <span className="font-mono font-bold text-slate-900 dark:text-slate-100">
                    {sig.symbol}
                  </span>
                  <span
                    className={`px-2 py-0.5 rounded font-bold text-[10px] flex items-center space-x-0.5 ${
                      isBuy
                        ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20'
                        : isSell
                        ? 'bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/20'
                        : 'bg-slate-500/10 text-slate-600 dark:text-slate-400 border border-slate-500/20'
                    }`}
                  >
                    {isBuy && <ArrowUpRight className="w-3 h-3 mr-0.5" />}
                    {isSell && <ArrowDownRight className="w-3 h-3 mr-0.5" />}
                    {!isBuy && !isSell && <Minus className="w-3 h-3 mr-0.5" />}
                    {sig.direction}
                  </span>
                  <span className="text-[10px] text-slate-400 dark:text-slate-500 font-mono">
                    {sig.regime}
                  </span>
                </div>
                <div className="flex items-center space-x-1.5 font-mono text-[11px]">
                  <span className="text-slate-400">Score:</span>
                  <span className="font-bold text-indigo-500">{confluencePct}%</span>
                </div>
              </div>

              {/* Rationale */}
              <p className="text-slate-600 dark:text-slate-300 leading-relaxed font-sans">
                {sig.rationale}
              </p>

              {/* Technical Indicator Snapshots */}
              <div className="flex flex-wrap gap-1.5 pt-1">
                {sig.indicators.rsi !== undefined && (
                  <span className="px-1.5 py-0.5 rounded bg-slate-200 dark:bg-slate-800 text-[10px] font-mono text-slate-700 dark:text-slate-300">
                    RSI: {sig.indicators.rsi.toFixed(1)}
                  </span>
                )}
                {sig.indicators.macd !== undefined && (
                  <span className="px-1.5 py-0.5 rounded bg-slate-200 dark:bg-slate-800 text-[10px] font-mono text-slate-700 dark:text-slate-300">
                    MACD: {sig.indicators.macd.toFixed(2)}
                  </span>
                )}
                {sig.indicators.supertrend && (
                  <span className="px-1.5 py-0.5 rounded bg-slate-200 dark:bg-slate-800 text-[10px] font-mono text-slate-700 dark:text-slate-300">
                    SuperTrend: {sig.indicators.supertrend}
                  </span>
                )}
                <span className="text-[10px] text-slate-400 dark:text-slate-500 ml-auto font-mono self-center">
                  {sig.timestamp}
                </span>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
};
