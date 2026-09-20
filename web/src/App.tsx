import React, { useState } from 'react';
import { useTheme } from './context/ThemeContext';
import { Sun, Moon, Activity, TrendingUp, Cpu, Layers, SlidersHorizontal, BarChart2 } from 'lucide-react';
import { useSSE } from './hooks/useSSE';
import { AssetTickerGrid } from './components/AssetTickerGrid';
import { TradingViewChart } from './components/TradingViewChart';
import { AllocationGauge } from './components/AllocationGauge';
import { PositionsTable } from './components/PositionsTable';
import { AISignalFeed } from './components/AISignalFeed';
import { AIWeightMatrix, INITIAL_WEIGHTS } from './components/AIWeightMatrix';
import { QuantDashboardView } from './components/QuantDashboardView';
import { MicrostructureCard } from './components/MicrostructureCard';
import { MacroCalendarPanel } from './components/MacroCalendarPanel';
import { AssetInfo, CandleData, TradePosition, IndicatorWeights, MicrostructureState, MacroCalendarEvent } from './types';

// Deterministic candle data generator for visual demonstration
function generateCandles(basePrice: number): CandleData[] {
  const candles: CandleData[] = [];
  let current = basePrice;
  const now = Math.floor(Date.now() / 1000);
  const periodSeconds = 3600; // 1h

  for (let i = 48; i >= 0; i--) {
    const time = now - i * periodSeconds;
    const variation = (Math.sin(i / 3) * 0.008 + (Math.random() - 0.48) * 0.01) * basePrice;
    const open = current;
    const close = open + variation;
    const high = Math.max(open, close) + Math.random() * 0.004 * basePrice;
    const low = Math.min(open, close) - Math.random() * 0.004 * basePrice;
    candles.push({ time, open, high, low, close });
    current = close;
  }
  return candles;
}

export const App: React.FC = () => {
  const { theme, toggleTheme } = useTheme();
  const { isConnected, assets, signals, positions, summary, setPositions } = useSSE();
  const [selectedSymbol, setSelectedSymbol] = useState<string>('XAU/USD');
  const [activeTab, setActiveTab] = useState<'terminal' | 'ai_weights' | 'quant'>('terminal');
  const [weights, setWeights] = useState<IndicatorWeights>(INITIAL_WEIGHTS);

  // Microstructure state for selected asset
  const [microState] = useState<MicrostructureState>({
    symbol: selectedSymbol,
    obi: 0.38,
    cvd: 4250,
    divergence: 'BULLISH_ABSORPTION',
    regime: 'NORMAL_TRENDING',
    volRatio: 1.12,
  });

  // Macro events for circuit breaker monitoring
  const [macroEvents] = useState<MacroCalendarEvent[]>([
    {
      id: 'FOMC-001',
      title: 'FOMC Rate Decision',
      currency: 'USD',
      impact: 'HIGH',
      scheduled_at: new Date(Date.now() + 1800000).toISOString(), // in 30 mins
      forecast: '5.25%',
      previous: '5.50%',
    },
    {
      id: 'CPI-002',
      title: 'US Core CPI YoY',
      currency: 'USD',
      impact: 'HIGH',
      scheduled_at: new Date(Date.now() + 14400000).toISOString(),
      forecast: '3.1%',
      previous: '3.2%',
    },
  ]);

  const selectedAsset = assets.find((a: AssetInfo) => a.symbol === selectedSymbol) || assets[0];
  const candleData = React.useMemo(() => {
    return generateCandles(selectedAsset.price);
  }, [selectedAsset.symbol]);

  const handleClosePosition = (id: string) => {
    setPositions((prev: TradePosition[]) => prev.filter((p: TradePosition) => p.id !== id));
  };

  return (
    <div className="min-h-screen bg-slate-50 dark:bg-[#070b14] text-slate-900 dark:text-slate-100 flex flex-col font-sans selection:bg-sky-500 selection:text-white transition-colors duration-200">
      {/* Top Navigation Bar */}
      <header className="border-b border-slate-200 dark:border-slate-800 bg-white/80 dark:bg-slate-900/80 backdrop-blur sticky top-0 z-50">
        <div className="max-w-7xl mx-auto px-4 h-16 flex items-center justify-between">
          <div className="flex items-center space-x-6">
            <div className="flex items-center space-x-3">
              <div className="w-10 h-10 rounded-xl bg-gradient-to-tr from-sky-500 to-indigo-600 flex items-center justify-center text-white shadow-md shadow-sky-500/20">
                <TrendingUp className="w-5 h-5" />
              </div>
              <div>
                <div className="flex items-center space-x-2">
                  <h1 className="font-bold text-lg leading-tight tracking-tight">Simple-Trader</h1>
                  <span className="text-[10px] uppercase font-bold tracking-wider px-1.5 py-0.5 rounded bg-sky-500/10 text-sky-500 border border-sky-500/20">
                    v2.0 PWA
                  </span>
                </div>
                <span className="text-xs text-slate-500 dark:text-slate-400 font-mono">
                  Autonomous Go & AI Quant Terminal
                </span>
              </div>
            </div>

            {/* Navigation Tabs */}
            <nav className="hidden sm:flex items-center space-x-1 p-1 bg-slate-100 dark:bg-slate-800/60 rounded-xl">
              <button
                onClick={() => setActiveTab('terminal')}
                className={`px-3 py-1.5 rounded-lg text-xs font-semibold font-mono transition-all duration-150 flex items-center space-x-1.5 ${
                  activeTab === 'terminal'
                    ? 'bg-white dark:bg-slate-900 text-slate-900 dark:text-slate-100 shadow-sm'
                    : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'
                }`}
              >
                <TrendingUp className="w-3.5 h-3.5 text-emerald-500" />
                <span>Market Terminal</span>
              </button>
              <button
                onClick={() => setActiveTab('quant')}
                className={`px-3 py-1.5 rounded-lg text-xs font-semibold font-mono transition-all duration-150 flex items-center space-x-1.5 ${
                  activeTab === 'quant'
                    ? 'bg-white dark:bg-slate-900 text-slate-900 dark:text-slate-100 shadow-sm'
                    : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'
                }`}
              >
                <BarChart2 className="w-3.5 h-3.5 text-indigo-500" />
                <span>Quant Suite & Backtester</span>
              </button>
              <button
                onClick={() => setActiveTab('ai_weights')}
                className={`px-3 py-1.5 rounded-lg text-xs font-semibold font-mono transition-all duration-150 flex items-center space-x-1.5 ${
                  activeTab === 'ai_weights'
                    ? 'bg-white dark:bg-slate-900 text-slate-900 dark:text-slate-100 shadow-sm'
                    : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'
                }`}
              >
                <SlidersHorizontal className="w-3.5 h-3.5 text-sky-500" />
                <span>AI Weight Heatmap</span>
              </button>
            </nav>
          </div>

          <div className="flex items-center space-x-3">
            {/* Live SSE status indicator */}
            <div
              className={`flex items-center space-x-2 text-xs px-3 py-1.5 rounded-full border ${
                isConnected
                  ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/20'
                  : 'bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/20'
              }`}
            >
              <span
                className={`w-2 h-2 rounded-full ${
                  isConnected ? 'bg-emerald-500 animate-pulse' : 'bg-amber-500'
                }`}
              ></span>
              <span className="font-medium font-mono">
                {isConnected ? 'SSE Live Feed' : 'Connecting SSE...'}
              </span>
            </div>

            {/* Dark / Light Mode Toggle */}
            <button
              onClick={toggleTheme}
              className="p-2 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors shadow-sm"
              title="Toggle theme"
            >
              {theme === 'dark' ? (
                <Sun className="w-4 h-4 text-amber-400" />
              ) : (
                <Moon className="w-4 h-4 text-slate-600" />
              )}
            </button>
          </div>
        </div>
      </header>

      {/* Main Container */}
      <main className="flex-1 max-w-7xl w-full mx-auto p-4 sm:p-6 space-y-6">
        {/* Metric Cards Banner */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          <div className="p-4 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 shadow-sm flex flex-col justify-between">
            <div className="flex items-center justify-between text-xs text-slate-500 dark:text-slate-400 mb-1">
              <span>Total Portfolio Equity</span>
              <Activity className="w-4 h-4 text-emerald-500" />
            </div>
            <div className="text-2xl font-bold font-mono tracking-tight">
              ${summary.totalEquity.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
            </div>
            <div className="text-xs text-emerald-500 mt-1 font-medium font-mono flex items-center">
              <span>+2.45% All-Time</span>
              <span className="mx-1.5 text-slate-300 dark:text-slate-700">•</span>
              <span className="text-slate-400">Paper Trading</span>
            </div>
          </div>

          <div className="p-4 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 shadow-sm flex flex-col justify-between">
            <div className="flex items-center justify-between text-xs text-slate-500 dark:text-slate-400 mb-1">
              <span>Core Allocation (Gold/Silver)</span>
              <span className="text-amber-500 font-bold text-xs">Target: 60%</span>
            </div>
            <div className="text-2xl font-bold font-mono tracking-tight text-amber-600 dark:text-amber-400">
              ${summary.coreEquity.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
            </div>
            <div className="text-xs text-slate-500 dark:text-slate-400 mt-1 font-mono">
              Current: {((summary.coreEquity / summary.totalEquity) * 100).toFixed(1)}% of total
            </div>
          </div>

          <div className="p-4 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 shadow-sm flex flex-col justify-between">
            <div className="flex items-center justify-between text-xs text-slate-500 dark:text-slate-400 mb-1">
              <span>Alpha Allocation (Crypto/Forex/Oil)</span>
              <span className="text-indigo-500 font-bold text-xs">Target: 40%</span>
            </div>
            <div className="text-2xl font-bold font-mono tracking-tight text-indigo-600 dark:text-indigo-400">
              ${summary.alphaEquity.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
            </div>
            <div className="text-xs text-slate-500 dark:text-slate-400 mt-1 font-mono">
              Current: {((summary.alphaEquity / summary.totalEquity) * 100).toFixed(1)}% of total
            </div>
          </div>

          <div className="p-4 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 shadow-sm flex flex-col justify-between">
            <div className="flex items-center justify-between text-xs text-slate-500 dark:text-slate-400 mb-1">
              <span>AI Learning & Regret Minimizer</span>
              <Cpu className="w-4 h-4 text-sky-500" />
            </div>
            <div className="text-2xl font-bold font-mono tracking-tight text-sky-500">
              Active Online
            </div>
            <div className="text-xs text-slate-500 dark:text-slate-400 mt-1 font-mono">
              Dynamic Weights Calibrated
            </div>
          </div>
        </div>

        {/* Dynamic View Mode: Terminal vs AI Weights */}
        {activeTab === 'terminal' ? (
          <>
            {/* Global Asset Ticker Selector Grid */}
            <div className="space-y-2">
              <div className="flex items-center justify-between text-xs text-slate-500 dark:text-slate-400 font-mono px-1">
                <span>SELECT GLOBAL ASSET TICKER:</span>
                <span>REAL-TIME QUOTES</span>
              </div>
              <AssetTickerGrid
                assets={assets}
                selectedSymbol={selectedSymbol}
                onSelectSymbol={setSelectedSymbol}
              />
            </div>

            {/* Chart and AI Signal Matrix View */}
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
              <div className="lg:col-span-2">
                <TradingViewChart symbol={selectedSymbol} data={candleData} />
              </div>
              <div className="lg:col-span-1">
                <AISignalFeed signals={signals} />
              </div>
            </div>

            {/* Allocation Gauge & Active Positions Table */}
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
              <div className="lg:col-span-1">
                <AllocationGauge summary={summary} />
              </div>
              <div className="lg:col-span-2 space-y-2">
                <div className="flex items-center justify-between text-xs text-slate-500 dark:text-slate-400 font-mono px-1">
                  <span className="flex items-center space-x-1.5">
                    <Layers className="w-3.5 h-3.5 text-sky-500" />
                    <span>OPEN POSITIONS & RISK PARAMETERS</span>
                  </span>
                  <span>SL/TP AUTOMATED EXITS</span>
                </div>
                <PositionsTable
                  positions={positions}
                  onClosePosition={handleClosePosition}
                />
              </div>
            </div>

            {/* Institutional Microstructure & Macro Circuit Breaker Panels */}
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
              <MicrostructureCard state={microState} />
              <MacroCalendarPanel events={macroEvents} activeSymbol={selectedSymbol} />
            </div>
          </>
        ) : activeTab === 'quant' ? (
          <div className="space-y-4">
            <div className="flex items-center justify-between px-1">
              <h2 className="text-sm font-bold uppercase tracking-wider font-mono text-slate-800 dark:text-slate-200">
                Institutional Quant Analytics & Simulation Lab
              </h2>
              <span className="text-xs text-slate-400 font-mono">
                Order Flow • Macro Halt • Monte Carlo Engine
              </span>
            </div>
            <QuantDashboardView symbol={selectedSymbol} />
          </div>
        ) : (
          <div className="space-y-4">
            <div className="flex items-center justify-between px-1">
              <h2 className="text-sm font-bold uppercase tracking-wider font-mono text-slate-800 dark:text-slate-200">
                AI Confluence & Dynamic Indicator Weights Calibration
              </h2>
              <span className="text-xs text-slate-400 font-mono">
                Continuous In-Context Calibration
              </span>
            </div>
            <AIWeightMatrix
              weights={weights}
              onUpdateWeights={(updated) => setWeights(updated)}
            />
          </div>
        )}
      </main>
    </div>
  );
};

export default App;
