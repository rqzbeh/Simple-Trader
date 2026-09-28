import React, { useState, useEffect, useCallback } from 'react';
import {
  Server,
  Zap,
  RefreshCw,
  AlertCircle,
  CheckCircle2,
  Clock,
  Shield,
  Key,
  Sliders,
  Save,
  FileText,
} from 'lucide-react';
import { getAuthToken } from '../context/AuthContext';
import { SystemConfigResponse } from '../types';

export interface GatewayStatsData {
  total: number;
  success: number;
  fail: number;
  success_rate: number;
  ema_latency_ms: number;
  last_latency_ms: number;
  last_error: string;
  last_ok_at: string | null;
}

export interface JevStatsData extends GatewayStatsData {
  model: string;
  key_configured: boolean;
}

export interface SystemStatsResponse {
  version: string;
  uptime_seconds: number;
  routing_threshold?: number;
  gateway: GatewayStatsData;
  jev: JevStatsData;
}

type StatusLevel = 'healthy' | 'degraded' | 'down';

function getHealthStatus(stats: GatewayStatsData): { status: StatusLevel; label: string } {
  if (!stats.last_ok_at || stats.success === 0) {
    return { status: 'down', label: 'DOWN' };
  }
  const okTime = new Date(stats.last_ok_at).getTime();
  const fiveMinAgo = Date.now() - 5 * 60 * 1000;
  if (okTime < fiveMinAgo) {
    return { status: 'down', label: 'DOWN (>5M STALE)' };
  }
  if (stats.fail > 0) {
    return { status: 'degraded', label: 'DEGRADED' };
  }
  return { status: 'healthy', label: 'HEALTHY' };
}

function formatUptime(seconds: number): string {
  if (seconds <= 0) return '0s';
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);
  const parts: string[] = [];
  if (d > 0) parts.push(`${d}d`);
  if (h > 0) parts.push(`${h}h`);
  if (m > 0) parts.push(`${m}m`);
  if (s > 0 || parts.length === 0) parts.push(`${s}s`);
  return parts.join(' ');
}

function formatLastOK(isoString: string | null): string {
  if (!isoString) return 'Never';
  const diffSec = Math.max(0, Math.floor((Date.now() - new Date(isoString).getTime()) / 1000));
  if (diffSec < 5) return 'Just now';
  if (diffSec < 60) return `${diffSec}s ago`;
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`;
  return `${Math.floor(diffSec / 3600)}h ago`;
}

export const SystemStatsView: React.FC = () => {
  const [stats, setStats] = useState<SystemStatsResponse | null>(null);
  const [config, setConfig] = useState<SystemConfigResponse | null>(null);
  const [loading, setLoading] = useState<boolean>(true);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null);

  // Decision Core editable state
  const [thresholdInput, setThresholdInput] = useState<string>('0.75');
  const [typesafeKeyInput, setTypesafeKeyInput] = useState<string>('');
  const [savingConfig, setSavingConfig] = useState<boolean>(false);
  const [configFeedback, setConfigFeedback] = useState<{ type: 'success' | 'error'; message: string } | null>(null);

  const fetchConfig = useCallback(async () => {
    try {
      const res = await fetch('/api/v1/system/config');
      if (res.ok) {
        const data: SystemConfigResponse = await res.json();
        setConfig(data);
        if (data.routing_confidence_threshold != null) {
          setThresholdInput(String(data.routing_confidence_threshold));
        }
      }
    } catch (err) {
      console.warn('Failed to load system config:', err);
    }
  }, []);

  const fetchStats = useCallback(async () => {
    try {
      setErrorMsg(null);
      const res = await fetch('/api/v1/system/stats');
      if (!res.ok) {
        throw new Error(`HTTP ${res.status}: Failed to load system telemetry`);
      }
      const data: SystemStatsResponse = await res.json();
      setStats(data);
      setLastUpdated(new Date());
    } catch (err: any) {
      console.error('Failed to fetch system stats:', err);
      setErrorMsg(err.message || 'Error fetching system stats');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchStats();
    fetchConfig();
    const interval = setInterval(fetchStats, 10000); // 10s auto-refresh
    return () => clearInterval(interval);
  }, [fetchStats, fetchConfig]);

  const handleSaveDecisionCore = async (e: React.FormEvent) => {
    e.preventDefault();
    setSavingConfig(true);
    setConfigFeedback(null);

    const val = parseFloat(thresholdInput);
    if (isNaN(val) || val <= 0 || val > 1.0) {
      setConfigFeedback({
        type: 'error',
        message: 'ROUTING_CONFIDENCE_THRESHOLD must be between 0 (exclusive) and 1 (inclusive)',
      });
      setSavingConfig(false);
      return;
    }

    try {
      const payload: Record<string, any> = {
        ROUTING_CONFIDENCE_THRESHOLD: thresholdInput,
      };
      if (typesafeKeyInput.trim()) {
        payload['TYPESAFE_API_KEY'] = typesafeKeyInput.trim();
      }

      const headers: Record<string, string> = {
        'Content-Type': 'application/json',
      };
      const token = getAuthToken();
      if (token) {
        headers['Authorization'] = `Bearer ${token}`;
      }

      const res = await fetch('/api/v1/system/config', {
        method: 'PUT',
        headers,
        credentials: 'include',
        body: JSON.stringify(payload),
      });

      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        throw new Error(data.error || `HTTP ${res.status}: Failed to update config`);
      }

      setConfig(data);
      if (data.routing_confidence_threshold != null) {
        setThresholdInput(String(data.routing_confidence_threshold));
      }
      setTypesafeKeyInput('');
      setConfigFeedback({
        type: 'success',
        message: `Decision Core settings persisted to ${data.env_file || '.env'} and applied live to DecisionRouter.`,
      });
      fetchStats();
    } catch (err: any) {
      setConfigFeedback({
        type: 'error',
        message: err.message || 'Failed to update Decision Core settings',
      });
    } finally {
      setSavingConfig(false);
    }
  };

  const renderStatusBadge = (statusInfo: { status: StatusLevel; label: string }) => {
    switch (statusInfo.status) {
      case 'healthy':
        return (
          <span className="inline-flex items-center space-x-1.5 px-2.5 py-1 rounded-full text-[10px] font-mono font-bold bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20">
            <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse"></span>
            <span>{statusInfo.label}</span>
          </span>
        );
      case 'degraded':
        return (
          <span className="inline-flex items-center space-x-1.5 px-2.5 py-1 rounded-full text-[10px] font-mono font-bold bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20">
            <span className="w-2 h-2 rounded-full bg-amber-500"></span>
            <span>{statusInfo.label}</span>
          </span>
        );
      case 'down':
      default:
        return (
          <span className="inline-flex items-center space-x-1.5 px-2.5 py-1 rounded-full text-[10px] font-mono font-bold bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/20">
            <span className="w-2 h-2 rounded-full bg-rose-500"></span>
            <span>{statusInfo.label}</span>
          </span>
        );
    }
  };

  return (
    <div className="space-y-4 font-mono">
      {/* Overview & Polling Bar */}
      <div className="p-4 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 shadow-sm flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div>
          <div className="flex items-center space-x-2">
            <Server className="w-4 h-4 text-sky-500" />
            <h3 className="text-sm font-bold text-slate-900 dark:text-slate-100">
              System Telemetry & AI Gateway Monitor
            </h3>
            <span className="text-[10px] px-2 py-0.5 rounded bg-sky-500/10 text-sky-500 border border-sky-500/20 font-bold">
              v{stats?.version || '3.0.0'}
            </span>
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
            Real-time in-process operational metrics for 9Router AI gateway and TypeSafe Jev decision engine.
          </p>
        </div>

        <div className="flex items-center space-x-3">
          <div className="text-right text-[11px] text-slate-400">
            <span>Poll: 10s</span>
            {lastUpdated && (
              <span className="ml-2 hidden md:inline">
                Updated: {lastUpdated.toLocaleTimeString()}
              </span>
            )}
          </div>
          <button
            onClick={() => {
              setLoading(true);
              fetchStats();
              fetchConfig();
            }}
            disabled={loading}
            className="p-2 rounded-lg border border-slate-200 dark:border-slate-800 hover:bg-slate-100 dark:hover:bg-slate-800 text-slate-600 dark:text-slate-300 transition-colors"
            title="Refresh Telemetry"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>

      {errorMsg && (
        <div className="p-3 bg-rose-500/10 border border-rose-500/20 text-rose-600 dark:text-rose-400 rounded-xl text-xs flex items-center space-x-2">
          <AlertCircle className="w-4 h-4 shrink-0" />
          <span>{errorMsg}</span>
        </div>
      )}

      {/* Main Grid: 9Router AI Gateway & TypeSafe Jev Engine */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 sm:gap-6">
        {/* Card 1: 9Router Gateway */}
        <div className="p-5 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 shadow-sm space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center space-x-2.5">
              <div className="w-8 h-8 rounded-lg bg-sky-500/10 border border-sky-500/20 flex items-center justify-center text-sky-500">
                <Zap className="w-4 h-4" />
              </div>
              <div>
                <h4 className="text-sm font-bold text-slate-900 dark:text-slate-100">
                  9Router AI Gateway
                </h4>
                <p className="text-[11px] text-slate-400">
                  Chat Completions & News NLP Classifier
                </p>
              </div>
            </div>
            {stats && renderStatusBadge(getHealthStatus(stats.gateway))}
          </div>

          {/* Stats Metrics */}
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-3 pt-2">
            <div className="p-2.5 rounded-lg bg-slate-50 dark:bg-slate-800/40 border border-slate-100 dark:border-slate-800">
              <span className="text-[10px] text-slate-400 uppercase font-semibold">Total Requests</span>
              <p className="text-base font-bold text-slate-800 dark:text-slate-200 mt-0.5">
                {stats ? stats.gateway.total.toLocaleString() : '—'}
              </p>
            </div>
            <div className="p-2.5 rounded-lg bg-slate-50 dark:bg-slate-800/40 border border-slate-100 dark:border-slate-800">
              <span className="text-[10px] text-slate-400 uppercase font-semibold">Success</span>
              <p className="text-base font-bold text-emerald-600 dark:text-emerald-400 mt-0.5">
                {stats ? stats.gateway.success.toLocaleString() : '—'}
              </p>
            </div>
            <div className="p-2.5 rounded-lg bg-slate-50 dark:bg-slate-800/40 border border-slate-100 dark:border-slate-800">
              <span className="text-[10px] text-slate-400 uppercase font-semibold">Failed</span>
              <p className="text-base font-bold text-rose-500 mt-0.5">
                {stats ? stats.gateway.fail.toLocaleString() : '—'}
              </p>
            </div>
            <div className="p-2.5 rounded-lg bg-slate-50 dark:bg-slate-800/40 border border-slate-100 dark:border-slate-800">
              <span className="text-[10px] text-slate-400 uppercase font-semibold">Success Rate</span>
              <p className="text-base font-bold text-sky-500 mt-0.5">
                {stats && stats.gateway.total > 0
                  ? `${(stats.gateway.success_rate * 100).toFixed(1)}%`
                  : '—'}
              </p>
            </div>
            <div className="p-2.5 rounded-lg bg-slate-50 dark:bg-slate-800/40 border border-slate-100 dark:border-slate-800">
              <span className="text-[10px] text-slate-400 uppercase font-semibold">Last Latency</span>
              <p className="text-base font-bold text-slate-800 dark:text-slate-200 mt-0.5">
                {stats && stats.gateway.last_latency_ms > 0
                  ? `${stats.gateway.last_latency_ms.toFixed(1)}ms`
                  : '—'}
              </p>
            </div>
            <div className="p-2.5 rounded-lg bg-slate-50 dark:bg-slate-800/40 border border-slate-100 dark:border-slate-800">
              <span className="text-[10px] text-slate-400 uppercase font-semibold">EMA Latency</span>
              <p className="text-base font-bold text-purple-500 mt-0.5">
                {stats && stats.gateway.ema_latency_ms > 0
                  ? `${stats.gateway.ema_latency_ms.toFixed(1)}ms`
                  : '—'}
              </p>
            </div>
          </div>

          <div className="pt-2 border-t border-slate-100 dark:border-slate-800/60 flex items-center justify-between text-[11px] text-slate-400">
            <span>Last Successful Call:</span>
            <span className="font-semibold text-slate-600 dark:text-slate-300">
              {stats ? formatLastOK(stats.gateway.last_ok_at) : '—'}
            </span>
          </div>

          {stats?.gateway.last_error && (
            <div className="p-2.5 rounded-lg bg-rose-500/10 border border-rose-500/20 text-rose-600 dark:text-rose-400 text-[11px] break-all">
              <span className="font-bold">Last Error: </span>
              {stats.gateway.last_error}
            </div>
          )}
        </div>

        {/* Card 2: TypeSafe Jev System One */}
        <div className="p-5 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 shadow-sm space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center space-x-2.5">
              <div className="w-8 h-8 rounded-lg bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-500">
                <Shield className="w-4 h-4" />
              </div>
              <div>
                <h4 className="text-sm font-bold text-slate-900 dark:text-slate-100">
                  TypeSafe Jev Engine
                </h4>
                <p className="text-[11px] text-slate-400">
                  System One • Parallel Quantitative Arbiter
                </p>
              </div>
            </div>
            {stats && renderStatusBadge(getHealthStatus(stats.jev))}
          </div>

          {/* Model & Key Configuration Badges */}
          <div className="flex flex-wrap items-center gap-2 text-[10px]">
            <span className="px-2 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700">
              Model: <span className="font-bold">{stats?.jev.model || 'jev-latest'}</span>
            </span>
            {stats?.jev.key_configured ? (
              <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20 font-bold">
                <CheckCircle2 className="w-3 h-3" />
                <span>API Key Configured</span>
              </span>
            ) : (
              <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/20 font-bold">
                <Key className="w-3 h-3" />
                <span>TYPESAFE_API_KEY Not Set</span>
              </span>
            )}
          </div>

          {/* Stats Metrics */}
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-3 pt-1">
            <div className="p-2.5 rounded-lg bg-slate-50 dark:bg-slate-800/40 border border-slate-100 dark:border-slate-800">
              <span className="text-[10px] text-slate-400 uppercase font-semibold">Total Requests</span>
              <p className="text-base font-bold text-slate-800 dark:text-slate-200 mt-0.5">
                {stats ? stats.jev.total.toLocaleString() : '—'}
              </p>
            </div>
            <div className="p-2.5 rounded-lg bg-slate-50 dark:bg-slate-800/40 border border-slate-100 dark:border-slate-800">
              <span className="text-[10px] text-slate-400 uppercase font-semibold">Success</span>
              <p className="text-base font-bold text-emerald-600 dark:text-emerald-400 mt-0.5">
                {stats ? stats.jev.success.toLocaleString() : '—'}
              </p>
            </div>
            <div className="p-2.5 rounded-lg bg-slate-50 dark:bg-slate-800/40 border border-slate-100 dark:border-slate-800">
              <span className="text-[10px] text-slate-400 uppercase font-semibold">Failed</span>
              <p className="text-base font-bold text-rose-500 mt-0.5">
                {stats ? stats.jev.fail.toLocaleString() : '—'}
              </p>
            </div>
            <div className="p-2.5 rounded-lg bg-slate-50 dark:bg-slate-800/40 border border-slate-100 dark:border-slate-800">
              <span className="text-[10px] text-slate-400 uppercase font-semibold">Success Rate</span>
              <p className="text-base font-bold text-emerald-500 mt-0.5">
                {stats && stats.jev.total > 0
                  ? `${(stats.jev.success_rate * 100).toFixed(1)}%`
                  : '—'}
              </p>
            </div>
            <div className="p-2.5 rounded-lg bg-slate-50 dark:bg-slate-800/40 border border-slate-100 dark:border-slate-800">
              <span className="text-[10px] text-slate-400 uppercase font-semibold">Last Latency</span>
              <p className="text-base font-bold text-slate-800 dark:text-slate-200 mt-0.5">
                {stats && stats.jev.last_latency_ms > 0
                  ? `${stats.jev.last_latency_ms.toFixed(1)}ms`
                  : '—'}
              </p>
            </div>
            <div className="p-2.5 rounded-lg bg-slate-50 dark:bg-slate-800/40 border border-slate-100 dark:border-slate-800">
              <span className="text-[10px] text-slate-400 uppercase font-semibold">EMA Latency</span>
              <p className="text-base font-bold text-purple-500 mt-0.5">
                {stats && stats.jev.ema_latency_ms > 0
                  ? `${stats.jev.ema_latency_ms.toFixed(1)}ms`
                  : '—'}
              </p>
            </div>
          </div>

          <div className="pt-2 border-t border-slate-100 dark:border-slate-800/60 flex items-center justify-between text-[11px] text-slate-400">
            <span>Last Successful Call:</span>
            <span className="font-semibold text-slate-600 dark:text-slate-300">
              {stats ? formatLastOK(stats.jev.last_ok_at) : '—'}
            </span>
          </div>

          {stats?.jev.last_error && (
            <div className="p-2.5 rounded-lg bg-rose-500/10 border border-rose-500/20 text-rose-600 dark:text-rose-400 text-[11px] break-all">
              <span className="font-bold">Last Error: </span>
              {stats.jev.last_error}
            </div>
          )}
        </div>
      </div>

      {/* Card 3: Runtime Overview & ROUTING Threshold */}
      <div className="p-5 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 shadow-sm space-y-4">
        <div className="flex items-center space-x-2">
          <Clock className="w-4 h-4 text-purple-500" />
          <h4 className="text-sm font-bold text-slate-900 dark:text-slate-100">
            Runtime Architecture & Routing Thresholds
          </h4>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
          <div className="p-3 rounded-lg bg-slate-50 dark:bg-slate-800/40 border border-slate-100 dark:border-slate-800 space-y-1">
            <span className="text-[10px] text-slate-400 uppercase font-semibold">System Uptime</span>
            <p className="text-lg font-bold text-slate-800 dark:text-slate-200">
              {stats ? formatUptime(stats.uptime_seconds) : '—'}
            </p>
            <p className="text-[10px] text-slate-400">Continuous in-process runtime</p>
          </div>

          <div className="p-3 rounded-lg bg-slate-50 dark:bg-slate-800/40 border border-slate-100 dark:border-slate-800 space-y-1">
            <span className="text-[10px] text-slate-400 uppercase font-semibold">Routing Confidence Threshold</span>
            <p className="text-lg font-bold text-sky-500">
              {stats && stats.routing_threshold != null
                ? `${(stats.routing_threshold * 100).toFixed(0)}% (${stats.routing_threshold.toFixed(2)})`
                : 'Default'}
            </p>
            <p className="text-[10px] text-slate-400">
              Escalates to slow 9Router brain when Jev confidence is below this threshold
            </p>
          </div>

          <div className="p-3 rounded-lg bg-slate-50 dark:bg-slate-800/40 border border-slate-100 dark:border-slate-800 space-y-1">
            <span className="text-[10px] text-slate-400 uppercase font-semibold">Decision Core Mode</span>
            <p className="text-lg font-bold text-emerald-500">Dual-Brain Live</p>
            <p className="text-[10px] text-slate-400">
              TypeSafe Fast Arbiter + 9Router Escalation Core
            </p>
          </div>
        </div>
      </div>

      {/* Card 4: Decision Core Settings */}
      <div className="p-5 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 shadow-sm space-y-4">
        <div className="flex items-center justify-between pb-3 border-b border-slate-100 dark:border-slate-800">
          <div className="flex items-center space-x-2.5">
            <div className="w-8 h-8 rounded-lg bg-sky-500/10 border border-sky-500/20 flex items-center justify-center text-sky-500">
              <Sliders className="w-4 h-4" />
            </div>
            <div>
              <h4 className="text-sm font-bold text-slate-900 dark:text-slate-100">
                Decision Core Settings
              </h4>
              <p className="text-[11px] text-slate-400">
                Jev System One & 9Router Escalation Parameters • Persists to .env
              </p>
            </div>
          </div>

          {config?.env_file && (
            <span className="hidden sm:inline-flex items-center space-x-1 px-2.5 py-1 rounded-lg bg-slate-100 dark:bg-slate-800 text-[10px] text-slate-500 dark:text-slate-400 border border-slate-200 dark:border-slate-700">
              <FileText className="w-3 h-3 text-slate-400" />
              <span>Target: {config.env_file}</span>
            </span>
          )}
        </div>

        {/* Feedback message */}
        {configFeedback && (
          <div
            className={`p-3 rounded-xl text-xs font-mono flex items-center gap-2 ${
              configFeedback.type === 'success'
                ? 'bg-emerald-500/10 border border-emerald-500/20 text-emerald-600 dark:text-emerald-400'
                : 'bg-rose-500/10 border border-rose-500/20 text-rose-600 dark:text-rose-400'
            }`}
          >
            {configFeedback.type === 'success' ? (
              <CheckCircle2 className="w-4 h-4 shrink-0" />
            ) : (
              <AlertCircle className="w-4 h-4 shrink-0" />
            )}
            <span>{configFeedback.message}</span>
          </div>
        )}

        <form onSubmit={handleSaveDecisionCore} className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {/* Routing Confidence Threshold Input */}
            <div className="space-y-1.5 p-3 rounded-lg bg-slate-50 dark:bg-slate-800/40 border border-slate-100 dark:border-slate-800">
              <div className="flex items-center justify-between">
                <label className="text-xs font-semibold text-slate-700 dark:text-slate-300">
                  Routing Confidence Threshold (0 - 1)
                </label>
                <span className="text-[10px] font-bold text-sky-500 font-mono">
                  {thresholdInput ? `${(parseFloat(thresholdInput) * 100).toFixed(0)}% (${parseFloat(thresholdInput).toFixed(2)})` : '—'}
                </span>
              </div>
              <input
                type="number"
                min="0.05"
                max="1.0"
                step="0.05"
                value={thresholdInput}
                onChange={(e) => setThresholdInput(e.target.value)}
                required
                className="w-full px-3 py-2 text-xs rounded-xl bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 focus:outline-none focus:ring-2 focus:ring-sky-500 font-mono text-slate-900 dark:text-slate-100"
              />
              <p className="text-[10px] text-slate-400">
                Signals with Jev confidence below this threshold escalate to 9Router slow brain.
              </p>
            </div>

            {/* TypeSafe API Key & Base URL Info */}
            <div className="space-y-1.5 p-3 rounded-lg bg-slate-50 dark:bg-slate-800/40 border border-slate-100 dark:border-slate-800">
              <div className="flex items-center justify-between">
                <label className="text-xs font-semibold text-slate-700 dark:text-slate-300 flex items-center gap-1.5">
                  <Key className="w-3.5 h-3.5 text-emerald-500" />
                  <span>TypeSafe API Key</span>
                </label>
                <span className="text-[10px] text-slate-400 font-mono">
                  {config?.typesafe_api_key_configured
                    ? `Current: ${config.typesafe_api_key_masked || 'Configured'}`
                    : 'Not Configured'}
                </span>
              </div>
              <input
                type="password"
                placeholder={config?.typesafe_api_key_configured ? 'Enter new key to overwrite' : 'apikey_...'}
                value={typesafeKeyInput}
                onChange={(e) => setTypesafeKeyInput(e.target.value)}
                className="w-full px-3 py-2 text-xs rounded-xl bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 focus:outline-none focus:ring-2 focus:ring-sky-500 font-mono text-slate-900 dark:text-slate-100"
              />
              <p className="text-[10px] text-slate-400">
                Base URL: <span className="font-mono text-slate-500">{config?.typesafe_base_url || 'https://api.typesafe.ai'}</span>
              </p>
            </div>
          </div>

          <div className="flex items-center justify-between pt-2 border-t border-slate-100 dark:border-slate-800">
            <span className="text-[11px] text-slate-400">
              Single Source of Truth: edits persist across docker compose rebuilds.
            </span>
            <button
              type="submit"
              disabled={savingConfig}
              className="px-4 py-2 rounded-xl bg-sky-500 hover:bg-sky-600 text-white text-xs font-semibold shadow-sm flex items-center gap-1.5 transition-colors disabled:opacity-50"
            >
              <Save className={`w-3.5 h-3.5 ${savingConfig ? 'animate-spin' : ''}`} />
              <span>{savingConfig ? 'Saving to .env...' : 'Save Decision Core'}</span>
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
