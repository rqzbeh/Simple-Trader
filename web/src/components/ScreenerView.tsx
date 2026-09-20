import React, { useState, useEffect } from 'react';
import { Filter, CheckCircle, XCircle, RefreshCw, AlertCircle } from 'lucide-react';
import { ScreenedAsset } from '../types';

export const ScreenerView: React.FC = () => {
  const [assets, setAssets] = useState<ScreenedAsset[]>([]);
  const [activeUniverse, setActiveUniverse] = useState<string[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  const fetchScreener = async () => {
    try {
      setLoading(true);
      setErrorMsg(null);
      const res = await fetch('/api/v1/market/screener');
      if (!res.ok) {
        throw new Error(`Failed to load screener data: HTTP ${res.status}`);
      }
      const data = await res.json();
      setAssets(data.assets || []);
      setActiveUniverse(data.active_universe || []);
    } catch (err: any) {
      console.error('Failed fetching screener:', err);
      setErrorMsg(err.message || 'Error fetching dynamic screener data');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchScreener();
    const interval = setInterval(fetchScreener, 60000); // 1 minute auto-refresh
    return () => clearInterval(interval);
  }, []);

  return (
    <div className="space-y-4">
      {/* Overview & Rule Banner */}
      <div className="p-4 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 shadow-sm flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div>
          <div className="flex items-center space-x-2">
            <Filter className="w-4 h-4 text-sky-500" />
            <h3 className="text-sm font-bold font-mono text-slate-900 dark:text-slate-100">
              Dynamic Liquid Crypto Screener
            </h3>
            <span className="text-[10px] font-mono px-2 py-0.5 rounded bg-sky-500/10 text-sky-500 border border-sky-500/20 font-bold">
              Active Slippage Defense
            </span>
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
            Institutional criteria: 24h Volume &ge; $50,000,000 USD and Bid-Ask Spread &le; 10 bps to prevent quadratic slippage impact.
          </p>
        </div>

        <div className="flex items-center space-x-3">
          <div className="text-xs font-mono text-slate-500 dark:text-slate-400">
            Qualified Assets: <span className="font-bold text-sky-500">{activeUniverse.length}</span>
          </div>
          <button
            onClick={fetchScreener}
            disabled={loading}
            className="p-2 rounded-lg border border-slate-200 dark:border-slate-800 hover:bg-slate-100 dark:hover:bg-slate-800 text-slate-600 dark:text-slate-300 transition-colors"
            title="Re-run Screening Cycle"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>

      {errorMsg && (
        <div className="p-3 bg-rose-500/10 border border-rose-500/20 text-rose-600 dark:text-rose-400 rounded-xl text-xs flex items-center space-x-2">
          <AlertCircle className="w-4 h-4 shrink-0" />
          <span>{errorMsg}</span>
        </div>
      )}

      {/* Screened Assets Table */}
      <div className="border border-slate-200 dark:border-slate-800 rounded-xl overflow-hidden bg-white dark:bg-slate-900/60 shadow-sm">
        <div className="overflow-x-auto">
          <table className="w-full text-left border-collapse text-xs font-mono">
            <thead>
              <tr className="border-b border-slate-200 dark:border-slate-800 bg-slate-50/50 dark:bg-slate-800/30 text-slate-500 dark:text-slate-400">
                <th className="py-3 px-4 font-semibold">Candidate Symbol</th>
                <th className="py-3 px-4 font-semibold text-right">Current Price</th>
                <th className="py-3 px-4 font-semibold text-right">24h Volume (USD)</th>
                <th className="py-3 px-4 font-semibold text-right">Spread (bps)</th>
                <th className="py-3 px-4 font-semibold text-center">Status</th>
                <th className="py-3 px-4 font-semibold">Eligibility / Rejection Reason</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800/60">
              {assets.length === 0 ? (
                <tr>
                  <td colSpan={6} className="py-8 text-center text-slate-400 font-mono">
                    {loading ? 'Evaluating candidate assets against liquidity filters...' : 'No candidate evaluation snapshots found.'}
                  </td>
                </tr>
              ) : (
                assets.map((asset) => {
                  const isQualified = asset.status === 'ACTIVE';
                  return (
                    <tr key={asset.symbol} className="hover:bg-slate-50/50 dark:hover:bg-slate-800/30 transition-colors">
                      <td className="py-3 px-4 font-bold text-slate-900 dark:text-slate-100">
                        {asset.symbol}
                      </td>
                      <td className="py-3 px-4 text-right text-slate-700 dark:text-slate-300">
                        ${asset.price.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 4 })}
                      </td>
                      <td className="py-3 px-4 text-right">
                        <span className={asset.volume_24h >= 50000000 ? 'text-slate-900 dark:text-slate-100 font-semibold' : 'text-rose-500'}>
                          ${(asset.volume_24h / 1000000).toFixed(1)}M
                        </span>
                      </td>
                      <td className="py-3 px-4 text-right">
                        <span className={asset.bid_ask_spread_bps <= 10 ? 'text-slate-900 dark:text-slate-100 font-semibold' : 'text-rose-500'}>
                          {asset.bid_ask_spread_bps.toFixed(2)} bps
                        </span>
                      </td>
                      <td className="py-3 px-4 text-center">
                        <span className={`inline-flex items-center space-x-1 px-2 py-0.5 rounded-full text-[10px] font-bold ${
                          isQualified
                            ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20'
                            : 'bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/20'
                        }`}>
                          {isQualified ? <CheckCircle className="w-3 h-3" /> : <XCircle className="w-3 h-3" />}
                          <span>{asset.status}</span>
                        </span>
                      </td>
                      <td className="py-3 px-4 text-slate-500 dark:text-slate-400">
                        {isQualified ? (
                          <span className="text-emerald-600 dark:text-emerald-400">Qualified for autonomous execution</span>
                        ) : (
                          <span className="text-rose-500/90">{asset.rejection_reason || 'Disqualified'}</span>
                        )}
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
};
