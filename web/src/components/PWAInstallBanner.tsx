import React from 'react';
import { Download, X, Smartphone, ArrowUpRight } from 'lucide-react';
import { usePWAInstall } from '../hooks/usePWAInstall';

interface PWAInstallBannerProps {
  onShowIOSGuide?: () => void;
}

export const PWAInstallBanner: React.FC<PWAInstallBannerProps> = ({ onShowIOSGuide }) => {
  const { isInstallable, isIOS, isStandalone, triggerInstall } = usePWAInstall();
  const [dismissed, setDismissed] = React.useState(false);

  // If already running standalone or dismissed, don't show
  if (isStandalone || dismissed) {
    return null;
  }

  // Show Chromium/Android install button or iOS Safari prompt trigger
  if (!isInstallable && !isIOS) {
    return null;
  }

  const handleInstallClick = async () => {
    if (isInstallable) {
      await triggerInstall();
    } else if (isIOS && onShowIOSGuide) {
      onShowIOSGuide();
    }
  };

  return (
    <div className="fixed bottom-4 left-4 right-4 sm:left-auto sm:right-4 sm:w-96 z-40 bg-slate-900/95 border border-sky-500/30 text-white rounded-2xl shadow-2xl p-4 backdrop-blur-md animate-in slide-in-from-bottom duration-300">
      <div className="flex items-start justify-between gap-3">
        <div className="w-10 h-10 rounded-xl bg-sky-500/20 border border-sky-500/40 flex items-center justify-center text-sky-400 shrink-0">
          <Smartphone className="w-5 h-5" />
        </div>

        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-1.5 font-bold text-sm text-white">
            <span>Install Simple-Trader App</span>
            <span className="text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded bg-sky-500/20 text-sky-400 border border-sky-500/30">
              PWA
            </span>
          </div>
          <p className="text-xs text-slate-300 mt-0.5 leading-relaxed">
            {isIOS
              ? 'Add to iOS Home Screen for instant full-screen execution & push alerts.'
              : 'Install the native-feel desktop/mobile app for low-latency quant trading.'}
          </p>

          <div className="mt-3 flex items-center gap-2">
            <button
              onClick={handleInstallClick}
              className="px-3.5 py-1.5 bg-sky-500 hover:bg-sky-400 active:bg-sky-600 text-slate-950 font-bold text-xs rounded-lg transition-colors flex items-center gap-1.5 shadow-sm shadow-sky-500/30"
            >
              {isIOS ? (
                <>
                  <ArrowUpRight className="w-3.5 h-3.5" />
                  <span>How to Install</span>
                </>
              ) : (
                <>
                  <Download className="w-3.5 h-3.5" />
                  <span>Install App</span>
                </>
              )}
            </button>

            <button
              onClick={() => setDismissed(true)}
              className="px-3 py-1.5 bg-slate-800 hover:bg-slate-700 text-slate-400 hover:text-slate-200 text-xs rounded-lg transition-colors"
            >
              Not now
            </button>
          </div>
        </div>

        <button
          onClick={() => setDismissed(true)}
          className="p-1 rounded-lg text-slate-400 hover:text-white hover:bg-slate-800 transition-colors"
          title="Dismiss banner"
        >
          <X className="w-4 h-4" />
        </button>
      </div>
    </div>
  );
};
