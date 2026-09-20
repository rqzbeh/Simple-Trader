import React, { useState } from 'react';
import { BacktestSummary, MonteCarloSummary } from '../types';

interface BacktesterViewProps {
  symbol: string;
}

export const BacktesterView: React.FC<BacktesterViewProps> = ({ symbol }) => {
  const [initialCapital, setInitialCapital] = useState<number>(100000);
  const [barsCount, setBarsCount] = useState<number>(1000);
  const [loading, setLoading] = useState<boolean>(false);
  const [btResult, setBtResult] = useState<BacktestSummary | null>(null);
  const [mcResult, setMcResult] = useState<MonteCarloSummary | null>(null);
  const [error, setError] = useState<string | null>(null);

  const runSimulation = async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await fetch('/api/v1/backtest/run', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          symbol,
          initial_capital: initialCapital,
          bars_count: barsCount,
        }),
      });

      if (!response.ok) {
        throw new Error(`Server returned HTTP ${response.status}`);
      }

      const data = await response.json();
      setBtResult(data.backtest);
      setMcResult(data.monte_carlo);
    } catch (err: any) {
      setError(err.message || 'Backtest execution failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="bg-slate-900 border border-slate-800 rounded-xl p-5 shadow-lg">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center space-x-2">
          <span className="w-2.5 h-2.5 rounded-full bg-indigo-400" />
          <h3 className="font-semibold text-slate-100 text-sm tracking-wide">
            Vectorized Backtester & Monte Carlo Simulator (FR-008)
          </h3>
        </div>
        <button
          onClick={runSimulation}
          disabled={loading}
          className="px-4 py-1.5 bg-indigo-600 hover:bg-indigo-500 text-white rounded-lg text-xs font-semibold tracking-wide transition-all shadow-md hover:shadow-indigo-500/20 disabled:opacity-50"
        >
          {loading ? 'Evaluating 1,000 Paths...' : `Simulate ${symbol}`}
        </button>
      </div>

      {/* Control Inputs */}
      <div className="grid grid-cols-2 gap-3 mb-4">
        <div>
          <label className="text-[11px] text-slate-400 block mb-1">Initial Capital ($)</label>
          <input
            type="number"
            value={initialCapital}
            onChange={(e) => setInitialCapital(Number(e.target.value))}
            className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-1.5 text-xs text-slate-200 font-mono focus:outline-none focus:border-indigo-500"
          />
        </div>
        <div>
          <label className="text-[11px] text-slate-400 block mb-1">Historical Bars (Candles)</label>
          <input
            type="number"
            value={barsCount}
            onChange={(e) => setBarsCount(Number(e.target.value))}
            className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-1.5 text-xs text-slate-200 font-mono focus:outline-none focus:border-indigo-500"
          />
        </div>
      </div>

      {error && (
        <div className="p-3 bg-rose-950/40 border border-rose-500/30 rounded-lg text-rose-400 text-xs mb-4">
          {error}
        </div>
      )}

      {/* Results View */}
      {btResult && mcResult && (
        <div className="space-y-4">
          {/* Backtest Statistics Cards */}
          <div className="grid grid-cols-4 gap-2.5 font-mono text-center">
            <div className="bg-slate-800/60 p-2.5 rounded-lg border border-slate-700/50">
              <span className="text-[10px] text-slate-400 block">Total Return</span>
              <span className={`text-sm font-bold ${btResult.total_return_pct >= 0 ? 'text-emerald-400' : 'text-rose-400'}`}>
                {btResult.total_return_pct >= 0 ? `+${(btResult.total_return_pct * 100).toFixed(2)}%` : `${(btResult.total_return_pct * 100).toFixed(2)}%`}
              </span>
            </div>

            <div className="bg-slate-800/60 p-2.5 rounded-lg border border-slate-700/50">
              <span className="text-[10px] text-slate-400 block">Sharpe Ratio</span>
              <span className="text-sm font-bold text-slate-200">{btResult.sharpe_ratio.toFixed(2)}</span>
            </div>

            <div className="bg-slate-800/60 p-2.5 rounded-lg border border-slate-700/50">
              <span className="text-[10px] text-slate-400 block">Sortino Ratio</span>
              <span className="text-sm font-bold text-slate-200">{btResult.sortino_ratio.toFixed(2)}</span>
            </div>

            <div className="bg-slate-800/60 p-2.5 rounded-lg border border-slate-700/50">
              <span className="text-[10px] text-slate-400 block">Max Drawdown</span>
              <span className="text-sm font-bold text-rose-400">
                {(btResult.max_drawdown_pct * 100).toFixed(2)}%
              </span>
            </div>
          </div>

          {/* Monte Carlo Bootstrap Card */}
          <div className="bg-indigo-950/30 border border-indigo-500/30 rounded-lg p-3">
            <div className="flex items-center justify-between text-xs mb-2">
              <span className="font-semibold text-indigo-300">
                Monte Carlo Bootstrap ({mcResult.iterations.toLocaleString()} Iterations)
              </span>
              <span className="text-slate-400 font-mono">
                Ruin Probability: <strong className="text-emerald-400">{mcResult.probability_of_ruin_pct.toFixed(2)}%</strong>
              </span>
            </div>

            <div className="grid grid-cols-3 gap-2 text-xs font-mono text-slate-300">
              <div>
                <span className="text-[10px] text-slate-400 block">Median Return:</span>
                {(mcResult.median_return_pct * 100).toFixed(2)}%
              </div>
              <div>
                <span className="text-[10px] text-slate-400 block">95th Percentile DD:</span>
                <span className="text-amber-400">{(mcResult.max_drawdown_95th_pct * 100).toFixed(2)}%</span>
              </div>
              <div>
                <span className="text-[10px] text-slate-400 block">99th Percentile DD:</span>
                <span className="text-rose-400">{(mcResult.max_drawdown_99th_pct * 100).toFixed(2)}%</span>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
