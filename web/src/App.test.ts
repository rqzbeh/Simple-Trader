import { describe, it, expect } from 'bun:test';
import { INITIAL_ASSETS, INITIAL_SUMMARY } from './hooks/useSSE';

describe('Web Types and Mock Data', () => {
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
});
