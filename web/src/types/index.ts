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
    obi?: number;
    cvd?: number;
    volRatio?: number;
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

export interface MacroCalendarEvent {
  id: string;
  title: string;
  currency: string;
  impact: 'HIGH' | 'MEDIUM' | 'LOW';
  scheduled_at: string;
  actual?: string;
  forecast?: string;
  previous?: string;
}

export interface MicrostructureData {
  symbol: string;
  obi: number; // Order Book Imbalance (-1.0 to 1.0)
  cvd: number; // Cumulative Volume Delta
  divergence: string; // NONE, BULLISH_ABSORPTION, BEARISH_EXHAUSTION
  regime: string; // LOW_VOL_CONSOLIDATION, NORMAL_TRENDING, HIGH_VOL_CHOP
  volRatio: number;
}

export interface MicrostructureState {
  symbol: string;
  obi: number; // -1.0 to 1.0
  cvd: number;
  divergence: 'NONE' | 'BULLISH_ABSORPTION' | 'BEARISH_EXHAUSTION';
  regime: 'LOW_VOL_CONSOLIDATION' | 'NORMAL_TRENDING' | 'HIGH_VOL_CHOP';
  volRatio: number;
}

export interface BacktestTrade {
  entry_time: string;
  exit_time: string;
  side: string;
  entry_price: number;
  exit_price: number;
  size: number;
  net_pnl: number;
  return_pct: number;
  exit_reason: string;
  fee_paid?: number;
  slippage_paid?: number;
}

export interface BacktestSummary {
  total_trades: number;
  winning_trades: number;
  losing_trades: number;
  win_rate: number;
  total_return_pct: number;
  ending_capital: number;
  max_drawdown_pct: number;
  sharpe_ratio: number;
  sortino_ratio: number;
  profit_factor: number;
  avg_trade_return_pct?: number;
  trades: BacktestTrade[];
  equity_curve: number[];
  execution_duration?: number;
}

export type BacktestRunResult = BacktestSummary;

export interface MonteCarloSummary {
  iterations: number;
  mean_return_pct?: number;
  median_return_pct: number;
  percentile_5th_return?: number;
  percentile_95th_return?: number;
  max_drawdown_95th_pct: number;
  max_drawdown_99th_pct: number;
  probability_of_ruin_pct: number;
}

export type MonteCarloRunResult = MonteCarloSummary;

