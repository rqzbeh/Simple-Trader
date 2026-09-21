import React, { useState, useEffect } from 'react';
import { useTheme } from './context/ThemeContext';
import { useAuth } from './context/AuthContext';
import { Sun, Moon, Activity, TrendingUp, Layers, SlidersHorizontal, Users, Newspaper, Filter, MessageSquare, LogOut, ShieldCheck, Menu, X, Brain, Wallet } from 'lucide-react';
import { useSSE } from './hooks/useSSE';
import { AssetTickerGrid } from './components/AssetTickerGrid';
import { TradingViewChart } from './components/TradingViewChart';
import { AllocationGauge } from './components/AllocationGauge';
import { PositionsTable } from './components/PositionsTable';
import { AISignalFeed } from './components/AISignalFeed';
import { AIWeightMatrix, INITIAL_WEIGHTS } from './components/AIWeightMatrix';
import { MLTrainingView } from './components/MLTrainingView';
import { InvestorLedgerView } from './components/InvestorLedgerView';
import { NewsStreamView } from './components/NewsStreamView';
import { ScreenerView } from './components/ScreenerView';
import { TelegramConfigModal } from './components/TelegramConfigModal';
import { LoginModal } from './components/LoginModal';
import { PWAInstallBanner } from './components/PWAInstallBanner';
import { IOSInstallModal } from './components/IOSInstallModal';
import { AssetInfo, CandleData, TradePosition, IndicatorWeights } from './types';

export const App: React.FC = () => {
  const { theme, toggleTheme } = useTheme();
  const { isAuthenticated, tokenMasked, logout } = useAuth();
  const { isConnected, assets, positions, summary, setPositions } = useSSE();
  const [selectedSymbol, setSelectedSymbol] = useState<string>('BTC/USDT');
  const [activeTab, setActiveTab] = useState<'terminal' | 'investors' | 'screener' | 'news' | 'ai_weights' | 'ml'>('terminal');
  const [weights, setWeights] = useState<IndicatorWeights>(INITIAL_WEIGHTS);
  const [isTelegramModalOpen, setIsTelegramModalOpen] = useState<boolean>(false);
  const [isIOSGuideOpen, setIsIOSGuideOpen] = useState<boolean>(false);
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState<boolean>(false);
  const [candleData, setCandleData] = useState<CandleData[]>([]);

  // Fetch authentic online exchange Kline data for selected symbol
  useEffect(() => {
    let isMounted = true;
    async function fetchKlines() {
      try {
        const res = await fetch(`/api/v1/klines?symbol=${encodeURIComponent(selectedSymbol)}&interval=1h&limit=48`);
        if (res.ok) {
          const data = await res.json();
          if (isMounted && Array.isArray(data)) {
            setCandleData(data);
          }
        }
      } catch (err) {
        console.error('Failed to fetch authentic klines', err);
      }
    }
    fetchKlines();
    return () => {
      isMounted = false;
    };
  }, [selectedSymbol]);

  // Dynamic Mark-to-Market Total Return Calculation from online summary
  const baselineEquity = summary.initialEquity && summary.initialEquity > 0 ? summary.initialEquity : 10000.0;
  const returnPct = baselineEquity > 0 ? ((summary.totalEquity - baselineEquity) / baselineEquity) * 100 : 0;
  const isPositiveReturn = returnPct >= 0;

  const selectedAsset = assets.find((a: AssetInfo) => a.symbol === selectedSymbol) || assets[0];

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

            {/* Navigation Tabs (Desktop) */}
            <nav className="hidden lg:flex items-center space-x-1 p-1 bg-slate-100 dark:bg-slate-800/60 rounded-xl">
              <button
                onClick={() => setActiveTab('terminal')}
                className={`px-3 py-1.5 rounded-lg text-xs font-semibold font-mono transition-all duration-150 flex items-center space-x-1.5 ${
                  activeTab === 'terminal'
                    ? 'bg-white dark:bg-slate-900 text-slate-900 dark:text-slate-100 shadow-sm'
                    : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'
                }`}
              >
                <TrendingUp className="w-3.5 h-3.5 text-emerald-500" />
                <span>Terminal</span>
              </button>
              <button
                onClick={() => setActiveTab('investors')}
                className={`px-3 py-1.5 rounded-lg text-xs font-semibold font-mono transition-all duration-150 flex items-center space-x-1.5 ${
                  activeTab === 'investors'
                    ? 'bg-white dark:bg-slate-900 text-slate-900 dark:text-slate-100 shadow-sm'
                    : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'
                }`}
              >
                <Users className="w-3.5 h-3.5 text-sky-500" />
                <span>Investor Ledger</span>
              </button>
              <button
                onClick={() => setActiveTab('screener')}
                className={`px-3 py-1.5 rounded-lg text-xs font-semibold font-mono transition-all duration-150 flex items-center space-x-1.5 ${
                  activeTab === 'screener'
                    ? 'bg-white dark:bg-slate-900 text-slate-900 dark:text-slate-100 shadow-sm'
                    : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'
                }`}
              >
                <Filter className="w-3.5 h-3.5 text-amber-500" />
                <span>Liquid Screener</span>
              </button>
              <button
                onClick={() => setActiveTab('news')}
                className={`px-3 py-1.5 rounded-lg text-xs font-semibold font-mono transition-all duration-150 flex items-center space-x-1.5 ${
                  activeTab === 'news'
                    ? 'bg-white dark:bg-slate-900 text-slate-900 dark:text-slate-100 shadow-sm'
                    : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'
                }`}
              >
                <Newspaper className="w-3.5 h-3.5 text-rose-500" />
                <span>News Trading</span>
              </button>
              <button
                onClick={() => setActiveTab('ai_weights')}
                className={`px-3 py-1.5 rounded-lg text-xs font-semibold font-mono transition-all duration-150 flex items-center space-x-1.5 ${
                  activeTab === 'ai_weights'
                    ? 'bg-white dark:bg-slate-900 text-slate-900 dark:text-slate-100 shadow-sm'
                    : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'
                }`}
              >
                <SlidersHorizontal className="w-3.5 h-3.5 text-purple-500" />
                <span>Weights</span>
              </button>
              <button
                onClick={() => setActiveTab('ml')}
                className={`px-3 py-1.5 rounded-lg text-xs font-semibold font-mono transition-all duration-150 flex items-center space-x-1.5 ${
                  activeTab === 'ml'
                    ? 'bg-white dark:bg-slate-900 text-slate-900 dark:text-slate-100 shadow-sm'
                    : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-300'
                }`}
              >
                <Brain className="w-3.5 h-3.5 text-indigo-500" />
                <span>ML Engine</span>
              </button>
            </nav>
          </div>

          <div className="flex items-center space-x-2 sm:space-x-3">
            {/* Live SSE status indicator */}
            <div
              className={`flex items-center space-x-2 text-xs px-2.5 sm:px-3 py-1.5 rounded-full border ${
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
              <span className="font-medium font-mono text-[11px] sm:text-xs">
                {isConnected ? 'SSE Live Feed' : 'Connecting SSE...'}
              </span>
            </div>

            {/* Telegram Bot Config Trigger */}
            <button
              onClick={() => setIsTelegramModalOpen(true)}
              className="p-2 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors shadow-sm flex items-center gap-1.5 text-xs font-semibold text-slate-700 dark:text-slate-200"
              title="Configure Telegram Bot"
            >
              <MessageSquare className="w-4 h-4 text-sky-500" />
              <span className="hidden xl:inline font-mono">Telegram Bot</span>
            </button>

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

            {/* Admin Session Indicator & Logout */}
            {isAuthenticated && (
              <div className="flex items-center gap-2 pl-2 border-l border-slate-200 dark:border-slate-800">
                <div className="hidden lg:flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-emerald-500/10 border border-emerald-500/20 text-emerald-400 text-xs font-mono">
                  <ShieldCheck className="w-3.5 h-3.5 text-emerald-500" />
                  <span>Admin</span>
                  {tokenMasked && <span className="text-[10px] text-emerald-600 dark:text-emerald-400">({tokenMasked})</span>}
                </div>
                <button
                  onClick={logout}
                  className="p-2 rounded-xl border border-rose-500/20 bg-rose-500/10 hover:bg-rose-500/20 text-rose-400 transition-colors shadow-sm flex items-center gap-1 text-xs font-mono"
                  title="Logout Session"
                >
                  <LogOut className="w-4 h-4" />
                  <span className="hidden sm:inline">Logout</span>
                </button>
              </div>
            )}

            {/* Mobile / Tablet Menu Hamburger Button */}
            <button
              onClick={() => setIsMobileMenuOpen(!isMobileMenuOpen)}
              className="lg:hidden p-2 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 hover:bg-slate-100 dark:hover:bg-slate-800 text-slate-700 dark:text-slate-200 transition-colors shadow-sm focus:outline-none focus:ring-2 focus:ring-sky-500"
              aria-label="Toggle Navigation Menu"
            >
              {isMobileMenuOpen ? <X className="w-5 h-5 text-rose-500" /> : <Menu className="w-5 h-5" />}
            </button>
          </div>
        </div>

        {/* Mobile / Tablet Navigation Dropdown Drawer */}
        {isMobileMenuOpen && (
          <div className="lg:hidden border-t border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 px-4 py-3 shadow-lg animate-in slide-in-from-top-2 duration-150">
            <div className="grid grid-cols-2 gap-2">
              <button
                onClick={() => {
                  setActiveTab('terminal');
                  setIsMobileMenuOpen(false);
                }}
                className={`p-2.5 rounded-xl text-xs font-semibold font-mono flex items-center space-x-2 transition-colors min-h-[44px] ${
                  activeTab === 'terminal'
                    ? 'bg-sky-500/15 text-sky-500 border border-sky-500/30'
                    : 'bg-slate-50 dark:bg-slate-800/60 text-slate-700 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'
                }`}
              >
                <TrendingUp className="w-4 h-4 text-emerald-500" />
                <span>Terminal</span>
              </button>

              <button
                onClick={() => {
                  setActiveTab('investors');
                  setIsMobileMenuOpen(false);
                }}
                className={`p-2.5 rounded-xl text-xs font-semibold font-mono flex items-center space-x-2 transition-colors min-h-[44px] ${
                  activeTab === 'investors'
                    ? 'bg-sky-500/15 text-sky-500 border border-sky-500/30'
                    : 'bg-slate-50 dark:bg-slate-800/60 text-slate-700 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'
                }`}
              >
                <Users className="w-4 h-4 text-sky-500" />
                <span>Investor Ledger</span>
              </button>

              <button
                onClick={() => {
                  setActiveTab('screener');
                  setIsMobileMenuOpen(false);
                }}
                className={`p-2.5 rounded-xl text-xs font-semibold font-mono flex items-center space-x-2 transition-colors min-h-[44px] ${
                  activeTab === 'screener'
                    ? 'bg-sky-500/15 text-sky-500 border border-sky-500/30'
                    : 'bg-slate-50 dark:bg-slate-800/60 text-slate-700 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'
                }`}
              >
                <Filter className="w-4 h-4 text-amber-500" />
                <span>Screener</span>
              </button>

              <button
                onClick={() => {
                  setActiveTab('news');
                  setIsMobileMenuOpen(false);
                }}
                className={`p-2.5 rounded-xl text-xs font-semibold font-mono flex items-center space-x-2 transition-colors min-h-[44px] ${
                  activeTab === 'news'
                    ? 'bg-sky-500/15 text-sky-500 border border-sky-500/30'
                    : 'bg-slate-50 dark:bg-slate-800/60 text-slate-700 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'
                }`}
              >
                <Newspaper className="w-4 h-4 text-rose-500" />
                <span>News Stream</span>
              </button>

              <button
                onClick={() => {
                  setActiveTab('ai_weights');
                  setIsMobileMenuOpen(false);
                }}
                className={`p-2.5 rounded-xl text-xs font-semibold font-mono flex items-center space-x-2 transition-colors min-h-[44px] ${
                  activeTab === 'ai_weights'
                    ? 'bg-sky-500/15 text-sky-500 border border-sky-500/30'
                    : 'bg-slate-50 dark:bg-slate-800/60 text-slate-700 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'
                }`}
              >
                <SlidersHorizontal className="w-4 h-4 text-purple-500" />
                <span>AI Weights</span>
              </button>

              <button
                onClick={() => {
                  setActiveTab('ml');
                  setIsMobileMenuOpen(false);
                }}
                className={`p-2.5 rounded-xl text-xs font-semibold font-mono flex items-center space-x-2 transition-colors min-h-[44px] ${
                  activeTab === 'ml'
                    ? 'bg-sky-500/15 text-sky-500 border border-sky-500/30'
                    : 'bg-slate-50 dark:bg-slate-800/60 text-slate-700 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'
                }`}
              >
                <Brain className="w-4 h-4 text-indigo-500" />
                <span>ML Engine</span>
              </button>
            </div>
          </div>
        )}
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
            <div className={`text-xs mt-1 font-medium font-mono flex items-center ${isPositiveReturn ? 'text-emerald-500' : 'text-rose-500'}`}>
              <span>{isPositiveReturn ? '+' : ''}{returnPct.toFixed(2)}% All-Time</span>
              <span className="mx-1.5 text-slate-300 dark:text-slate-700">•</span>
              <span className="text-slate-400">Paper Trading</span>
            </div>
          </div>

          <div className="p-4 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 shadow-sm flex flex-col justify-between">
            <div className="flex items-center justify-between text-xs text-slate-500 dark:text-slate-400 mb-1">
              <span>Core Allocation (Gold/Reserves)</span>
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
              <span>Alpha Allocation (Tactical Crypto Futures)</span>
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
              <span>Tier 1 Cash & Margin Buffer</span>
              <Wallet className="w-4 h-4 text-sky-500" />
            </div>
            <div className="text-2xl font-bold font-mono tracking-tight text-sky-500">
              ${summary.cash.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
            </div>
            <div className="text-xs text-slate-500 dark:text-slate-400 mt-1 font-mono">
              Peak DD: <span className={summary.drawdownPct > 5 ? 'text-rose-500 font-bold' : 'text-emerald-500 font-bold'}>{summary.drawdownPct.toFixed(2)}%</span> (Peak: ${summary.peakEquity.toLocaleString()})
            </div>
          </div>
        </div>

        {/* Dynamic View Mode: Terminal vs AI Weights */}
        {activeTab === 'terminal' ? (
          <>
            {/* Global Asset Ticker Selector Grid */}
            <div className="space-y-2">
              <div className="flex items-center justify-between text-xs text-slate-500 dark:text-slate-400 font-mono px-1">
                <span className="font-semibold text-slate-700 dark:text-slate-300">Market Assets</span>
                <span>Live Quotes (Binance Stream)</span>
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
                <AISignalFeed selectedSymbol={selectedSymbol} currentPrice={selectedAsset.price} />
              </div>
            </div>

            {/* Allocation Gauge & Active Positions Table */}
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
              <div className="lg:col-span-1">
                <AllocationGauge summary={summary} />
              </div>
              <div className="lg:col-span-2 space-y-2">
                <div className="flex items-center justify-between text-xs text-slate-500 dark:text-slate-400 font-mono px-1">
                  <span className="flex items-center space-x-1.5 font-semibold text-slate-700 dark:text-slate-300">
                    <Layers className="w-3.5 h-3.5 text-sky-500" />
                    <span>Active Positions</span>
                  </span>
                  <span>Automated SL/TP</span>
                </div>
                <PositionsTable
                  positions={positions}
                  onClosePosition={handleClosePosition}
                />
              </div>
            </div>
          </>
        ) : activeTab === 'investors' ? (
          <div className="space-y-4">
            <div className="flex items-center justify-between px-1">
              <h2 className="text-sm font-bold uppercase tracking-wider font-mono text-slate-800 dark:text-slate-200">
                Institutional Investor Capital Ledger & NAV Accounting
              </h2>
              <span className="text-xs text-slate-400 font-mono">
                Tier 1 Instant Liquidity • Non-Diluting NAV Pool
              </span>
            </div>
            <InvestorLedgerView />
          </div>
        ) : activeTab === 'screener' ? (
          <div className="space-y-4">
            <div className="flex items-center justify-between px-1">
              <h2 className="text-sm font-bold uppercase tracking-wider font-mono text-slate-800 dark:text-slate-200">
                Dynamic Liquid Crypto Screener
              </h2>
              <span className="text-xs text-slate-400 font-mono">
                Slippage Defense • $50M 24h Volume • 10 bps Spread
              </span>
            </div>
            <ScreenerView />
          </div>
        ) : activeTab === 'news' ? (
          <div className="space-y-4">
            <div className="flex items-center justify-between px-1">
              <h2 className="text-sm font-bold uppercase tracking-wider font-mono text-slate-800 dark:text-slate-200">
                Real-Time Macro & Crypto News Trading
              </h2>
              <span className="text-xs text-slate-400 font-mono">
                SHA-256 Deduplication • Tanh Polarity NLP Scoring
              </span>
            </div>
            <NewsStreamView />
          </div>
        ) : activeTab === 'ml' ? (
          <div className="space-y-4">
            <div className="flex items-center justify-between px-1">
              <h2 className="text-sm font-bold uppercase tracking-wider font-mono text-slate-800 dark:text-slate-200">
                GPU & Statistical Deep Learning Model Training
              </h2>
              <span className="text-xs text-slate-400 font-mono">
                CUDA Acceleration • Authentic Binance Kline Data • Thompson Sampling
              </span>
            </div>
            <MLTrainingView />
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

      {/* Telegram Bot Configuration Modal */}
      <TelegramConfigModal
        isOpen={isTelegramModalOpen}
        onClose={() => setIsTelegramModalOpen(false)}
      />

      {/* PWA Floating Install Banner (US5) */}
      <PWAInstallBanner onShowIOSGuide={() => setIsIOSGuideOpen(true)} />

      {/* iOS Safari Home Screen Installation Modal Guide (US5) */}
      <IOSInstallModal
        isOpen={isIOSGuideOpen}
        onClose={() => setIsIOSGuideOpen(false)}
      />

      {/* Admin Authentication Modal Gate */}
      {!isAuthenticated && <LoginModal />}
    </div>
  );
};

export default App;
