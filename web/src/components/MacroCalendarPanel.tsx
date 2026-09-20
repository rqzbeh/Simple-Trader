import React, { useState, useEffect } from 'react';
import { MacroCalendarEvent } from '../types';

interface MacroCalendarPanelProps {
  events: MacroCalendarEvent[];
  activeSymbol: string;
}

export const MacroCalendarPanel: React.FC<MacroCalendarPanelProps> = ({ events, activeSymbol }) => {
  const [currentTime, setCurrentTime] = useState(new Date());

  useEffect(() => {
    const timer = setInterval(() => setCurrentTime(new Date()), 1000);
    return () => clearInterval(timer);
  }, []);

  // Determine if trading is currently halted for active symbol (within +/- 15 mins of HIGH impact event)
  const isSymbolHalted = events.some((ev) => {
    if (ev.impact !== 'HIGH') return false;
    const evTime = new Date(ev.scheduled_at).getTime();
    const diffMins = Math.abs(currentTime.getTime() - evTime) / (60 * 1000);
    return diffMins <= 15;
  });

  return (
    <div className="bg-slate-900 border border-slate-800 rounded-xl p-5 shadow-lg">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center space-x-2">
          <span className="w-2.5 h-2.5 rounded-full bg-amber-400" />
          <h3 className="font-semibold text-slate-100 text-sm tracking-wide">
            Macro Economic Calendar & Circuit Breaker
          </h3>
        </div>
        {isSymbolHalted ? (
          <span className="px-2.5 py-1 rounded text-xs font-semibold bg-rose-500/20 text-rose-400 border border-rose-500/40 animate-pulse">
            ENTRY HALTED: HIGH-IMPACT MACRO EVENT
          </span>
        ) : (
          <span className="px-2.5 py-1 rounded text-xs font-semibold bg-emerald-500/20 text-emerald-400 border border-emerald-500/30">
            TRADING ACTIVE: SAFE MACRO WINDOW
          </span>
        )}
      </div>

      <div className="space-y-2.5">
        {events.length === 0 ? (
          <div className="text-slate-400 text-xs py-4 text-center">
            No high-impact releases scheduled in the near window.
          </div>
        ) : (
          events.map((ev) => {
            const evDate = new Date(ev.scheduled_at);
            const diffMs = evDate.getTime() - currentTime.getTime();
            const diffMins = Math.round(diffMs / (60 * 1000));
            const isImminent = Math.abs(diffMins) <= 15;

            return (
              <div
                key={ev.id}
                className={`p-3 rounded-lg border flex items-center justify-between text-xs transition-colors ${
                  isImminent
                    ? 'bg-rose-950/30 border-rose-500/40'
                    : 'bg-slate-800/40 border-slate-750'
                }`}
              >
                <div>
                  <div className="flex items-center space-x-2">
                    <span className="font-bold text-slate-200">{ev.title}</span>
                    <span className="px-1.5 py-0.5 rounded text-[10px] font-mono font-semibold bg-amber-500/20 text-amber-300">
                      {ev.currency}
                    </span>
                    <span className="px-1.5 py-0.5 rounded text-[10px] font-mono font-semibold bg-rose-500/20 text-rose-300">
                      {ev.impact}
                    </span>
                  </div>
                  <div className="text-slate-400 text-[11px] mt-0.5 font-mono">
                    Time: {evDate.toLocaleTimeString()} | Forecast: {ev.forecast || 'N/A'} | Previous: {ev.previous || 'N/A'}
                  </div>
                </div>

                <div className="text-right font-mono">
                  {diffMins > 0 ? (
                    <span className="text-amber-400 font-semibold">in {diffMins}m</span>
                  ) : diffMins >= -15 ? (
                    <span className="text-rose-400 font-semibold animate-pulse">
                      NOW ({Math.abs(diffMins)}m ago)
                    </span>
                  ) : (
                    <span className="text-slate-400">{Math.abs(diffMins)}m ago</span>
                  )}
                </div>
              </div>
            );
          })
        )}
      </div>

      <div className="mt-4 pt-3 border-t border-slate-800 flex justify-between items-center text-[11px] text-slate-400 font-mono">
        <span>FR-005 Protocol: Automatic Entry Freeze ±15 min</span>
        <span>Active Asset: {activeSymbol}</span>
      </div>
    </div>
  );
};
