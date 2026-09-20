import React from 'react';
import { MicrostructureState } from '../types';

interface MicrostructureCardProps {
  state: MicrostructureState;
}

export const MicrostructureCard: React.FC<MicrostructureCardProps> = ({ state }) => {
  // Format OBI between -1.0 and +1.0
  const obiPct = Math.round(state.obi * 100);
  const isObiBullish = state.obi > 0.05;
  const isObiBearish = state.obi < -0.05;

  const getRegimeBadge = () => {
    switch (state.regime) {
      case 'LOW_VOL_CONSOLIDATION':
        return (
          <span className="px-2.5 py-1 rounded text-xs font-semibold bg-blue-500/20 text-blue-400 border border-blue-500/30">
            LOW VOL CONSOLIDATION
          </span>
        );
      case 'HIGH_VOL_CHOP':
        return (
          <span className="px-2.5 py-1 rounded text-xs font-semibold bg-rose-500/20 text-rose-400 border border-rose-500/30 animate-pulse">
            HIGH VOL CHOP (SIZE DAMPENED)
          </span>
        );
      case 'NORMAL_TRENDING':
      default:
        return (
          <span className="px-2.5 py-1 rounded text-xs font-semibold bg-emerald-500/20 text-emerald-400 border border-emerald-500/30">
            NORMAL TRENDING
          </span>
        );
    }
  };

  return (
    <div className="bg-slate-900 border border-slate-800 rounded-xl p-5 shadow-lg flex flex-col justify-between">
      <div>
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center space-x-2">
            <span className="w-2.5 h-2.5 rounded-full bg-cyan-400 animate-ping" />
            <h3 className="font-semibold text-slate-100 text-sm tracking-wide">
              L2 Order Flow & Microstructure
            </h3>
          </div>
          {getRegimeBadge()}
        </div>

        {/* OBI Gauge */}
        <div className="mb-4">
          <div className="flex justify-between text-xs text-slate-400 mb-1.5 font-mono">
            <span>Bids (Buyers)</span>
            <span className={isObiBullish ? 'text-emerald-400 font-bold' : isObiBearish ? 'text-rose-400 font-bold' : 'text-slate-300'}>
              OBI: {state.obi >= 0 ? `+${state.obi.toFixed(3)}` : state.obi.toFixed(3)} ({obiPct}%)
            </span>
            <span>Asks (Sellers)</span>
          </div>
          <div className="w-full bg-slate-800 h-2.5 rounded-full overflow-hidden relative flex">
            {/* Center line */}
            <div className="absolute left-1/2 top-0 bottom-0 w-0.5 bg-slate-600 z-10" />
            <div
              className={`h-full transition-all duration-300 ${
                state.obi >= 0 ? 'ml-auto bg-emerald-500' : 'bg-rose-500 mr-auto'
              }`}
              style={{
                width: `${Math.min(50, Math.abs(state.obi) * 50)}%`,
                transform: state.obi >= 0 ? 'translateX(0)' : 'translateX(0)',
              }}
            />
          </div>
        </div>

        {/* Microstructure Metrics Grid */}
        <div className="grid grid-cols-2 gap-3 mb-2">
          <div className="bg-slate-800/60 rounded-lg p-2.5 border border-slate-700/50">
            <span className="text-[11px] text-slate-400 block mb-0.5">CVD Volume Delta</span>
            <span className={`text-sm font-mono font-semibold ${state.cvd >= 0 ? 'text-emerald-400' : 'text-rose-400'}`}>
              {state.cvd >= 0 ? `+${state.cvd.toLocaleString()}` : state.cvd.toLocaleString()}
            </span>
          </div>

          <div className="bg-slate-800/60 rounded-lg p-2.5 border border-slate-700/50">
            <span className="text-[11px] text-slate-400 block mb-0.5">VolRatio (ATR / SMA)</span>
            <span className="text-sm font-mono font-semibold text-slate-200">
              {state.volRatio.toFixed(2)}x
            </span>
          </div>
        </div>
      </div>

      {/* Divergence Tag */}
      <div className="mt-2 pt-2.5 border-t border-slate-800 flex items-center justify-between text-xs font-mono">
        <span className="text-slate-400">CVD Flow State:</span>
        <span
          className={`font-semibold ${
            state.divergence === 'BULLISH_ABSORPTION'
              ? 'text-emerald-400'
              : state.divergence === 'BEARISH_EXHAUSTION'
              ? 'text-rose-400'
              : 'text-slate-400'
          }`}
        >
          {state.divergence}
        </span>
      </div>
    </div>
  );
};
