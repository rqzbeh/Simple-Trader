export type AssetBucket = 'CORE' | 'ALPHA';

export interface AssetInfo {
  symbol: string;
  name: string;
  bucket: AssetBucket;
  type: string;
  price: number;
  change24h: number;
  high24h?: number;
  low24h?: number;
  volume?: number;
}

export interface CandleData {
  time: number | string; // lightweight-charts compatible
  open: number;
  high: number;
  low: number;
  close: number;
  volume?: number;
}

export interface TradePosition {
  id: string;
  symbol: string;
  bucket: AssetBucket;
  side: 'BUY' | 'SELL';
  entryPrice: number;
  currentPrice: number;
  size: number;
  stopLoss: number;
  takeProfit: number;
  unrealizedPnL: number;
  pnlPercent: number;
  entryTime: string;
}

export interface AISignal {
  id: string;
  symbol: string;
  direction: 'BUY' | 'SELL' | 'HOLD';
  confluenceScore: number; // 0.0 - 1.0
  rationale: string;
  regime: string;
  timestamp: string;
  indicators: {
    rsi?: number;
    macd?: number;
    supertrend?: string;
    bollinger?: string;
  };
}

export interface IndicatorWeights {
  symbol: string;
  regime: string;
  weights: {
    [indicator: string]: number; // e.g. "RSI": 1.25, "MACD": 0.85
  };
  lastUpdated: string;
}

export interface PortfolioSummary {
  totalEquity: number;
  coreEquity: number;
  alphaEquity: number;
  targetCorePct: number; // 0.60
  targetAlphaPct: number; // 0.40
  cash: number;
  peakEquity: number;
  drawdownPct: number;
  circuitBreakerHalted: boolean;
}
