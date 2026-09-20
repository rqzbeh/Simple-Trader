import React, { useState, useEffect } from 'react';
import { MacroRegimeState } from '../types';
import { Globe, ShieldAlert, AlertTriangle, CheckCircle2, RefreshCw } from 'lucide-react';

interface MacroRegimeViewProps {
  apiBaseUrl?: string;
}

export const MacroRegimeView: React.FC<MacroRegimeViewProps> = ({ apiBaseUrl = '' }) => {
  const [state, setState] = useState<MacroRegimeState | null>(null);
  const [loading, setLoading] = useState<boolean>(true);
  const [updating, setUpdating] = useState<boolean>(false);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  // Editable indicator sliders
  const [geoIndex, setGeoIndex] = useState<number>(0.70);
  const [infIndex, setInfIndex] = useState<number>(0.55);
  const [rateIndex, setRateIndex] = useState<number>(0.65);

  const fetchRegime = async () => {
    try {
      setLoading(true);
      setErrorMsg(null);
      const res = await fetch(`${apiBaseUrl}/api/v1/macro/regime`);
      if (!res.ok) throw new Error(`HTTP ${res.status}: Failed to fetch macro regime`);
      const data: MacroRegimeState = await res.json();
      setState(data);
      if (data.indicators) {
        setGeoIndex(data.indicators.geopolitical_index ?? 0.70);
        setInfIndex(data.indicators.inflation_index ?? 0.55);
        setRateIndex(data.indicators.interest_rate_index ?? 0.65);
      }
    } catch (err: any) {
      setErrorMsg(err.message || 'Error fetching macro regime');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchRegime();
  }, []);

  const handleUpdateIndicators = async () => {
    try {
      setUpdating(true);
      setErrorMsg(null);
      const payload = {
        geopolitical_index: geoIndex,
        inflation_index: infIndex,
        interest_rate_index: rateIndex,
        active_conflicts: ['Eastern Europe Front', 'Middle East Regional Conflict', 'Red Sea Corridor'],
        inflation_rate_yoy: 3.1,
        benchmark_rate: 5.25,
      };

      const res = await fetch(`${apiBaseUrl}/api/v1/macro/regime`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });

      if (!res.ok) throw new Error(`HTTP ${res.status}: Failed to update macro indicators`);
      const updated: MacroRegimeState = await res.json();
      setState(updated);
    } catch (err: any) {
      setErrorMsg(err.message || 'Failed to update indicators');
    } finally {
      setUpdating(false);
    }
  };

  if (loading && !state) {
    return (
      <div className="p-8 text-center text-xs font-mono text-slate-500">
        Loading Dynamic Macroeconomic Regime Engine...
      </div>
    );
  }

  const score = state?.score ?? 0.65;
  const isCrisis = state?.regime === 'CRISIS';
  const isDovish = state?.regime === 'DOVISH_EXPANSION';

  const tier1Pct = state ? Math.round(state.target_tier1_pct * 100) : 20;
  const corePct = state ? Math.round(state.target_core_pct * 100) : 55;
  const alphaPct = state ? Math.round(state.target_alpha_pct * 100) : 25;

  return (
    <div className="space-y-6">
      {/* Top Banner & Active Regime Badge */}
      <div className="p-5 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 shadow-sm flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            <Globe className="w-5 h-5 text-sky-500" />
            <h2 className="text-base font-bold tracking-tight">Real-World Dynamic Macro Allocation Engine</h2>
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1 max-w-2xl">
            Quantitative regime calibration: War & Geopolitical Disruption (40%), Inflation Trajectory (35%), and Central Bank Rate Bias (25%). Programmatically shifts Tier 1 Cash, Tier 2 Core Commodities, and Tier 3 Alpha.
          </p>
        </div>

        <div className="flex items-center gap-3">
          <div
            className={`px-3.5 py-1.5 rounded-lg border text-xs font-bold font-mono tracking-wider flex items-center gap-2 uppercase ${
              isCrisis
                ? 'bg-rose-500/10 text-rose-500 border-rose-500/30 animate-pulse'
                : isDovish
                ? 'bg-emerald-500/10 text-emerald-500 border-emerald-500/30'
                : 'bg-amber-500/10 text-amber-500 border-amber-500/30'
            }`}
          >
            {isCrisis ? <ShieldAlert className="w-4 h-4" /> : isDovish ? <CheckCircle2 className="w-4 h-4" /> : <AlertTriangle className="w-4 h-4" />}
            <span>REGIME: {state?.regime}</span>
          </div>

          <button
            onClick={fetchRegime}
            disabled={loading}
            className="p-2 rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors"
            title="Refresh macro state"
          >
            <RefreshCw className={`w-4 h-4 text-slate-500 ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>

      {errorMsg && (
        <div className="p-3 rounded-lg bg-rose-500/10 border border-rose-500/20 text-rose-600 dark:text-rose-400 text-xs font-mono">
          {errorMsg}
        </div>
      )}

      {/* Grid: Stress Gauge & Dynamic 3-Tier Split */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Card 1: Macro Stress Score Telemetry */}
        <div className="p-5 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 shadow-sm space-y-4">
          <div className="flex items-center justify-between">
            <span className="text-xs font-mono font-semibold uppercase text-slate-500">Macro Stress Index (S)</span>
            <span className="text-xs font-mono text-slate-400">Score Range: [0.0 - 1.0]</span>
          </div>

          <div className="flex items-baseline gap-3">
            <div className="text-4xl font-extrabold font-mono tracking-tight">
              {score.toFixed(3)}
            </div>
            <div className="text-xs font-medium font-mono text-slate-500">
              {isCrisis ? 'Crisis Threshold (≥0.65) Exceeded' : isDovish ? 'Dovish Expansion (<0.35)' : 'Balanced Macro Baseline'}
            </div>
          </div>

          {/* Progress gauge bar */}
          <div className="space-y-1.5">
            <div className="h-3 w-full bg-slate-100 dark:bg-slate-800 rounded-full overflow-hidden relative">
              <div
                style={{ width: `${Math.min(100, Math.max(0, score * 100))}%` }}
                className={`h-full transition-all duration-500 ${
                  isCrisis ? 'bg-gradient-to-r from-orange-500 to-rose-600' : isDovish ? 'bg-emerald-500' : 'bg-amber-500'
                }`}
              />
            </div>
            <div className="flex justify-between text-[10px] font-mono text-slate-400">
              <span>0.0 Dovish</span>
              <span>0.35 Normal Baseline</span>
              <span>0.65 Crisis Threshold</span>
              <span>1.0 War / Shock</span>
            </div>
          </div>

          <div className="p-3 rounded-lg bg-slate-50 dark:bg-slate-800/60 border border-slate-200/60 dark:border-slate-700/60 text-xs font-mono text-slate-700 dark:text-slate-300">
            {state?.description}
          </div>
        </div>

        {/* Card 2: Dynamic 3-Tier Allocation Targets */}
        <div className="p-5 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 shadow-sm space-y-4">
          <div className="flex items-center justify-between">
            <span className="text-xs font-mono font-semibold uppercase text-slate-500">Dynamic 3-Tier Target Split</span>
            <span className="text-xs font-mono text-emerald-500 font-bold">100.0% Fully Hedged</span>
          </div>

          {/* Tri-color Stacked Bar */}
          <div className="space-y-2">
            <div className="h-4 w-full bg-slate-100 dark:bg-slate-800 rounded-full overflow-hidden flex">
              <div
                style={{ width: `${tier1Pct}%` }}
                className="bg-sky-500 h-full transition-all duration-500"
                title={`Tier 1 Cash: ${tier1Pct}%`}
              />
              <div
                style={{ width: `${corePct}%` }}
                className="bg-amber-500 h-full transition-all duration-500"
                title={`Tier 2 Core (Gold/Silver): ${corePct}%`}
              />
              <div
                style={{ width: `${alphaPct}%` }}
                className="bg-indigo-500 h-full transition-all duration-500"
                title={`Tier 3 Tactical Alpha: ${alphaPct}%`}
              />
            </div>

            <div className="grid grid-cols-3 gap-2 text-xs font-mono pt-2">
              <div className="p-2.5 rounded-lg bg-sky-500/10 border border-sky-500/20">
                <span className="text-[10px] text-sky-600 dark:text-sky-400 block font-semibold">Tier 1: Cash Buffer</span>
                <span className="text-lg font-bold text-sky-500">{tier1Pct}%</span>
                <span className="text-[10px] text-slate-400 block mt-0.5">Target: 15% - 25%</span>
              </div>

              <div className="p-2.5 rounded-lg bg-amber-500/10 border border-amber-500/20">
                <span className="text-[10px] text-amber-600 dark:text-amber-400 block font-semibold">Tier 2: Core Metals</span>
                <span className="text-lg font-bold text-amber-500">{corePct}%</span>
                <span className="text-[10px] text-slate-400 block mt-0.5">Target: 35% - 60%</span>
              </div>

              <div className="p-2.5 rounded-lg bg-indigo-500/10 border border-indigo-500/20">
                <span className="text-[10px] text-indigo-600 dark:text-indigo-400 block font-semibold">Tier 3: Tactical Alpha</span>
                <span className="text-lg font-bold text-indigo-500">{alphaPct}%</span>
                <span className="text-[10px] text-slate-400 block mt-0.5">Target: 15% - 50%</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Real-World Macro Indicator Calibration Form */}
      <div className="p-5 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 shadow-sm space-y-4">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 border-b border-slate-100 dark:border-slate-800 pb-3">
          <div>
            <h3 className="text-sm font-bold tracking-tight">Real-World Economic & Geopolitical Calibrator</h3>
            <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
              Simulate or synchronize real economic tension and policy shifts to observe automated rebalancing
            </p>
          </div>

          <button
            onClick={handleUpdateIndicators}
            disabled={updating}
            className="px-4 py-2 rounded-lg bg-gradient-to-r from-sky-500 to-indigo-600 hover:from-sky-600 hover:to-indigo-700 text-white text-xs font-semibold shadow-sm flex items-center gap-1.5 transition-all disabled:opacity-50 self-start sm:self-auto"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${updating ? 'animate-spin' : ''}`} />
            <span>{updating ? 'Recalibrating Tiers...' : 'Apply Macro Shock & Recalibrate'}</span>
          </button>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-6 pt-2">
          {/* Geopolitical Conflict Slider */}
          <div className="space-y-2">
            <div className="flex justify-between text-xs font-mono">
              <span className="text-slate-500">Geopolitical armed conflict (40%):</span>
              <span className="font-bold text-rose-500">{(geoIndex * 100).toFixed(0)}%</span>
            </div>
            <input
              type="range"
              min="0"
              max="1"
              step="0.05"
              value={geoIndex}
              onChange={(e) => setGeoIndex(parseFloat(e.target.value))}
              className="w-full h-2 bg-slate-200 dark:bg-slate-700 rounded-lg appearance-none cursor-pointer accent-rose-500"
            />
            <div className="flex justify-between text-[10px] text-slate-400 font-mono">
              <span>Peace (0.0)</span>
              <span>Regional War (1.0)</span>
            </div>
          </div>

          {/* Inflation Index Slider */}
          <div className="space-y-2">
            <div className="flex justify-between text-xs font-mono">
              <span className="text-slate-500">Inflation CPI trajectory (35%):</span>
              <span className="font-bold text-amber-500">{(infIndex * 100).toFixed(0)}%</span>
            </div>
            <input
              type="range"
              min="0"
              max="1"
              step="0.05"
              value={infIndex}
              onChange={(e) => setInfIndex(parseFloat(e.target.value))}
              className="w-full h-2 bg-slate-200 dark:bg-slate-700 rounded-lg appearance-none cursor-pointer accent-amber-500"
            />
            <div className="flex justify-between text-[10px] text-slate-400 font-mono">
              <span>Sub-2% (0.0)</span>
              <span>Hyper-CPI (1.0)</span>
            </div>
          </div>

          {/* Interest Rate Index Slider */}
          <div className="space-y-2">
            <div className="flex justify-between text-xs font-mono">
              <span className="text-slate-500">Central Bank Rate Bias (25%):</span>
              <span className="font-bold text-sky-500">{(rateIndex * 100).toFixed(0)}%</span>
            </div>
            <input
              type="range"
              min="0"
              max="1"
              step="0.05"
              value={rateIndex}
              onChange={(e) => setRateIndex(parseFloat(e.target.value))}
              className="w-full h-2 bg-slate-200 dark:bg-slate-700 rounded-lg appearance-none cursor-pointer accent-sky-500"
            />
            <div className="flex justify-between text-[10px] text-slate-400 font-mono">
              <span>0.0% Dovish (0.0)</span>
              <span>Hawkish 6%+ (1.0)</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
