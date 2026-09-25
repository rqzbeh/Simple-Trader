import React, { useEffect, useRef } from 'react';
import { createChart, IChartApi, ISeriesApi, ColorType } from 'lightweight-charts';
import { useTheme } from '../context/ThemeContext';
import { formatChartTick, formatChartTime, useTimezone } from '../utils/time';
import { CandleData } from '../types';

interface ChartProps {
  symbol: string;
  data: CandleData[];
}

export const TradingViewChart: React.FC<ChartProps> = ({ symbol, data }) => {
  const chartContainerRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<IChartApi | null>(null);
  const candlestickSeriesRef = useRef<ISeriesApi<'Candlestick'> | null>(null);
  const { theme } = useTheme();
  // Recreate the chart when the display timezone changes: tick marks are
  // formatted at creation and lightweight-charts draws them in UTC otherwise.
  const timezone = useTimezone();

  const isDark = theme === 'dark';

  useEffect(() => {
    if (!chartContainerRef.current) return;

    // Clean up previous instance
    if (chartRef.current) {
      chartRef.current.remove();
    }

    const chart = createChart(chartContainerRef.current, {
      layout: {
        background: {
          type: ColorType.Solid,
          color: isDark ? '#0b0f19' : '#ffffff',
        },
        textColor: isDark ? '#94a3b8' : '#475569',
      },
      grid: {
        vertLines: { color: isDark ? '#1e293b' : '#f1f5f9' },
        horzLines: { color: isDark ? '#1e293b' : '#f1f5f9' },
      },
      crosshair: {
        vertLine: {
          color: isDark ? '#38bdf8' : '#0284c7',
          width: 1,
          style: 3,
        },
        horzLine: {
          color: isDark ? '#38bdf8' : '#0284c7',
          width: 1,
          style: 3,
        },
      },
      localization: {
        // Crosshair value in the selected display timezone
        timeFormatter: (time: any) => formatChartTime(time),
      },
      timeScale: {
        borderColor: isDark ? '#1e293b' : '#e2e8f0',
        timeVisible: true,
        secondsVisible: false,
        // Axis labels in the selected display timezone (library default is UTC)
        tickMarkFormatter: (time: any, tickMarkType: number) => formatChartTick(time, tickMarkType),
      },
      rightPriceScale: {
        borderColor: isDark ? '#1e293b' : '#e2e8f0',
      },
      width: chartContainerRef.current.clientWidth,
      height: 420,
    });

    const candlestickSeries = chart.addCandlestickSeries({
      upColor: '#10b981',
      downColor: '#ef4444',
      borderVisible: false,
      wickUpColor: '#10b981',
      wickDownColor: '#ef4444',
    });

    if (data && data.length > 0) {
      candlestickSeries.setData(data as any);
    }

    chartRef.current = chart;
    candlestickSeriesRef.current = candlestickSeries;

    const handleResize = () => {
      if (chartContainerRef.current && chartRef.current) {
        chartRef.current.applyOptions({
          width: chartContainerRef.current.clientWidth,
        });
      }
    };

    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
      chart.remove();
      chartRef.current = null;
    };
  }, [isDark, timezone]);

  // Update data when props change
  useEffect(() => {
    if (candlestickSeriesRef.current && data && data.length > 0) {
      candlestickSeriesRef.current.setData(data as any);
    }
  }, [data]);

  return (
    <div className="w-full bg-white dark:bg-[#0b0f19] rounded-xl border border-slate-200 dark:border-slate-800 p-4 shadow-sm flex flex-col">
      <div className="flex items-center justify-between pb-3 border-b border-slate-100 dark:border-slate-800/80 mb-3">
        <div className="flex items-center space-x-3">
          <span className="font-bold font-mono text-base tracking-tight text-slate-800 dark:text-slate-100">
            {symbol}
          </span>
          <span className="text-xs px-2 py-0.5 rounded bg-sky-500/10 text-sky-500 border border-sky-500/20 font-mono">
            1H Live Feed
          </span>
        </div>
        <div className="flex items-center space-x-2 text-xs text-slate-500 dark:text-slate-400">
          <span className="inline-block w-2 h-2 rounded-full bg-emerald-500 animate-ping"></span>
          <span>Streaming Candlesticks</span>
        </div>
      </div>
      <div ref={chartContainerRef} className="w-full h-[420px]" />
    </div>
  );
};
