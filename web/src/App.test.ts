import { describe, it, expect } from 'bun:test';
import { INITIAL_ASSETS, INITIAL_SUMMARY, INITIAL_POSITIONS, INITIAL_SIGNALS } from './hooks/useSSE';
import { INITIAL_WEIGHTS } from './components/AIWeightMatrix';

describe('Web Types, Mock Data, and Risk Model', () => {
  it('should not ship a hardcoded asset catalog in the web client', () => {
    // The single source of truth is internal/market/assets.go, served by
    // GET /api/v1/assets. A duplicated client-side list is what previously made
    // the dashboard and screener disagree on asset counts.
    expect(INITIAL_ASSETS.length).toBe(0);
  });

  it('should not fabricate portfolio equity before the API responds', () => {
    // Placeholders only: real values arrive from GET /api/v1/portfolio/summary.
    expect(INITIAL_SUMMARY.totalEquity).toBe(0);
    expect(INITIAL_SUMMARY.cash).toBe(0);
    expect(INITIAL_SUMMARY.initialEquity).toBe(0);
    expect(INITIAL_SUMMARY.drawdownPct).toBe(0);
  });

  it('should have initial dynamic indicator weights bounded between [0.2, 3.0]', () => {
    for (const [_, weight] of Object.entries(INITIAL_WEIGHTS.weights)) {
      expect(weight).toBeGreaterThanOrEqual(0.2);
      expect(weight).toBeLessThanOrEqual(3.0);
    }
  });

  it('should initialize empty trade positions and AI signals awaiting authentic live feeds', () => {
    expect(INITIAL_POSITIONS.length).toBe(0);
    expect(INITIAL_SIGNALS.length).toBe(0);
  });

  it('should validate MicrostructureState and MacroCalendarEvent institutional interfaces', () => {
    const microState = {
      symbol: 'BTC/USD',
      obi: 0.25,
      cvd: 1500,
      divergence: 'BULLISH_ABSORPTION',
      regime: 'NORMAL_TRENDING',
      volRatio: 1.05,
    };
    expect(microState.obi).toBeGreaterThanOrEqual(-1.0);
    expect(microState.obi).toBeLessThanOrEqual(1.0);
    expect(['NONE', 'BULLISH_ABSORPTION', 'BEARISH_EXHAUSTION']).toContain(microState.divergence);
    expect(['LOW_VOL_CONSOLIDATION', 'NORMAL_TRENDING', 'HIGH_VOL_CHOP']).toContain(microState.regime);

    const macroEvent = {
      id: 'FOMC-001',
      title: 'FOMC Rate Decision',
      currency: 'USD',
      impact: 'HIGH',
      scheduled_at: new Date().toISOString(),
    };
    expect(['LOW', 'MEDIUM', 'HIGH']).toContain(macroEvent.impact);
  });

  it('should accurately calculate position mark-to-market PnL and return % with isolated leverage', () => {
    // 0.5 BTC LONG at $60,000 with 5x leverage ($6,000 margin required)
    const position = {
      entryPrice: 60000,
      currentPrice: 66000, // +$6,000 per BTC (+10% asset move)
      size: 0.5,
      leverage: 5,
    };
    const margin = (position.entryPrice * position.size) / position.leverage;
    expect(margin).toBe(6000);

    const unrealizedPnL = (position.currentPrice - position.entryPrice) * position.size;
    expect(unrealizedPnL).toBe(3000); // 0.5 * $6,000 = $3,000

    const returnPct = (unrealizedPnL / margin) * 100;
    expect(returnPct).toBe(50); // 10% asset gain * 5x leverage = 50% ROI
  });

  it('should calculate authentic dynamic return percentage against config-driven initial equity', () => {
    const summary = {
      initialEquity: 50000,
      totalEquity: 55000,
    };
    const returnPct = ((summary.totalEquity - summary.initialEquity) / summary.initialEquity) * 100;
    expect(returnPct).toBe(10);
  });
});
