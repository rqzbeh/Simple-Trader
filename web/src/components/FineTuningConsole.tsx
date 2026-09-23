import React, { useState } from 'react';
import { Download, Terminal, Settings, CheckCircle2 } from 'lucide-react';

export const FineTuningConsole: React.FC = () => {
  const [modelId, setModelId] = useState<string>('');
  const [apiKey, setApiKey] = useState<string>('');
  const [endpoint, setEndpoint] = useState<string>('Configured via Environment');
  const [isSaved, setIsSaved] = useState<boolean>(false);
  const [isDownloading, setIsDownloading] = useState<boolean>(false);

  React.useEffect(() => {
    fetch('/api/v1/system/config')
      .then((res) => (res.ok ? res.json() : null))
      .then((data) => {
        if (data?.ai_model_id) setModelId(data.ai_model_id);
      })
      .catch(() => {});
  }, []);

  const handleSave = (e: React.FormEvent) => {
    e.preventDefault();
    setIsSaved(true);
    setTimeout(() => setIsSaved(false), 2500);
  };

  const handleDownloadDataset = async () => {
    setIsDownloading(true);
    try {
      const res = await fetch('/api/v1/learning/dataset.jsonl');
      if (res.ok) {
        const text = await res.text();
        const blob = new Blob([text], { type: 'application/jsonl' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `dataset_${Date.now()}.jsonl`;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
      } else {
        alert('No closed trade datasets available from database for export.');
      }
    } catch (err: any) {
      alert(err.message || 'Dataset export failed');
    } finally {
      setIsDownloading(false);
    }
  };

  return (
    <div className="p-5 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/60 shadow-sm flex flex-col space-y-4">
      <div className="flex items-center justify-between pb-3 border-b border-slate-100 dark:border-slate-800/80">
        <div className="flex items-center space-x-2">
          <Terminal className="w-4 h-4 text-indigo-500" />
          <h3 className="font-semibold text-sm tracking-tight text-slate-800 dark:text-slate-100">
            OpenAI Unified API & Continuous Fine-Tuning Console
          </h3>
        </div>
        <span className="text-xs text-slate-400 font-mono">Compatible with Groq, vLLM, Ollama, OpenAI</span>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Settings Form */}
        <form onSubmit={handleSave} className="space-y-3">
          <div className="flex items-center space-x-1.5 text-xs font-semibold text-slate-700 dark:text-slate-300">
            <Settings className="w-3.5 h-3.5" />
            <span>Endpoint Configuration</span>
          </div>

          <div>
            <label className="block text-[11px] font-mono text-slate-500 mb-1">
              API Base URL
            </label>
            <input
              type="text"
              value={endpoint}
              onChange={(e) => setEndpoint(e.target.value)}
              placeholder="https://api.openai.com/v1"
              className="w-full px-3 py-1.5 text-xs font-mono bg-slate-50 dark:bg-slate-800/50 border border-slate-200 dark:border-slate-700 rounded-lg focus:outline-none focus:ring-1 focus:ring-sky-500"
            />
          </div>

          <div>
            <label className="block text-[11px] font-mono text-slate-500 mb-1">
              Model ID
            </label>
            <input
              type="text"
              value={modelId}
              onChange={(e) => setModelId(e.target.value)}
              placeholder="Configured via environment (.env)"
              className="w-full px-3 py-1.5 text-xs font-mono bg-slate-50 dark:bg-slate-800/50 border border-slate-200 dark:border-slate-700 rounded-lg focus:outline-none focus:ring-1 focus:ring-sky-500"
            />
          </div>

          <div>
            <label className="block text-[11px] font-mono text-slate-500 mb-1">
              API Secret Key
            </label>
            <input
              type="password"
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              placeholder="sk-..."
              className="w-full px-3 py-1.5 text-xs font-mono bg-slate-50 dark:bg-slate-800/50 border border-slate-200 dark:border-slate-700 rounded-lg focus:outline-none focus:ring-1 focus:ring-sky-500"
            />
          </div>

          <button
            type="submit"
            className="w-full py-1.5 px-3 rounded-lg bg-indigo-500 hover:bg-indigo-600 text-white text-xs font-semibold flex items-center justify-center space-x-1.5 transition-colors shadow-sm"
          >
            {isSaved ? <CheckCircle2 className="w-3.5 h-3.5" /> : null}
            <span>{isSaved ? 'Settings Applied' : 'Save AI Configuration'}</span>
          </button>
        </form>

        {/* Dataset Export Box */}
        <div className="flex flex-col justify-between p-4 rounded-lg bg-slate-50 dark:bg-slate-800/30 border border-slate-100 dark:border-slate-800/80 space-y-3">
          <div>
            <h4 className="text-xs font-bold text-slate-800 dark:text-slate-200 mb-1">
              Continuous Fine-Tuning Pipeline
            </h4>
            <p className="text-xs text-slate-500 dark:text-slate-400 leading-relaxed">
              Every completed paper trade generates a ChatML training pair attributing whether indicator weights improved or harmed trade expectancy.
            </p>
          </div>

          <div className="p-2.5 rounded bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 font-mono text-[10px] text-slate-600 dark:text-slate-300">
            <code>
              {`{"messages":[{"role":"system","content":"Autonomous trader..."},{"role":"user","content":"Indicators..."},{"role":"assistant","content":"Decision..."}]}`}
            </code>
          </div>

          <button
            onClick={handleDownloadDataset}
            disabled={isDownloading}
            className="w-full py-2 px-3 rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-white dark:hover:bg-slate-800 text-slate-700 dark:text-slate-200 text-xs font-mono font-medium flex items-center justify-center space-x-2 transition-colors shadow-sm"
          >
            <Download className="w-3.5 h-3.5 text-sky-500" />
            <span>{isDownloading ? 'Preparing JSONL...' : 'Download dataset.jsonl'}</span>
          </button>
        </div>
      </div>
    </div>
  );
};
