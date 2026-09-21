import { useEffect, useRef, useState } from 'react';
import { AssetInfo, AISignal, TradePosition, PortfolioSummary } from '../types';

export const INITIAL_ASSETS: AssetInfo[] = [
  { symbol: 'XAU/USD', name: 'Gold Spot', bucket: 'CORE', type: 'Commodity', price: 2980.50, change24h: 1.15, high24h: 2995.00, low24h: 2965.20 },
  { symbol: 'XAG/USD', name: 'Silver Spot', bucket: 'CORE', type: 'Commodity', price: 34.25, change24h: -0.42, high24h: 34.90, low24h: 33.80 },
  { symbol: 'BTC/USD', name: 'Bitcoin', bucket: 'ALPHA', type: 'Crypto', price: 92450.00, change24h: 3.42, high24h: 93800.00, low24h: 89400.00 },
  { symbol: 'ETH/USD', name: 'Ethereum', bucket: 'ALPHA', type: 'Crypto', price: 3450.75, change24h: 2.18, high24h: 3520.00, low24h: 3380.00 },
  { symbol: 'SOL/USD', name: 'Solana', bucket: 'ALPHA', type: 'Crypto', price: 185.30, change24h: -1.25, high24h: 192.50, low24h: 181.00 },
  { symbol: 'EUR/USD', name: 'Euro / US Dollar', bucket: 'ALPHA', type: 'Forex', price: 1.0845, change24h: 0.12, high24h: 1.0870, low24h: 1.0820 },
  { symbol: 'WTI/USD', name: 'Crude Oil WTI', bucket: 'ALPHA', type: 'Commodity', price: 72.80, change24h: -0.85, high24h: 74.10, low24h: 71.90 },
];

export const INITIAL_SUMMARY: PortfolioSummary = {
  totalEquity: 102450.00,
  coreEquity: 61470.00,
  alphaEquity: 40980.00,
  targetCorePct: 0.60,
  targetAlphaPct: 0.40,
  cash: 35200.00,
  peakEquity: 104200.00,
  drawdownPct: 1.68,
  circuitBreakerHalted: false,
};

export const INITIAL_POSITIONS: TradePosition[] = [
  {
    id: 'pos-1',
    symbol: 'XAU/USD',
    bucket: 'CORE',
    side: 'BUY',
    entryPrice: 2955.00,
    currentPrice: 2980.50,
    size: 15.5,
    stopLoss: 2920.00,
    takeProfit: 3040.00,
    unrealizedPnL: 395.25,
    pnlPercent: 0.86,
    entryTime: new Date(Date.now() - 3600000 * 4).toLocaleTimeString(),
  },
  {
    id: 'pos-2',
    symbol: 'BTC/USD',
    bucket: 'ALPHA',
    side: 'BUY',
    entryPrice: 90200.00,
    currentPrice: 92450.00,
    size: 0.25,
    stopLoss: 88500.00,
    takeProfit: 95000.00,
    unrealizedPnL: 562.50,
    pnlPercent: 2.49,
    entryTime: new Date(Date.now() - 3600000 * 12).toLocaleTimeString(),
  },
  {
    id: 'pos-3',
    symbol: 'SOL/USD',
    bucket: 'ALPHA',
    side: 'SELL',
    entryPrice: 189.40,
    currentPrice: 185.30,
    size: 40.0,
    stopLoss: 194.00,
    takeProfit: 178.00,
    unrealizedPnL: 164.00,
    pnlPercent: 2.16,
    entryTime: new Date(Date.now() - 3600000 * 2).toLocaleTimeString(),
  },
];

export const INITIAL_SIGNALS: AISignal[] = [
  {
    id: 'sig-1',
    symbol: 'XAU/USD',
    direction: 'BUY',
    confluenceScore: 0.88,
    rationale: 'RSI bullish continuation (58.4) with SuperTrend bull support above $2960 and positive macro flight to quality.',
    regime: 'Bullish Trending',
    timestamp: new Date().toLocaleTimeString(),
    indicators: { rsi: 58.4, macd: 4.2, supertrend: 'BULL', bollinger: 'UPPER_EXPANSION' },
  },
  {
    id: 'sig-2',
    symbol: 'BTC/USD',
    direction: 'BUY',
    confluenceScore: 0.92,
    rationale: 'Strong VWAP cross with MACD positive divergence; Alpha momentum favorable after volume expansion.',
    regime: 'High Volatility Momentum',
    timestamp: new Date(Date.now() - 120000).toLocaleTimeString(),
    indicators: { rsi: 64.1, macd: 128.5, supertrend: 'BULL', bollinger: 'EXPANDING' },
  },
  {
    id: 'sig-3',
    symbol: 'EUR/USD',
    direction: 'HOLD',
    confluenceScore: 0.45,
    rationale: 'Consolidation inside tight Bollinger squeeze; no clear macro divergence.',
    regime: 'Rangebound',
    timestamp: new Date(Date.now() - 300000).toLocaleTimeString(),
    indicators: { rsi: 49.8, macd: -0.0004, supertrend: 'NEUTRAL', bollinger: 'SQUEEZE' },
  },
];

export function useSSE(endpoint: string = '/api/v1/events') {
  const [isConnected, setIsConnected] = useState<boolean>(false);
  const [assets, setAssets] = useState<AssetInfo[]>(INITIAL_ASSETS);
  const [signals, setSignals] = useState<AISignal[]>(INITIAL_SIGNALS);
  const [positions, setPositions] = useState<TradePosition[]>(INITIAL_POSITIONS);
  const [summary, setSummary] = useState<PortfolioSummary>(INITIAL_SUMMARY);
  const eventSourceRef = useRef<EventSource | null>(null);

  useEffect(() => {
    let reconnectTimeout: ReturnType<typeof setTimeout> | undefined;

    const connect = () => {
      try {
        const es = new EventSource(endpoint);
        eventSourceRef.current = es;

        es.onopen = () => {
          setIsConnected(true);
        };

        es.addEventListener('tick', (e: MessageEvent) => {
          try {
            const tick = JSON.parse(e.data);
            const newPrice = Number(tick.price);
            if (!newPrice || isNaN(newPrice)) return;

            setAssets((prev) =>
              prev.map((a) => {
                if (a.symbol === tick.symbol) {
                  const diff = newPrice - a.price;
                  return {
                    ...a,
                    price: newPrice,
                    change24h: Number((a.change24h + (diff / a.price) * 10).toFixed(2)),
                  };
                }
                return a;
              })
            );

            // Dynamically mark-to-market revalue open positions and update portfolio equity
            setPositions((prevPositions) => {
              let updated = false;
              const nextPositions = prevPositions.map((pos) => {
                if (pos.symbol === tick.symbol) {
                  updated = true;
                  const diff = pos.side === 'BUY' ? newPrice - pos.entryPrice : pos.entryPrice - newPrice;
                  const unrealizedPnL = Number((diff * pos.size).toFixed(2));
                  const pnlPercent = Number(((diff / pos.entryPrice) * 100).toFixed(2));
                  return {
                    ...pos,
                    currentPrice: newPrice,
                    unrealizedPnL,
                    pnlPercent,
                  };
                }
                return pos;
              });

              // Recalculate dynamic Core and Alpha valuations
              if (updated) {
                setSummary((prevSummary) => {
                  let corePnL = 0;
                  let alphaPnL = 0;
                  nextPositions.forEach((p) => {
                    if (p.bucket === 'CORE') corePnL += p.unrealizedPnL;
                    else alphaPnL += p.unrealizedPnL;
                  });

                  const baseCore = 61470.0;
                  const baseAlpha = 40980.0;
                  const dynamicCore = Number((baseCore + corePnL).toFixed(2));
                  const dynamicAlpha = Number((baseAlpha + alphaPnL).toFixed(2));
                  const dynamicTotal = Number((prevSummary.cash + dynamicCore + dynamicAlpha - (baseCore + baseAlpha - (102450.0 - prevSummary.cash))).toFixed(2));
                  const peak = Math.max(prevSummary.peakEquity, dynamicTotal);
                  const dd = peak > 0 ? Number((((peak - dynamicTotal) / peak) * 100).toFixed(2)) : 0;

                  return {
                    ...prevSummary,
                    coreEquity: dynamicCore,
                    alphaEquity: dynamicAlpha,
                    totalEquity: dynamicTotal,
                    peakEquity: peak,
                    drawdownPct: dd,
                  };
                });
              }

              return nextPositions;
            });
          } catch (err) {
            console.error('Failed to parse SSE tick', err);
          }
        });

        es.addEventListener('signal', (e: MessageEvent) => {
          try {
            const sig: AISignal = JSON.parse(e.data);
            setSignals((prev) => [sig, ...prev.slice(0, 9)]);
          } catch (err) {
            console.error('Failed to parse SSE signal', err);
          }
        });

        es.addEventListener('trade', (e: MessageEvent) => {
          try {
            const trade = JSON.parse(e.data);
            if (trade.position) {
              setPositions((prev) => [
                trade.position,
                ...prev.filter((p) => p.id !== trade.position.id),
              ]);
            }
          } catch (err) {
            console.error('Failed to parse SSE trade', err);
          }
        });

        es.addEventListener('halt', () => {
          setSummary((prev) => ({ ...prev, circuitBreakerHalted: true }));
        });

        es.onerror = () => {
          setIsConnected(false);
          es.close();
          reconnectTimeout = setTimeout(connect, 4000);
        };
      } catch {
        setIsConnected(false);
        reconnectTimeout = setTimeout(connect, 4000);
      }
    };

    connect();

    return () => {
      if (reconnectTimeout) clearTimeout(reconnectTimeout);
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
      }
    };
  }, [endpoint]);

  return {
    isConnected,
    assets,
    signals,
    positions,
    summary,
    setPositions,
    setSummary,
    setAssets,
  };
}
