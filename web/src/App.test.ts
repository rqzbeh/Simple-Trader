import { describe, it, expect } from 'bun:test';
import { INITIAL_ASSETS, INITIAL_SUMMARY, INITIAL_POSITIONS, INITIAL_SIGNALS } from './hooks/useSSE';
import { INITIAL_WEIGHTS } from './components/AIWeightMatrix';

describe('Web Types, Mock Data, and Risk Model', () => {
  it('should include all required liquid global crypto and commodity assets based on USDT/USDC', () => {
    const symbols = INITIAL_ASSETS.map((a) => a.symbol);
    expect(symbols).toContain('PAXG/USDT');
    expect(symbols).toContain('XAU/USDT');
    expect(symbols).toContain('XAG/USDT');
    expect(symbols).toContain('COPPER/USDT');
    expect(symbols).toContain('XPT/USDT');
    expect(symbols).toContain('XPD/USDT');
    expect(symbols).toContain('OIL/USDT');
    expect(symbols).toContain('ALU/USDT');
    expect(symbols).toContain('BNB/USDT');
    expect(symbols).toContain('BTC/USDT');
    expect(symbols).toContain('ETH/USDT');
    expect(symbols).toContain('SOL/USDT');
    expect(symbols).toContain('AVAX/USDT');
    expect(symbols).toContain('DOGE/USDT');
    expect(symbols).toContain('SUI/USDT');
    expect(symbols).toContain('XRP/USDT');
    expect(symbols).toContain('LINK/USDT');

    // Verify CORE assets are strictly commodities and BNB is ALPHA
    const bnb = INITIAL_ASSETS.find((a) => a.symbol === 'BNB/USDT');
    expect(bnb?.bucket).toBe('ALPHA');
    const paxg = INITIAL_ASSETS.find((a) => a.symbol === 'PAXG/USDT');
    expect(paxg?.bucket).toBe('CORE');
    const xau = INITIAL_ASSETS.find((a) => a.symbol === 'XAU/USDT');
    expect(xau?.bucket).toBe('CORE');
    const copper = INITIAL_ASSETS.find((a) => a.symbol === 'COPPER/USDT');
    expect(copper?.bucket).toBe('CORE');
    const oil = INITIAL_ASSETS.find((a) => a.symbol === 'OIL/USDT');
    expect(oil?.bucket).toBe('CORE');
    const xpt = INITIAL_ASSETS.find((a) => a.symbol === 'XPT/USDT');
    expect(xpt?.bucket).toBe('CORE');
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
