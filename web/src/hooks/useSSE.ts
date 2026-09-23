import { useEffect, useRef, useState } from 'react';
import { AssetInfo, AISignal, TradePosition, PortfolioSummary } from '../types';

export const INITIAL_ASSETS: AssetInfo[] = [
  // CORE (Commodities - 8 assets)
  { symbol: 'PAXG/USDT', name: 'PAX Gold (Tokenized Gold)', bucket: 'CORE', type: 'Crypto', price: 0, change24h: 0 },
  { symbol: 'XAU/USDT', name: 'Gold Futures', bucket: 'CORE', type: 'Crypto', price: 0, change24h: 0 },
  { symbol: 'XAG/USDT', name: 'Silver Futures', bucket: 'CORE', type: 'Crypto', price: 0, change24h: 0 },
  { symbol: 'COPPER/USDT', name: 'Copper Futures', bucket: 'CORE', type: 'Crypto', price: 0, change24h: 0 },
  { symbol: 'XPT/USDT', name: 'Platinum Futures', bucket: 'CORE', type: 'Crypto', price: 0, change24h: 0 },
  { symbol: 'XPD/USDT', name: 'Palladium Futures', bucket: 'CORE', type: 'Crypto', price: 0, change24h: 0 },
  { symbol: 'OIL/USDT', name: 'WTI Crude Oil', bucket: 'CORE', type: 'Crypto', price: 0, change24h: 0 },
  { symbol: 'ALU/USDT', name: 'Aluminum Futures', bucket: 'CORE', type: 'Crypto', price: 0, change24h: 0 },
  // ALPHA (Crypto - 17 assets)
  { symbol: 'BTC/USDT', name: 'Bitcoin', bucket: 'ALPHA', type: 'Crypto', price: 0, change24h: 0 },
  { symbol: 'ETH/USDT', name: 'Ethereum', bucket: 'ALPHA', type: 'Crypto', price: 0, change24h: 0 },
  { symbol: 'SOL/USDT', name: 'Solana', bucket: 'ALPHA', type: 'Crypto', price: 0, change24h: 0 },
  { symbol: 'BNB/USDT', name: 'BNB', bucket: 'ALPHA', type: 'Crypto', price: 0, change24h: 0 },
  { symbol: 'XRP/USDT', name: 'XRP', bucket: 'ALPHA', type: 'Crypto', price: 0, change24h: 0 },
  { symbol: 'DOGE/USDT', name: 'Dogecoin', bucket: 'ALPHA', type: 'Crypto', price: 0, change24h: 0 },
  { symbol: 'ADA/USDT', name: 'Cardano', bucket: 'ALPHA', type: 'Crypto', price: 0, change24h: 0 },
  { symbol: 'AVAX/USDT', name: 'Avalanche', bucket: 'ALPHA', type: 'Crypto', price: 0, change24h: 0 },
  { symbol: 'SUI/USDT', name: 'Sui', bucket: 'ALPHA', type: 'Crypto', price: 0, change24h: 0 },
  { symbol: 'LINK/USDT', name: 'Chainlink', bucket: 'ALPHA', type: 'Crypto', price: 0, change24h: 0 },
  { symbol: 'DOT/USDT', name: 'Polkadot', bucket: 'ALPHA', type: 'Crypto', price: 0, change24h: 0 },
  { symbol: 'NEAR/USDT', name: 'NEAR Protocol', bucket: 'ALPHA', type: 'Crypto', price: 0, change24h: 0 },
  { symbol: 'LTC/USDT', name: 'Litecoin', bucket: 'ALPHA', type: 'Crypto', price: 0, change24h: 0 },
  { symbol: 'BCH/USDT', name: 'Bitcoin Cash', bucket: 'ALPHA', type: 'Crypto', price: 0, change24h: 0 },
  { symbol: 'UNI/USDT', name: 'Uniswap', bucket: 'ALPHA', type: 'Crypto', price: 0, change24h: 0 },
  { symbol: 'APT/USDT', name: 'Aptos', bucket: 'ALPHA', type: 'Crypto', price: 0, change24h: 0 },
  { symbol: 'TON/USDT', name: 'Toncoin', bucket: 'ALPHA', type: 'Crypto', price: 0, change24h: 0 },
];

export const INITIAL_SUMMARY: PortfolioSummary = {
  totalEquity: 10000,
  coreEquity: 6000,
  alphaEquity: 4000,
  targetCorePct: 0.60,
  targetAlphaPct: 0.40,
  cash: 10000,
  initialEquity: 10000,
  peakEquity: 10000,
  drawdownPct: 0,
  circuitBreakerHalted: false,
};

export const INITIAL_POSITIONS: TradePosition[] = [];
export const INITIAL_SIGNALS: AISignal[] = [];

export function useSSE(endpoint: string = '/api/v1/events') {
  const [isConnected, setIsConnected] = useState<boolean>(false);
  const [assets, setAssets] = useState<AssetInfo[]>(INITIAL_ASSETS);
  const [signals, setSignals] = useState<AISignal[]>(INITIAL_SIGNALS);
  const [positions, setPositions] = useState<TradePosition[]>(INITIAL_POSITIONS);
  const [summary, setSummary] = useState<PortfolioSummary>(INITIAL_SUMMARY);
  const eventSourceRef = useRef<EventSource | null>(null);

  // Fetch authentic online initial state on mount
  useEffect(() => {
    let isMounted = true;

    async function fetchInitialState() {
      try {
        const [assetsRes, positionsRes, summaryRes] = await Promise.all([
          fetch('/api/v1/assets').catch(() => null),
          fetch('/api/v1/positions').catch(() => null),
          fetch('/api/v1/portfolio/summary').catch(() => null),
        ]);

        if (assetsRes && assetsRes.ok) {
          const data = await assetsRes.json();
          if (isMounted && data.assets && Array.isArray(data.assets)) {
            setAssets((prev) =>
              data.assets.map((item: any) => {
                const existing = prev.find((p) => p.symbol === item.symbol);
                return {
                  symbol: item.symbol,
                  name: item.name,
                  bucket: item.bucket,
                  type: 'Crypto',
                  price: typeof item.price === 'number' && item.price > 0 ? item.price : (existing?.price || 0),
                  change24h: typeof item.change24h === 'number' ? item.change24h : (typeof item.change_24h === 'number' ? item.change_24h : (existing?.change24h || 0)),
                  high24h: typeof item.high24h === 'number' ? item.high24h : (typeof item.high_24h === 'number' ? item.high_24h : existing?.high24h),
                  low24h: typeof item.low24h === 'number' ? item.low24h : (typeof item.low_24h === 'number' ? item.low_24h : existing?.low24h),
                  volume: typeof item.volume === 'number' ? item.volume : existing?.volume,
                };
              })
            );
          }
        }

        if (positionsRes && positionsRes.ok) {
          const data = await positionsRes.json();
          if (isMounted && Array.isArray(data)) {
            setPositions(data);
          }
        }

        if (summaryRes && summaryRes.ok) {
          const data = await summaryRes.json();
          if (isMounted && data && typeof data.totalEquity === 'number') {
            setSummary((prev) => ({
              ...prev,
              totalEquity: data.totalEquity,
              coreEquity: data.coreEquity,
              alphaEquity: data.alphaEquity,
              targetCorePct: data.targetCorePct ?? prev.targetCorePct,
              targetAlphaPct: data.targetAlphaPct ?? prev.targetAlphaPct,
              cash: data.cash ?? prev.cash,
              initialEquity: data.initialEquity ?? data.totalEquity,
              peakEquity: data.peakEquity ?? data.totalEquity,
              drawdownPct: data.drawdownPct ?? 0,
              circuitBreakerHalted: !!data.circuitBreakerHalted,
            }));
          }
        }
      } catch (err) {
        console.error('Failed to fetch initial online state', err);
      }
    }

    fetchInitialState();

    return () => {
      isMounted = false;
    };
  }, []);

  // Connect to SSE stream
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

            const tickChange = tick.change24h !== undefined ? Number(tick.change24h) : (tick.change_24h !== undefined ? Number(tick.change_24h) : 0);
            const tickHigh = tick.high24h !== undefined ? Number(tick.high24h) : (tick.high_24h !== undefined ? Number(tick.high_24h) : newPrice);
            const tickLow = tick.low24h !== undefined ? Number(tick.low24h) : (tick.low_24h !== undefined ? Number(tick.low_24h) : newPrice);
            const tickVolume = tick.volume !== undefined ? Number(tick.volume) : 0;

            setAssets((prev) => {
              const symbolExists = prev.some((a) => a.symbol === tick.symbol);
              if (!symbolExists) {
                return [
                  ...prev,
                  {
                    symbol: tick.symbol,
                    name: tick.symbol.split('/')[0] || tick.symbol,
                    bucket: 'ALPHA',
                    type: 'Crypto',
                    price: newPrice,
                    change24h: tickChange,
                    high24h: tickHigh,
                    low24h: tickLow,
                    volume: tickVolume,
                  },
                ];
              }

              return prev.map((a) => {
                if (a.symbol === tick.symbol) {
                  return {
                    ...a,
                    price: newPrice,
                    change24h: tickChange,
                    high24h: tickHigh,
                    low24h: tickLow,
                    volume: tickVolume > 0 ? tickVolume : a.volume,
                  };
                }
                return a;
              });
            });

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

              if (updated) {
                setSummary((prevSummary) => {
                  let corePnL = 0;
                  let alphaPnL = 0;
                  nextPositions.forEach((p) => {
                    if (p.bucket === 'CORE') corePnL += p.unrealizedPnL;
                    else alphaPnL += p.unrealizedPnL;
                  });

                  const baseCore = (prevSummary.initialEquity || prevSummary.totalEquity) * prevSummary.targetCorePct;
                  const baseAlpha = (prevSummary.initialEquity || prevSummary.totalEquity) * prevSummary.targetAlphaPct;
                  const dynamicCore = Number((baseCore + corePnL).toFixed(2));
                  const dynamicAlpha = Number((baseAlpha + alphaPnL).toFixed(2));
                  const dynamicTotal = Number((dynamicCore + dynamicAlpha).toFixed(2));
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
