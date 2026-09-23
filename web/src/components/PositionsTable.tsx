import React from 'react';
import { TradePosition } from '../types';
import { Shield, Zap, XCircle } from 'lucide-react';

interface PositionsTableProps {
  positions: TradePosition[];
  onClosePosition?: (id: string) => void;
}

export const PositionsTable: React.FC<PositionsTableProps> = ({
  positions,
  onClosePosition,
}) => {
  if (positions.length === 0) {
    return (
      <div className="p-8 text-center rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/40 text-slate-500 dark:text-slate-400">
        <p className="text-sm font-medium">No active positions open</p>
        <span className="text-xs text-slate-400 dark:text-slate-500">
          The risk engine & paper trader will open positions when AI confluence exceeds threshold.
        </span>
      </div>
    );
  }

  return (
    <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/40 shadow-sm">
      <table className="w-full text-left text-sm">
        <thead className="text-xs uppercase font-mono tracking-wider text-slate-500 dark:text-slate-400 bg-slate-50 dark:bg-slate-800/40 border-b border-slate-200 dark:border-slate-800">
          <tr>
            <th className="px-4 py-3">Asset</th>
            <th className="px-4 py-3">Side</th>
            <th className="px-4 py-3">Leverage</th>
            <th className="px-4 py-3">Size</th>
            <th className="px-4 py-3">Entry Price</th>
            <th className="px-4 py-3">Current</th>
            <th className="px-4 py-3">SL / TP</th>
            <th className="px-4 py-3">Liq. Price</th>
            <th className="px-4 py-3">Unrealized PnL</th>
            <th className="px-4 py-3 text-right">Action</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-slate-100 dark:divide-slate-800 font-mono text-xs">
          {positions.map((pos) => {
            const isProfit = pos.unrealizedPnL >= 0;
            return (
              <tr key={pos.id} className="hover:bg-slate-50/50 dark:hover:bg-slate-800/20 transition-colors">
                <td className="px-4 py-3 font-semibold text-slate-900 dark:text-slate-100 flex items-center space-x-1.5">
                  <span>{pos.symbol}</span>
                  <span
                    className={`text-[9px] px-1 py-0.5 rounded flex items-center ${
                      pos.bucket === 'CORE'
                        ? 'bg-amber-500/10 text-amber-500 border border-amber-500/20'
                        : 'bg-indigo-500/10 text-indigo-500 border border-indigo-500/20'
                    }`}
                  >
                    {pos.bucket === 'CORE' ? <Shield className="w-2.5 h-2.5 mr-0.5" /> : <Zap className="w-2.5 h-2.5 mr-0.5" />}
                    {pos.bucket}
                  </span>
                </td>
                <td className="px-4 py-3">
                  <span
                    className={`font-bold px-1.5 py-0.5 rounded text-[11px] ${
                      pos.side === 'BUY'
                        ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
                        : 'bg-rose-500/10 text-rose-600 dark:text-rose-400'
                    }`}
                  >
                    {pos.side}
                  </span>
                </td>
                <td className="px-4 py-3">
                  <span className="font-semibold px-1.5 py-0.5 rounded text-[10px] bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20">
                    {pos.leverage ? `${pos.leverage}x` : '1x'}
                  </span>
                </td>
                <td className="px-4 py-3 text-slate-700 dark:text-slate-300">
                  {pos.size ?? 0}
                </td>
                <td className="px-4 py-3 text-slate-700 dark:text-slate-300">
                  ${(pos.entryPrice ?? 0) < 10 ? (pos.entryPrice ?? 0).toFixed(4) : (pos.entryPrice ?? 0).toFixed(2)}
                </td>
                <td className="px-4 py-3 font-medium text-slate-900 dark:text-slate-100">
                  ${(pos.currentPrice ?? 0) < 10 ? (pos.currentPrice ?? 0).toFixed(4) : (pos.currentPrice ?? 0).toFixed(2)}
                </td>
                <td className="px-4 py-3 text-slate-500 dark:text-slate-400">
                  <span className="text-rose-500 dark:text-rose-400">${(pos.stopLoss ?? 0) < 10 ? (pos.stopLoss ?? 0).toFixed(4) : (pos.stopLoss ?? 0).toFixed(2)}</span>
                  {' / '}
                  <span className="text-emerald-500 dark:text-emerald-400">${(pos.takeProfit ?? 0) < 10 ? (pos.takeProfit ?? 0).toFixed(4) : (pos.takeProfit ?? 0).toFixed(2)}</span>
                </td>
                <td className="px-4 py-3 text-slate-500 dark:text-slate-400">
                  {pos.liquidationPrice ? (
                    <span className="text-amber-500/90 dark:text-amber-400/90">
                      ${pos.liquidationPrice < 10 ? pos.liquidationPrice.toFixed(4) : pos.liquidationPrice.toFixed(2)}
                    </span>
                  ) : (
                    <span className="text-slate-400">--</span>
                  )}
                </td>
                <td className="px-4 py-3 font-bold">
                  <span className={isProfit ? 'text-emerald-500' : 'text-rose-500'}>
                    {isProfit ? '+' : ''}${(pos.unrealizedPnL ?? 0).toFixed(2)} ({isProfit ? '+' : ''}{(pos.pnlPercent ?? 0).toFixed(2)}%)
                  </span>
                </td>
                <td className="px-4 py-3 text-right">
                  <button
                    onClick={() => onClosePosition?.(pos.id)}
                    className="p-1 text-slate-400 hover:text-rose-500 dark:hover:text-rose-400 transition-colors"
                    title="Close Position (Paper)"
                  >
                    <XCircle className="w-4 h-4" />
                  </button>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
};
