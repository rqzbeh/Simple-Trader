import { describe, it, expect } from 'bun:test';
import { getHealthStatus } from './SystemStatsView';

const base = (over: Partial<Record<string, unknown>> = {}) => ({
  total: 0, success: 0, fail: 0, success_rate: 0,
  ema_latency_ms: 0, last_latency_ms: 0, last_error: '', last_ok_at: null,
  ...over,
}) as never;

describe('getHealthStatus truth matrix (spec-022 FR-801)', () => {
  it('cold boot with zero traffic is STANDBY, not DOWN', () => {
    expect(getHealthStatus(base())).toEqual({ status: 'standby', label: 'STANDBY' });
  });

  it('healthy recent success is HEALTHY', () => {
    expect(getHealthStatus(base({
      total: 10, success: 10, last_ok_at: new Date().toISOString(),
    }))).toEqual({ status: 'healthy', label: 'HEALTHY' });
  });

  it('quiet event-driven window is IDLE, not DOWN', () => {
    const sixMinAgo = new Date(Date.now() - 6 * 60 * 1000).toISOString();
    expect(getHealthStatus(base({
      total: 17, success: 17, last_ok_at: sixMinAgo,
    }))).toEqual({ status: 'idle', label: 'IDLE (>5M)' });
  });

  it('recent failures with no success is DOWN', () => {
    expect(getHealthStatus(base({ total: 5, fail: 5 }))).toEqual({ status: 'down', label: 'DOWN' });
  });

  it('failures mixed with successes is DEGRADED', () => {
    expect(getHealthStatus(base({
      total: 10, success: 7, fail: 3, last_ok_at: new Date().toISOString(),
    }))).toEqual({ status: 'degraded', label: 'DEGRADED' });
  });
});
