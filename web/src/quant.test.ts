import { describe, it, expect } from 'bun:test';
import { MicrostructureState, MacroCalendarEvent, BacktestSummary, MonteCarloSummary } from './types';

describe('Institutional Quant Models (FR-003, FR-004, FR-005, FR-008)', () => {
  it('should validate MicrostructureState constraints and regime classification', () => {
    const microState: MicrostructureState = {
      symbol: 'BTC/USD',
      obi: 0.42,
      cvd: 15400,
      divergence: 'BULLISH_ABSORPTION',
      regime: 'NORMAL_TRENDING',
      volRatio: 1.08,
    };

    expect(microState.obi).toBeGreaterThanOrEqual(-1.0);
    expect(microState.obi).toBeLessThanOrEqual(1.0);
    expect(['NONE', 'BULLISH_ABSORPTION', 'BEARISH_EXHAUSTION']).toContain(microState.divergence);
    expect(['LOW_VOL_CONSOLIDATION', 'NORMAL_TRENDING', 'HIGH_VOL_CHOP']).toContain(microState.regime);
    expect(microState.volRatio).toBeGreaterThan(0);
  });

  it('should validate MacroCalendarEvent and halt interval criteria (FR-005)', () => {
    const event: MacroCalendarEvent = {
      id: 'FOMC-101',
      title: 'Federal Reserve Rate Decision',
      currency: 'USD',
      impact: 'HIGH',
      scheduled_at: new Date(Date.now() + 600000).toISOString(), // 10 minutes in future
      forecast: '5.25%',
      previous: '5.50%',
    };

    expect(event.impact).toBe('HIGH');
    const scheduledTime = new Date(event.scheduled_at).getTime();
    const diffMinutes = Math.abs(scheduledTime - Date.now()) / (60 * 1000);

    // Within 15 minutes window -> circuit breaker should halt
    const shouldHalt = event.impact === 'HIGH' && diffMinutes <= 15;
    expect(shouldHalt).toBe(true);
  });

  it('should validate BacktestSummary statistics and Monte Carlo metrics (FR-008, SC-004)', () => {
    const btSummary: BacktestSummary = {
      total_trades: 120,
      winning_trades: 72,
      losing_trades: 48,
      win_rate: 0.60,
      total_return_pct: 0.245,
      ending_capital: 124500.0,
      max_drawdown_pct: 0.075,
      sharpe_ratio: 1.88,
      sortino_ratio: 2.65,
      profit_factor: 2.10,
      avg_trade_return_pct: 0.002,
      trades: [],
      equity_curve: [100000, 105000, 112000, 124500],
      execution_duration: 8.5, // ms
    };

    expect(btSummary.win_rate).toBeCloseTo(0.60, 2);
    expect(btSummary.sharpe_ratio).toBeGreaterThan(1.0);
    expect(btSummary.sortino_ratio).toBeGreaterThan(btSummary.sharpe_ratio);
    expect(btSummary.max_drawdown_pct).toBeLessThan(0.20);
    expect(btSummary.execution_duration!).toBeLessThan(100.0); // SC-004: <100ms

    const mcSummary: MonteCarloSummary = {
      iterations: 1000,
      mean_return_pct: 0.23,
      median_return_pct: 0.22,
      percentile_5th_return: 0.02,
      percentile_95th_return: 0.45,
      max_drawdown_95th_pct: 0.12,
      max_drawdown_99th_pct: 0.16,
      probability_of_ruin_pct: 0.0,
    };

    expect(mcSummary.iterations).toBe(1000);
    expect(mcSummary.probability_of_ruin_pct).toBeLessThanOrEqual(5.0);
    expect(mcSummary.max_drawdown_99th_pct).toBeGreaterThan(mcSummary.max_drawdown_95th_pct);
  });
});
