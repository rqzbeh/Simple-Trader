import React, { useEffect, useRef } from 'react';
import { createChart, IChartApi, ISeriesApi } from 'lightweight-charts';
import { CandleData } from '../types';

interface ChartProps {
  symbol: string;
  data?: CandleData[];
  theme: 'dark' | 'light';
}

// Generate realistic pseudo candles for initial view if none provided
function generateMockCandles(basePrice: number, count: number = 60): CandleData[] {
  const candles: CandleData[] = [];
  let current = basePrice;
  const now = Math.floor(Date.now() / 1000);
  const interval = 60; // 1m candles

  for (let i = count; i >= 0; i--) {
    const time = now - i * interval;
    const change = (Math.random() - 0.49) * (basePrice * 0.003);
    const open = current;
    const close = open + change;
    const high = Math.max(open, close) + Math.random() * (basePrice * 0.0015);
    const low = Math.min(open, close) - Math.random() * (basePrice * 0.0015);
    candles.push({ time, open, high, low, close });
    current = close;
  }
  return candles;
}

export const LightweightChart: React.FC<ChartProps> = ({ symbol, data, theme }) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<IChartApi | null>(null);
  const seriesRef = useRef<ISeriesApi<'Candlestick'> | null>(null);

  const isDark = theme === 'dark';

  useEffect(() => {
    if (!containerRef.current) return;

    // Clean up existing chart
    if (chartRef.current) {
      chartRef.current.remove();
      chartRef.current = null;
    }

    const chart = createChart(containerRef.current, {
      width: containerRef.current.clientWidth,
      height: 380,
      layout: {
        background: { color: isDark ? '#0b1329' : '#ffffff' },
        textColor: isDark ? '#94a3b8' : '#475569',
      },
      grid: {
        vertLines: { color: isDark ? '#1e293b' : '#f1f5f9' },
        horzLines: { color: isDark ? '#1e293b' : '#f1f5f9' },
      },
      crosshair: {
        vertLine: { color: '#38bdf8', width: 1, style: 2 },
        horzLine: { color: '#38bdf8', width: 1, style: 2 },
      },
      timeScale: {
        borderColor: isDark ? '#1e293b' : '#e2e8f0',
        timeVisible: true,
        secondsVisible: false,
      },
      rightPriceScale: {
        borderColor: isDark ? '#1e293b' : '#e2e8f0',
      },
    });

    const candlestickSeries = chart.addCandlestickSeries({
      upColor: '#10b981',
      downColor: '#ef4444',
      borderVisible: false,
      wickUpColor: '#10b981',
      wickDownColor: '#ef4444',
    });

    // Default base price by asset
    let basePrice = 2980;
    if (symbol.includes('BTC')) basePrice = 92450;
    else if (symbol.includes('ETH')) basePrice = 3450;
    else if (symbol.includes('SOL')) basePrice = 185;
    else if (symbol.includes('EUR')) basePrice = 1.085;
    else if (symbol.includes('XAG')) basePrice = 34.2;
    else if (symbol.includes('WTI')) basePrice = 72.8;

    const initialCandles = data && data.length > 0 ? data : generateMockCandles(basePrice, 80);
    candlestickSeries.setData(initialCandles as any);

    chartRef.current = chart;
    seriesRef.current = candlestickSeries;

    const handleResize = () => {
      if (containerRef.current && chartRef.current) {
        chartRef.current.applyOptions({ width: containerRef.current.clientWidth });
      }
    };

    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
      if (chartRef.current) {
        chartRef.current.remove();
        chartRef.current = null;
      }
    };
  }, [symbol, isDark]);

  return (
    <div className="relative w-full rounded-xl overflow-hidden border border-slate-200 dark:border-slate-800 bg-white dark:bg-[#0b1329] shadow-sm">
      <div className="flex items-center justify-between px-4 py-3 border-b border-slate-200 dark:border-slate-800">
        <div className="flex items-center space-x-2">
          <span className="font-mono font-bold text-base text-slate-900 dark:text-slate-100">{symbol}</span>
          <span className="text-xs px-2 py-0.5 rounded bg-sky-500/10 text-sky-600 dark:text-sky-400 font-mono font-medium">
            1M Live Chart
          </span>
        </div>
        <div className="text-xs text-slate-500 dark:text-slate-400 font-mono">
          TradingView Lightweight Engine
        </div>
      </div>
      <div ref={containerRef} className="w-full h-[380px]" />
    </div>
  );
};
