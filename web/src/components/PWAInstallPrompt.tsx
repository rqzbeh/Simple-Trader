import React, { useState } from 'react';
import { Download, Share, PlusSquare, X, Smartphone, CheckCircle } from 'lucide-react';
import { usePWAInstall } from '../hooks/usePWAInstall';

export const PWAInstallPrompt: React.FC = () => {
  const { isInstallable, isIOS, isStandalone, isDismissed, triggerInstall, dismissPrompt } = usePWAInstall();
  const [showIOSModal, setShowIOSModal] = useState(false);

  // If already running standalone or user dismissed recently, don't display
  if (isStandalone || isDismissed) {
    return null;
  }

  // Neither installable (Chromium/Android) nor iOS Safari
  if (!isInstallable && !isIOS) {
    return null;
  }

  return (
    <>
      {/* Non-intrusive Floating Install Banner */}
      <div className="fixed bottom-4 left-4 right-4 sm:left-auto sm:right-6 sm:max-w-md z-40 animate-in slide-in-from-bottom-5 duration-300">
        <div className="p-4 rounded-2xl bg-slate-900/95 border border-slate-700/80 shadow-2xl backdrop-blur-md flex items-center justify-between gap-4 text-white">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-xl bg-gradient-to-tr from-sky-500 to-indigo-600 flex items-center justify-center shrink-0 shadow-md shadow-sky-500/20">
              <Smartphone className="w-5 h-5 text-white" />
            </div>
            <div>
              <h4 className="text-xs font-bold tracking-tight">Install Simple-Trader App</h4>
              <p className="text-[11px] text-slate-400">
                {isIOS
                  ? 'Add to iOS Home Screen for full screen mode & instant alerts.'
                  : 'Fast, native-like algorithmic trading experience.'}
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2 shrink-0">
            {isInstallable && (
              <button
                onClick={triggerInstall}
                className="px-3 py-1.5 bg-sky-500 hover:bg-sky-400 text-slate-950 font-bold text-xs rounded-xl flex items-center gap-1.5 transition-all shadow-md shadow-sky-500/20"
              >
                <Download className="w-3.5 h-3.5" />
                <span>Install</span>
              </button>
            )}

            {isIOS && (
              <button
                onClick={() => setShowIOSModal(true)}
                className="px-3 py-1.5 bg-sky-500 hover:bg-sky-400 text-slate-950 font-bold text-xs rounded-xl flex items-center gap-1.5 transition-all shadow-md shadow-sky-500/20"
              >
                <Share className="w-3.5 h-3.5" />
                <span>How to Install</span>
              </button>
            )}

            <button
              onClick={dismissPrompt}
              className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-slate-800 transition-colors"
              title="Dismiss"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>
      </div>

      {/* iOS Step-by-Step Guidance Modal */}
      {showIOSModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-md p-4 animate-in fade-in duration-200">
          <div className="w-full max-w-sm bg-slate-900 border border-slate-700/80 rounded-2xl shadow-2xl p-6 text-slate-100 flex flex-col relative">
            <button
              onClick={() => setShowIOSModal(false)}
              className="absolute top-4 right-4 p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-slate-800 transition-colors"
            >
              <X className="w-4 h-4" />
            </button>

            <div className="w-12 h-12 rounded-xl bg-sky-500/10 border border-sky-500/20 flex items-center justify-center mb-4 text-sky-400 self-center">
              <Smartphone className="w-6 h-6" />
            </div>

            <h3 className="text-base font-bold text-center text-white mb-2">
              Install Simple-Trader on iOS
            </h3>
            <p className="text-xs text-slate-400 text-center mb-6">
              Install on your iPhone or iPad for zero-latency execution view and full-screen trading workspace.
            </p>

            <div className="space-y-4 text-xs">
              <div className="flex items-start gap-3 p-3 rounded-xl bg-slate-800/60 border border-slate-700/50">
                <div className="w-6 h-6 rounded-lg bg-sky-500/20 text-sky-400 flex items-center justify-center shrink-0 font-bold">
                  1
                </div>
                <div>
                  <div className="font-semibold text-slate-200 flex items-center gap-1.5">
                    Tap the Share button
                    <Share className="w-3.5 h-3.5 text-sky-400 inline" />
                  </div>
                  <div className="text-slate-400 text-[11px] mt-0.5">
                    In Safari's bottom toolbar, tap the square icon with an upward arrow.
                  </div>
                </div>
              </div>

              <div className="flex items-start gap-3 p-3 rounded-xl bg-slate-800/60 border border-slate-700/50">
                <div className="w-6 h-6 rounded-lg bg-sky-500/20 text-sky-400 flex items-center justify-center shrink-0 font-bold">
                  2
                </div>
                <div>
                  <div className="font-semibold text-slate-200 flex items-center gap-1.5">
                    Select "Add to Home Screen"
                    <PlusSquare className="w-3.5 h-3.5 text-emerald-400 inline" />
                  </div>
                  <div className="text-slate-400 text-[11px] mt-0.5">
                    Scroll down the share sheet and tap <span className="text-white font-medium">Add to Home Screen</span>.
                  </div>
                </div>
              </div>

              <div className="flex items-start gap-3 p-3 rounded-xl bg-slate-800/60 border border-slate-700/50">
                <div className="w-6 h-6 rounded-lg bg-sky-500/20 text-sky-400 flex items-center justify-center shrink-0 font-bold">
                  3
                </div>
                <div>
                  <div className="font-semibold text-slate-200 flex items-center gap-1.5">
                    Tap "Add" in top right corner
                    <CheckCircle className="w-3.5 h-3.5 text-sky-400 inline" />
                  </div>
                  <div className="text-slate-400 text-[11px] mt-0.5">
                    Confirm the app name and tap <span className="text-white font-medium">Add</span> to complete installation.
                  </div>
                </div>
              </div>
            </div>

            <button
              onClick={() => {
                setShowIOSModal(false);
                dismissPrompt();
              }}
              className="mt-6 w-full py-2.5 bg-sky-500 hover:bg-sky-400 text-slate-950 font-semibold text-xs rounded-xl transition-all"
            >
              Got it, thanks!
            </button>
          </div>
        </div>
      )}
    </>
  );
};
