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
  leverage?: number;
  liquidationPrice?: number;
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
  targetCorePct: number;
  targetAlphaPct: number;
  cash: number;
  initialEquity?: number;
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

// Investor Capital Ledger Types (US1, FR-009, FR-010)
export interface Investor {
  id: string;
  name: string;
  contact_tag: string;
  notes: string;
  total_deposited: number;
  total_withdrawn: number;
  pool_units: number;
  status: 'ACTIVE' | 'INACTIVE';
  current_equity: number;
  roi: number; // e.g. 0.0845 = 8.45%
  pool_share_pct: number; // e.g. 24.50%
  created_at: string;
  updated_at: string;
}

export interface CapitalTransaction {
  id: string;
  investor_id: string;
  tx_type: 'DEPOSIT' | 'WITHDRAWAL';
  amount: number;
  units_transacted: number;
  unit_nav_at_tx: number;
  settled_tier1_cash: number;
  notes: string;
  timestamp: string;
}

export interface TierAllocationBreakdown {
  tier1_cash: number;
  tier1_pct: number;
  tier2_core: number;
  tier2_pct: number;
  tier3_alpha: number;
  tier3_pct: number;
  total_capital: number;
}

// Live News Stream Types (FR-004)
export interface NewsArticle {
  id?: string;
  content_hash: string;
  title: string;
  source: string;
  url: string;
  sentiment_score: number; // -1.0 to +1.0
  polarity: 'BULLISH' | 'BEARISH' | 'NEUTRAL';
  key_phrases?: string[];
  published_at: string;
  ingested_at: string;
}

export interface NewsSentimentSummary {
  score: number;
  polarity: 'BULLISH' | 'BEARISH' | 'NEUTRAL';
  headline_count: number;
  bullish_count: number;
  bearish_count: number;
  neutral_count: number;
  key_phrases?: string[];
}

// Dynamic Liquid Crypto Screener (FR-006)
export interface ScreenedAsset {
  symbol: string;
  price: number;
  volume_24h: number;
  bid_ask_spread_bps: number;
  status: 'ACTIVE' | 'DISQUALIFIED';
  rejection_reason?: string;
  screened_at: string;
}

// Two-Sided Futures Trade Signal (US1)
export interface FuturesTradeSignal {
  id: number;
  symbol: string;
  direction: 'LONG' | 'SHORT';
  status: 'ACTIVE' | 'CLOSED' | 'CANCELLED';
  catalyst_headline: string;
  catalyst_source: string;
  catalyst_sentiment: number;
  entry_price: number;
  stop_loss: number;
  take_profit_1: number;
  take_profit_2?: number;
  leverage: number;
  risk_reward_ratio: number;
  allocated_capital_usd: number;
  allocated_capital_pct: number;
  exit_price?: number;
  exit_reason?: string;
  realized_pnl_usd?: number;
  realized_roi_pct?: number;
  created_at: string;
  closed_at?: string;
}

// Dynamic Macroeconomic Regime (US2)
export type MacroRegimeType = 'CRISIS' | 'NORMAL' | 'DOVISH_EXPANSION';

export interface MacroIndicators {
  geopolitical_index: number;
  inflation_index: number;
  interest_rate_index: number;
  active_conflicts?: string[];
  inflation_rate_yoy?: number;
  benchmark_rate?: number;
}

export interface MacroRegimeState {
  score: number;
  regime: MacroRegimeType;
  description: string;
  indicators: MacroIndicators;
  target_tier1_pct: number; // Cash
  target_core_pct: number;  // Core Commodities (Gold/Silver)
  target_alpha_pct: number; // Tactical Alpha
  last_updated: string;
}

// Telegram Signals Bot Integration (US3)
export interface TelegramConfigResponse {
  bot_token_configured: boolean;
  bot_token_masked: string;
  chat_id: string;
  enabled: boolean;
}

export interface TelegramConfigRequest {
  bot_token: string;
  chat_id: string;
  enabled: boolean;
}

// Real-Data Machine Learning Training & Model Telemetry (US6)
export interface MLTrainingRun {
  id: string;
  symbol: string;
  timeframe: string;
  sample_count: number;
  date_start: string;
  date_end: string;
  training_loss: number;
  directional_accuracy: number;
  weights_snapshot?: Record<string, number>;
  created_at: string;
}

export interface MLStatusResponse {
  is_training: boolean;
  hardware: string;
  cuda_enabled: boolean;
  device_name?: string;
  vram_allocated_mb?: number;
  vram_reserved_mb?: number;
  total_vram_mb?: number;
  current_loss?: number;
  current_val_accuracy?: number;
  peak_accuracy?: number;
  progress_pct?: number;
  elapsed_seconds?: number;
  active_run?: {
    symbol: string;
    epochs: number;
    current_epoch: number;
    train_loss: number;
    val_loss: number;
    val_accuracy: number;
    peak_val_accuracy: number;
  };
  bayesian_posteriors?: Record<string, { alpha: number; beta: number; mean: number; variance: number; expected_value?: number }>;
  current_weights?: Record<string, number>;
}






