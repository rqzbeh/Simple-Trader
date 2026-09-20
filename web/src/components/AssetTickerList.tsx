import React from 'react';
import { AssetInfo } from '../types';
import { TrendingUp, TrendingDown, Shield, Zap } from 'lucide-react';

interface AssetTickerListProps {
  assets: AssetInfo[];
  selectedSymbol: string;
  onSelectSymbol: (symbol: string) => void;
}

export const AssetTickerList: React.FC<AssetTickerListProps> = ({
  assets,
  selectedSymbol,
  onSelectSymbol,
}) => {
  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-7 gap-3">
      {assets.map((asset) => {
        const isSelected = asset.symbol === selectedSymbol;
        const isPositive = asset.change24h >= 0;

        return (
          <button
            key={asset.symbol}
            onClick={() => onSelectSymbol(asset.symbol)}
            className={`flex flex-col p-3 rounded-xl border text-left transition-all duration-150 ${
              isSelected
                ? 'border-sky-500 bg-sky-50/50 dark:bg-sky-950/30 shadow-md ring-1 ring-sky-500'
                : 'border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 hover:border-slate-300 dark:hover:border-slate-700'
            }`}
          >
            <div className="flex items-center justify-between mb-1">
              <span className="font-mono font-bold text-xs tracking-wider text-slate-900 dark:text-slate-100">
                {asset.symbol}
              </span>
              <span
                className={`text-[10px] font-semibold px-1.5 py-0.5 rounded flex items-center space-x-0.5 ${
                  asset.bucket === 'CORE'
                    ? 'bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20'
                    : 'bg-indigo-500/10 text-indigo-600 dark:text-indigo-400 border border-indigo-500/20'
                }`}
              >
                {asset.bucket === 'CORE' ? (
                  <Shield className="w-2.5 h-2.5 mr-0.5" />
                ) : (
                  <Zap className="w-2.5 h-2.5 mr-0.5" />
                )}
                {asset.bucket}
              </span>
            </div>

            <div className="font-mono font-semibold text-sm text-slate-800 dark:text-slate-200 mt-0.5">
              ${asset.price > 10 ? asset.price.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 }) : asset.price.toFixed(4)}
            </div>

            <div
              className={`flex items-center text-xs font-medium mt-1 ${
                isPositive ? 'text-emerald-500' : 'text-rose-500'
              }`}
            >
              {isPositive ? (
                <TrendingUp className="w-3 h-3 mr-1" />
              ) : (
                <TrendingDown className="w-3 h-3 mr-1" />
              )}
              <span>{isPositive ? '+' : ''}{asset.change24h}%</span>
            </div>
          </button>
        );
      })}
    </div>
  );
};
