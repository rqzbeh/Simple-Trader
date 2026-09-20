import React, { useState, useEffect } from 'react';
import { Newspaper, TrendingUp, TrendingDown, Minus, ExternalLink, RefreshCw, AlertCircle } from 'lucide-react';
import { NewsArticle, NewsSentimentSummary } from '../types';

export const NewsStreamView: React.FC = () => {
  const [articles, setArticles] = useState<NewsArticle[]>([]);
  const [sentiment, setSentiment] = useState<NewsSentimentSummary | null>(null);
  const [loading, setLoading] = useState<boolean>(true);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  const fetchNews = async () => {
    try {
      setLoading(true);
      setErrorMsg(null);
      const res = await fetch('/api/v1/news/stream');
      if (!res.ok) {
        throw new Error(`Failed to load news: HTTP ${res.status}`);
      }
      const data = await res.json();
      setArticles(data.articles || []);
      setSentiment(data.sentiment || null);
    } catch (err: any) {
      console.error('Failed fetching news stream:', err);
      setErrorMsg(err.message || 'Error fetching news headlines');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchNews();
    const interval = setInterval(fetchNews, 30000); // 30s auto-refresh
    return () => clearInterval(interval);
  }, []);

  const getPolarityBadge = (polarity: string, score: number) => {
    switch (polarity) {
      case 'BULLISH':
        return (
          <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-full text-[10px] font-mono font-bold bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20">
            <TrendingUp className="w-3 h-3" />
            <span>BULLISH (+{(score * 100).toFixed(0)}%)</span>
          </span>
        );
      case 'BEARISH':
        return (
          <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-full text-[10px] font-mono font-bold bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/20">
            <TrendingDown className="w-3 h-3" />
            <span>BEARISH ({(score * 100).toFixed(0)}%)</span>
          </span>
        );
      default:
        return (
          <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-full text-[10px] font-mono font-bold bg-slate-500/10 text-slate-600 dark:text-slate-400 border border-slate-500/20">
            <Minus className="w-3 h-3" />
            <span>NEUTRAL</span>
          </span>
        );
    }
  };

  return (
    <div className="space-y-4">
      {/* Header with Sentiment Overview */}
      <div className="p-4 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 shadow-sm flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div>
          <div className="flex items-center space-x-2">
            <Newspaper className="w-4 h-4 text-sky-500" />
            <h3 className="text-sm font-bold font-mono text-slate-900 dark:text-slate-100">
              Live Macro & Crypto News Ingestion
            </h3>
            <span className="text-[10px] font-mono px-2 py-0.5 rounded bg-sky-500/10 text-sky-500 border border-sky-500/20 font-bold">
              SHA-256 Deduplication
            </span>
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
            Real-time multi-source crawler (Yahoo Finance, CoinDesk, CoinTelegraph) with hyperbolic tangent NLP polarity scoring.
          </p>
        </div>

        <div className="flex items-center space-x-3">
          {sentiment && (
            <div className="flex items-center space-x-2 px-3 py-1.5 rounded-lg border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/50 text-xs font-mono">
              <span className="text-slate-400">Aggregate Mood:</span>
              {getPolarityBadge(sentiment.polarity, sentiment.score)}
            </div>
          )}
          <button
            onClick={fetchNews}
            disabled={loading}
            className="p-2 rounded-lg border border-slate-200 dark:border-slate-800 hover:bg-slate-100 dark:hover:bg-slate-800 text-slate-600 dark:text-slate-300 transition-colors"
            title="Refresh Headlines"
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

      {/* Headlines List */}
      <div className="border border-slate-200 dark:border-slate-800 rounded-xl overflow-hidden bg-white dark:bg-slate-900/60 shadow-sm divide-y divide-slate-100 dark:divide-slate-800/60">
        {articles.length === 0 ? (
          <div className="p-8 text-center text-xs font-mono text-slate-400">
            {loading ? 'Ingesting real-time headlines...' : 'No news articles available. Feeds will populate shortly.'}
          </div>
        ) : (
          articles.map((art) => (
            <div key={art.content_hash} className="p-3.5 hover:bg-slate-50/50 dark:hover:bg-slate-800/20 transition-colors flex flex-col sm:flex-row sm:items-center justify-between gap-2">
              <div className="space-y-1 max-w-3xl">
                <div className="flex items-center space-x-2">
                  <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-400 font-semibold">
                    {art.source}
                  </span>
                  <span className="text-[10px] font-mono text-slate-400">
                    {new Date(art.published_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                  </span>
                </div>
                <a
                  href={art.url}
                  target="_blank"
                  rel="noreferrer"
                  className="text-xs font-medium text-slate-900 dark:text-slate-100 hover:text-sky-500 dark:hover:text-sky-400 transition-colors flex items-center space-x-1 group"
                >
                  <span>{art.title}</span>
                  <ExternalLink className="w-3 h-3 opacity-0 group-hover:opacity-100 transition-opacity" />
                </a>
              </div>
              <div className="shrink-0">
                {getPolarityBadge(art.polarity, art.sentiment_score)}
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
};
