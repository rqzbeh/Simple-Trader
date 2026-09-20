import React from 'react';
import { useTheme } from './context/ThemeContext';
import { Sun, Moon, Activity, TrendingUp, Cpu } from 'lucide-react';

export const App: React.FC = () => {
  const { theme, toggleTheme } = useTheme();

  return (
    <div className="min-h-screen bg-slate-50 dark:bg-[#090d16] text-slate-900 dark:text-slate-100 flex flex-col">
      {/* Header */}
      <header className="border-b border-slate-200 dark:border-slate-800 bg-white/80 dark:bg-slate-900/80 backdrop-blur sticky top-0 z-50">
        <div className="max-w-7xl mx-auto px-4 h-16 flex items-center justify-between">
          <div className="flex items-center space-x-3">
            <div className="w-9 h-9 rounded-lg bg-emerald-500/10 border border-emerald-500/30 flex items-center justify-center text-emerald-500">
              <TrendingUp className="w-5 h-5" />
            </div>
            <div>
              <h1 className="font-bold text-lg leading-none tracking-tight">Simple-Trader</h1>
              <span className="text-xs text-slate-500 dark:text-slate-400 font-mono">v2.0 • Pure Go & AI Engine</span>
            </div>
          </div>

          <div className="flex items-center space-x-4">
            <div className="flex items-center space-x-2 text-xs px-3 py-1.5 rounded-full bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20">
              <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse"></span>
              <span className="font-medium">System Operational</span>
            </div>

            <button
              onClick={toggleTheme}
              className="p-2 rounded-lg border border-slate-200 dark:border-slate-800 hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors"
              title="Toggle Theme"
            >
              {theme === 'dark' ? <Sun className="w-4 h-4 text-amber-400" /> : <Moon className="w-4 h-4 text-slate-600" />}
            </button>
          </div>
        </div>
      </header>

      {/* Main Content Area */}
      <main className="flex-1 max-w-7xl w-full mx-auto p-4 space-y-6">
        {/* Metric Cards Banner */}
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
          <div className="p-4 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 shadow-sm">
            <div className="flex items-center justify-between text-xs text-slate-500 dark:text-slate-400 mb-1">
              <span>Total Equity</span>
              <Activity className="w-4 h-4 text-emerald-500" />
            </div>
            <div className="text-2xl font-bold font-mono">$102,450.00</div>
            <div className="text-xs text-emerald-500 mt-1 font-medium">+2.45% all-time</div>
          </div>

          <div className="p-4 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 shadow-sm">
            <div className="flex items-center justify-between text-xs text-slate-500 dark:text-slate-400 mb-1">
              <span>Core Allocation (Gold/Silver)</span>
              <span className="text-amber-500 font-bold">60%</span>
            </div>
            <div className="text-2xl font-bold font-mono">$61,470.00</div>
            <div className="text-xs text-slate-500 dark:text-slate-400 mt-1">Target: 60.0%</div>
          </div>

          <div className="p-4 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 shadow-sm">
            <div className="flex items-center justify-between text-xs text-slate-500 dark:text-slate-400 mb-1">
              <span>Alpha Allocation (Crypto/Forex)</span>
              <span className="text-indigo-500 font-bold">40%</span>
            </div>
            <div className="text-2xl font-bold font-mono">$40,980.00</div>
            <div className="text-xs text-slate-500 dark:text-slate-400 mt-1">Target: 40.0%</div>
          </div>

          <div className="p-4 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 shadow-sm">
            <div className="flex items-center justify-between text-xs text-slate-500 dark:text-slate-400 mb-1">
              <span>AI Learning State</span>
              <Cpu className="w-4 h-4 text-sky-500" />
            </div>
            <div className="text-2xl font-bold text-sky-500">Active</div>
            <div className="text-xs text-slate-500 dark:text-slate-400 mt-1">Dynamic Weights Online</div>
          </div>
        </div>

        {/* Placeholder placeholder container for charts & matrix */}
        <div id="chart-viewport" className="p-6 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 shadow-sm text-center">
          <h2 className="text-lg font-semibold mb-2">Live Market Terminal & AI Confluence Workspace</h2>
          <p className="text-sm text-slate-500 dark:text-slate-400">
            Real-time SSE event stream connected to Go backend. Liquid Core (XAU/USD, XAG/USD) and Alpha (BTC/USD, ETH/USD, SOL/USD, EUR/USD, WTI/USD) asset feeds ready.
          </p>
        </div>
      </main>
    </div>
  );
};

export default App;
