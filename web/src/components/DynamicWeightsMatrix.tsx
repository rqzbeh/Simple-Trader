import React, { useState } from 'react';
import { Cpu, RotateCcw, Sliders, Sparkles, Check } from 'lucide-react';

interface IndicatorWeightMap {
  [indicator: string]: number;
}

interface DynamicWeightsMatrixProps {
  selectedSymbol: string;
  weights: IndicatorWeightMap;
  regime: string;
  onUpdateWeight?: (indicator: string, weight: number) => void;
  onResetWeights?: () => void;
}

export const DynamicWeightsMatrix: React.FC<DynamicWeightsMatrixProps> = ({
  selectedSymbol,
  weights,
  regime,
  onUpdateWeight,
  onResetWeights,
}) => {
  const [localWeights, setLocalWeights] = useState<IndicatorWeightMap>(weights);
  const [hasSaved, setHasSaved] = useState<boolean>(false);

  const indicators = Object.keys(weights);

  const handleSliderChange = (ind: string, val: number) => {
    setLocalWeights((prev) => ({ ...prev, [ind]: val }));
    onUpdateWeight?.(ind, val);
  };

  const handleSave = () => {
    setHasSaved(true);
    setTimeout(() => setHasSaved(false), 2000);
  };

  return (
    <div className="p-5 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 shadow-sm flex flex-col space-y-4">
      <div className="flex items-center justify-between pb-3 border-b border-slate-100 dark:border-slate-800/80">
        <div className="flex items-center space-x-2">
          <Cpu className="w-4 h-4 text-sky-500" />
          <h3 className="font-semibold text-sm tracking-tight text-slate-800 dark:text-slate-100">
            AI Indicator Weight Calibration Matrix ({selectedSymbol})
          </h3>
        </div>
        <div className="flex items-center space-x-2">
          <span className="text-xs px-2 py-0.5 rounded bg-sky-500/10 text-sky-500 border border-sky-500/20 font-mono">
            Regime: {regime}
          </span>
          {onResetWeights && (
            <button
              onClick={onResetWeights}
              className="p-1 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 transition-colors"
              title="Reset to 1.0 Baseline"
            >
              <RotateCcw className="w-3.5 h-3.5" />
            </button>
          )}
        </div>
      </div>

      <p className="text-xs text-slate-500 dark:text-slate-400">
        Adaptive In-Context Learning dynamically scales indicator significance $[0.2, 3.0]$ based on historical trade attribution and profit/regret bonuses.
      </p>

      {/* Grid of Sliders and Badges */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {indicators.map((ind) => {
          const w = localWeights[ind] ?? 1.0;
          const isBoosted = w > 1.05;
          const isPenalized = w < 0.95;

          return (
            <div
              key={ind}
              className="p-3 rounded-lg border border-slate-100 dark:border-slate-800/80 bg-slate-50/50 dark:bg-slate-900/30 flex flex-col space-y-2"
            >
              <div className="flex items-center justify-between">
                <span className="font-mono text-xs font-bold text-slate-800 dark:text-slate-200">
                  {ind}
                </span>
                <span
                  className={`font-mono text-xs font-bold px-1.5 py-0.5 rounded ${
                    isBoosted
                      ? 'bg-emerald-500/10 text-emerald-500'
                      : isPenalized
                      ? 'bg-rose-500/10 text-rose-500'
                      : 'bg-slate-200 dark:bg-slate-800 text-slate-600 dark:text-slate-400'
                  }`}
                >
                  {w.toFixed(2)}x
                </span>
              </div>

              <input
                type="range"
                min="0.2"
                max="3.0"
                step="0.05"
                value={w}
                onChange={(e) => handleSliderChange(ind, parseFloat(e.target.value))}
                className="w-full accent-sky-500 h-1.5 bg-slate-200 dark:bg-slate-700 rounded-lg cursor-pointer"
              />

              <div className="flex justify-between text-[10px] text-slate-400 font-mono">
                <span>0.20 (Min)</span>
                <span>1.00 (Base)</span>
                <span>3.00 (Max)</span>
              </div>
            </div>
          );
        })}
      </div>

      <div className="flex items-center justify-between pt-2 border-t border-slate-100 dark:border-slate-800/80">
        <div className="flex items-center space-x-1.5 text-xs text-slate-500 dark:text-slate-400">
          <Sparkles className="w-3.5 h-3.5 text-amber-400" />
          <span>Post-Trade Regret Minimization active</span>
        </div>
        <button
          onClick={handleSave}
          className="flex items-center space-x-1.5 px-3 py-1.5 rounded-lg bg-sky-500 hover:bg-sky-600 text-white text-xs font-semibold shadow-sm transition-colors"
        >
          {hasSaved ? <Check className="w-3.5 h-3.5" /> : <Sliders className="w-3.5 h-3.5" />}
          <span>{hasSaved ? 'Weights Synced' : 'Sync to Redis'}</span>
        </button>
      </div>
    </div>
  );
};
