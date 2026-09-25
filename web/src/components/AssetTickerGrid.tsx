import React, { useState } from 'react';
import { AssetInfo } from '../types';
import { TrendingUp, TrendingDown, ChevronDown, ChevronUp } from 'lucide-react';

interface AssetTickerGridProps {
  assets: AssetInfo[];
  selectedSymbol: string;
  onSelectSymbol: (symbol: string) => void;
}

// With 123 instruments a full grid is ~60 stacked rows on a phone. Default to
// one scroll-snap strip of the top movers; "Show all" expands the full grid.
export const AssetTickerGrid: React.FC<AssetTickerGridProps> = ({
  assets,
  selectedSymbol,
  onSelectSymbol,
}) => {
  const [showAll, setShowAll] = useState(false);

  const strip = React.useMemo(() => {
    const ranked = [...assets].sort((a, b) => {
      // Selected asset first, then biggest absolute movers
      if (a.symbol === selectedSymbol) return -1;
      if (b.symbol === selectedSymbol) return 1;
      const am = Math.abs(typeof a.change24h === 'number' ? a.change24h : 0);
      const bm = Math.abs(typeof b.change24h === 'number' ? b.change24h : 0);
      return bm - am;
    });
    return ranked.slice(0, 14);
  }, [assets, selectedSymbol]);

  const listed = showAll ? assets : strip;

  const tickerCard = (asset: AssetInfo) => {
    const isSelected = asset.symbol === selectedSymbol;
    const price = typeof asset.price === 'number' && !isNaN(asset.price) ? asset.price : 0;
    const change = typeof asset.change24h === 'number' && !isNaN(asset.change24h) ? asset.change24h : 0;
    const isPositive = change >= 0;

    return (
      <button
        key={asset.symbol}
        onClick={() => onSelectSymbol(asset.symbol)}
        className={`p-3 rounded-xl border text-left transition-colors duration-150 flex flex-col justify-between shrink-0 min-w-[132px] ${
          isSelected
            ? 'border-sky-500 bg-sky-500/5 ring-1 ring-sky-500/30'
            : 'border-slate-200 dark:border-slate-800/80 bg-white dark:bg-slate-900/40 hover:border-slate-300 dark:hover:border-slate-700'
        }`}
      >
        <div className="flex items-center justify-between w-full mb-1 gap-1">
          <span className="font-bold font-mono text-sm tracking-tight text-slate-800 dark:text-slate-100 truncate">
            {asset.symbol}
          </span>
          <span
            className={`text-[10px] font-bold uppercase px-1.5 py-0.5 rounded shrink-0 ${
              asset.bucket === 'CORE'
                ? 'bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20'
                : 'bg-indigo-500/10 text-indigo-600 dark:text-indigo-400 border border-indigo-500/20'
            }`}
          >
            {asset.bucket}
          </span>
        </div>

        <div className="mt-1">
          <div className="text-base font-mono font-bold text-slate-900 dark:text-slate-100 whitespace-nowrap">
            ${price < 10 ? price.toFixed(4) : price.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
          </div>
          <div
            className={`flex items-center space-x-1 text-xs font-mono font-medium mt-0.5 ${
              isPositive ? 'text-emerald-500' : 'text-rose-500'
            }`}
          >
            {isPositive ? (
              <TrendingUp className="w-3 h-3" />
            ) : (
              <TrendingDown className="w-3 h-3" />
            )}
            <span>
              {isPositive ? '+' : ''}
              {change.toFixed(2)}%
            </span>
          </div>
        </div>
      </button>
    );
  };

  return (
    <div className="space-y-2">
      {showAll ? (
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-7 gap-2.5">
          {listed.map(tickerCard)}
        </div>
      ) : (
        <div className="flex gap-2.5 overflow-x-auto pb-2 -mx-1 px-1 snap-x snap-mandatory [scrollbar-width:thin]">
          {listed.map(tickerCard)}
        </div>
      )}

      <button
        onClick={() => setShowAll(!showAll)}
        className="min-h-[44px] px-3.5 rounded-lg border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 text-xs font-semibold font-mono text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-800 transition-colors flex items-center gap-1.5"
      >
        {showAll ? (
          <>
            <ChevronUp className="w-3.5 h-3.5" />
            <span>Show Top Movers Only</span>
          </>
        ) : (
          <>
            <ChevronDown className="w-3.5 h-3.5" />
            <span>Show All {assets.length} Assets</span>
          </>
        )}
      </button>
    </div>
  );
};
