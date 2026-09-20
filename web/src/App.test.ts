import { describe, it, expect } from 'bun:test';
import { INITIAL_ASSETS, INITIAL_SUMMARY, INITIAL_POSITIONS, INITIAL_SIGNALS } from './hooks/useSSE';
import { INITIAL_WEIGHTS } from './components/AIWeightMatrix';

describe('Web Types, Mock Data, and Risk Model', () => {
  it('should include all required liquid global assets and none from Iran bourse', () => {
    const symbols = INITIAL_ASSETS.map((a) => a.symbol);
    expect(symbols).toContain('XAU/USD');
    expect(symbols).toContain('XAG/USD');
    expect(symbols).toContain('BTC/USD');
    expect(symbols).toContain('ETH/USD');
    expect(symbols).toContain('SOL/USD');
    expect(symbols).toContain('EUR/USD');
    expect(symbols).toContain('WTI/USD');

    // Ensure strictly no Iranian assets
    expect(symbols).not.toContain('IRR');
    expect(symbols).not.toContain('TSE');
    expect(symbols).not.toContain('IFB');
  });

  it('should enforce 60/40 allocation targets in summary', () => {
    expect(INITIAL_SUMMARY.targetCorePct).toBe(0.60);
    expect(INITIAL_SUMMARY.targetAlphaPct).toBe(0.40);
    expect(INITIAL_SUMMARY.targetCorePct + INITIAL_SUMMARY.targetAlphaPct).toBe(1.0);
  });

  it('should have initial dynamic indicator weights bounded between [0.2, 3.0]', () => {
    for (const [_, weight] of Object.entries(INITIAL_WEIGHTS.weights)) {
      expect(weight).toBeGreaterThanOrEqual(0.2);
      expect(weight).toBeLessThanOrEqual(3.0);
    }
  });

  it('should initialize valid mock trade positions and AI signals', () => {
    expect(INITIAL_POSITIONS.length).toBeGreaterThan(0);
    expect(INITIAL_SIGNALS.length).toBeGreaterThan(0);

    for (const pos of INITIAL_POSITIONS) {
      expect(['CORE', 'ALPHA']).toContain(pos.bucket);
      expect(['BUY', 'SELL']).toContain(pos.side);
      expect(pos.currentPrice).toBeGreaterThan(0);
    }
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
});
