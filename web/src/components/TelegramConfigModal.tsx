import React, { useState, useEffect } from 'react';
import { Send, CheckCircle2, AlertCircle, RefreshCw, X, MessageSquare } from 'lucide-react';
import { TelegramConfigResponse } from '../types';

interface TelegramConfigModalProps {
  isOpen: boolean;
  onClose: () => void;
  apiBaseUrl?: string;
}

export const TelegramConfigModal: React.FC<TelegramConfigModalProps> = ({
  isOpen,
  onClose,
  apiBaseUrl = '',
}) => {
  const [config, setConfig] = useState<TelegramConfigResponse | null>(null);
  const [botToken, setBotToken] = useState<string>('');
  const [chatID, setChatID] = useState<string>('');
  const [enabled, setEnabled] = useState<boolean>(true);

  const [loading, setLoading] = useState<boolean>(false);
  const [saving, setSaving] = useState<boolean>(false);
  const [testing, setTesting] = useState<boolean>(false);
  const [feedback, setFeedback] = useState<{ type: 'success' | 'error'; message: string } | null>(null);

  const fetchConfig = async () => {
    try {
      setLoading(true);
      const res = await fetch(`${apiBaseUrl}/api/v1/telegram/config`);
      if (!res.ok) throw new Error(`HTTP ${res.status}: Failed to load config`);
      const data: TelegramConfigResponse = await res.json();
      setConfig(data);
      setChatID(data.chat_id || '');
      setEnabled(data.enabled);
    } catch (err: any) {
      setFeedback({ type: 'error', message: err.message || 'Error loading config' });
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (isOpen) {
      setFeedback(null);
      fetchConfig();
    }
  }, [isOpen]);

  if (!isOpen) return null;

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setFeedback(null);

    try {
      const res = await fetch(`${apiBaseUrl}/api/v1/telegram/config`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          bot_token: botToken,
          chat_id: chatID,
          enabled: enabled,
        }),
      });

      if (!res.ok) {
        const errJson = await res.json().catch(() => ({}));
        throw new Error(errJson.error || `HTTP ${res.status}: Failed to save configuration`);
      }

      const updated: TelegramConfigResponse = await res.json();
      setConfig(updated);
      setBotToken('');
      setFeedback({ type: 'success', message: 'Telegram configuration saved successfully.' });
    } catch (err: any) {
      setFeedback({ type: 'error', message: err.message || 'Save failed' });
    } finally {
      setSaving(false);
    }
  };

  const handleTest = async () => {
    setTesting(true);
    setFeedback(null);

    try {
      const res = await fetch(`${apiBaseUrl}/api/v1/telegram/test`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          bot_token: botToken || undefined,
          chat_id: chatID || undefined,
        }),
      });

      const data = await res.json();
      if (!res.ok) {
        throw new Error(data.error || `HTTP ${res.status}: Test transmission failed`);
      }

      setFeedback({ type: 'success', message: 'Test message transmitted to Telegram successfully!' });
    } catch (err: any) {
      setFeedback({ type: 'error', message: err.message || 'Test delivery failed' });
    } finally {
      setTesting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-950/70 backdrop-blur-sm animate-fade-in">
      <div className="relative w-full max-w-md bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-2xl shadow-2xl overflow-hidden">
        {/* Modal Header */}
        <div className="p-5 border-b border-slate-100 dark:border-slate-800 flex items-center justify-between">
          <div className="flex items-center gap-2.5">
            <div className="p-2 rounded-xl bg-sky-500/10 text-sky-500">
              <MessageSquare className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-sm font-bold tracking-tight">Telegram Signals Bot</h3>
              <p className="text-[11px] text-slate-500 dark:text-slate-400">
                Institutional MarkdownV2 alerts & realized ROI reporting
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded-lg text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Modal Body */}
        <form onSubmit={handleSave} className="p-5 space-y-4">
          {/* Status Badge */}
          <div className="flex items-center justify-between p-3 rounded-xl bg-slate-50 dark:bg-slate-800/60 border border-slate-200/60 dark:border-slate-700/60">
            <span className="text-xs font-medium text-slate-600 dark:text-slate-300">Bot Connection Status</span>
            <span
              className={`px-2.5 py-1 rounded-full text-[10px] font-bold font-mono tracking-wide flex items-center gap-1.5 ${
                config?.enabled && config?.bot_token_configured
                  ? 'bg-emerald-500/10 text-emerald-500 border border-emerald-500/20'
                  : 'bg-amber-500/10 text-amber-500 border border-amber-500/20'
              }`}
            >
              {config?.enabled && config?.bot_token_configured ? (
                <>
                  <CheckCircle2 className="w-3 h-3" /> ACTIVE
                </>
              ) : (
                <>
                  <AlertCircle className="w-3 h-3" /> NOT CONFIGURED
                </>
              )}
            </span>
          </div>

          {/* Feedback message */}
          {feedback && (
            <div
              className={`p-3 rounded-xl text-xs font-mono flex items-center gap-2 ${
                feedback.type === 'success'
                  ? 'bg-emerald-500/10 border border-emerald-500/20 text-emerald-600 dark:text-emerald-400'
                  : 'bg-rose-500/10 border border-rose-500/20 text-rose-600 dark:text-rose-400'
              }`}
            >
              {feedback.type === 'success' ? (
                <CheckCircle2 className="w-4 h-4 shrink-0" />
              ) : (
                <AlertCircle className="w-4 h-4 shrink-0" />
              )}
              <span>{feedback.message}</span>
            </div>
          )}

          {/* Token Input */}
          <div className="space-y-1.5">
            <label className="text-xs font-semibold text-slate-700 dark:text-slate-300 flex items-center justify-between">
              <span>Bot API Token</span>
              {config?.bot_token_configured && (
                <span className="text-[10px] font-mono text-slate-400">
                  Current: {config.bot_token_masked}
                </span>
              )}
            </label>
            <input
              type="password"
              placeholder={config?.bot_token_configured ? 'Enter new token to overwrite' : 'e.g. 7952819240:AA...'}
              value={botToken}
              onChange={(e) => setBotToken(e.target.value)}
              className="w-full px-3 py-2 text-xs rounded-xl bg-slate-50 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 focus:outline-none focus:ring-2 focus:ring-sky-500 font-mono"
            />
          </div>

          {/* Chat ID Input */}
          <div className="space-y-1.5">
            <label className="text-xs font-semibold text-slate-700 dark:text-slate-300">
              Channel / Group / User Chat ID
            </label>
            <input
              type="text"
              placeholder="e.g. -100123456789 or 3239664627"
              value={chatID}
              onChange={(e) => setChatID(e.target.value)}
              className="w-full px-3 py-2 text-xs rounded-xl bg-slate-50 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 focus:outline-none focus:ring-2 focus:ring-sky-500 font-mono"
            />
          </div>

          {/* Enable Toggle */}
          <div className="flex items-center justify-between pt-1">
            <span className="text-xs font-medium text-slate-700 dark:text-slate-300">
              Enable Autonomous Broadcasts
            </span>
            <label className="relative inline-flex items-center cursor-pointer">
              <input
                type="checkbox"
                checked={enabled}
                onChange={(e) => setEnabled(e.target.checked)}
                className="sr-only peer"
              />
              <div className="w-9 h-5 bg-slate-200 peer-focus:outline-none rounded-full peer dark:bg-slate-700 peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-slate-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-sky-500"></div>
            </label>
          </div>

          {/* Action Buttons */}
          <div className="pt-3 flex items-center justify-between gap-3 border-t border-slate-100 dark:border-slate-800">
            <button
              type="button"
              onClick={handleTest}
              disabled={testing || (!config?.bot_token_configured && !botToken) || (!chatID && !config?.chat_id)}
              className="px-3.5 py-2 rounded-xl border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-xs font-semibold flex items-center gap-1.5 transition-colors disabled:opacity-40"
            >
              <Send className={`w-3.5 h-3.5 text-sky-500 ${testing ? 'animate-bounce' : ''}`} />
              <span>{testing ? 'Sending...' : 'Send Test Ping'}</span>
            </button>

            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={onClose}
                className="px-3.5 py-2 rounded-xl border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-xs font-semibold text-slate-600 dark:text-slate-300 transition-colors"
              >
                Close
              </button>
              <button
                type="submit"
                disabled={saving || loading}
                className="px-4 py-2 rounded-xl bg-sky-500 hover:bg-sky-600 text-white text-xs font-semibold shadow-sm flex items-center gap-1.5 transition-colors disabled:opacity-50"
              >
                <RefreshCw className={`w-3.5 h-3.5 ${saving ? 'animate-spin' : ''}`} />
                <span>{saving ? 'Saving...' : 'Save Settings'}</span>
              </button>
            </div>
          </div>
        </form>
      </div>
    </div>
  );
};
