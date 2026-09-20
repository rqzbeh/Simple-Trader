import React from 'react';
import { X, Share, PlusSquare, CheckCircle, Smartphone } from 'lucide-react';

interface IOSInstallModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export const IOSInstallModal: React.FC<IOSInstallModalProps> = ({ isOpen, onClose }) => {
  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-md p-4 animate-in fade-in duration-200">
      <div className="w-full max-w-md bg-slate-900 border border-slate-800 rounded-2xl shadow-2xl p-6 text-slate-100 flex flex-col relative">
        <button
          onClick={onClose}
          className="absolute top-4 right-4 p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-slate-800 transition-colors"
          title="Close guide"
        >
          <X className="w-4 h-4" />
        </button>

        <div className="flex items-center gap-3 mb-4">
          <div className="w-12 h-12 rounded-xl bg-sky-500/20 border border-sky-500/40 flex items-center justify-center text-sky-400 shrink-0">
            <Smartphone className="w-6 h-6" />
          </div>
          <div>
            <h3 className="text-base font-bold text-white">Install on iOS Safari</h3>
            <p className="text-xs text-slate-400">Add Simple-Trader to your Home Screen</p>
          </div>
        </div>

        <div className="space-y-4 my-2 text-xs">
          {/* Step 1 */}
          <div className="p-3.5 rounded-xl bg-slate-800/80 border border-slate-700/60 flex items-start gap-3">
            <div className="w-6 h-6 rounded-lg bg-sky-500/20 text-sky-400 font-bold flex items-center justify-center shrink-0 text-xs">
              1
            </div>
            <div>
              <div className="font-semibold text-white flex items-center gap-1.5">
                <span>Tap the Share button in Safari</span>
                <Share className="w-3.5 h-3.5 text-sky-400 inline" />
              </div>
              <p className="text-slate-400 mt-1 leading-relaxed">
                Look at the bottom toolbar of Safari (or top right on iPad) and tap the square icon with the upward arrow.
              </p>
            </div>
          </div>

          {/* Step 2 */}
          <div className="p-3.5 rounded-xl bg-slate-800/80 border border-slate-700/60 flex items-start gap-3">
            <div className="w-6 h-6 rounded-lg bg-emerald-500/20 text-emerald-400 font-bold flex items-center justify-center shrink-0 text-xs">
              2
            </div>
            <div>
              <div className="font-semibold text-white flex items-center gap-1.5">
                <span>Select &quot;Add to Home Screen&quot;</span>
                <PlusSquare className="w-3.5 h-3.5 text-emerald-400 inline" />
              </div>
              <p className="text-slate-400 mt-1 leading-relaxed">
                Scroll down through the share actions menu and tap <span className="text-white font-medium">&quot;Add to Home Screen&quot;</span>.
              </p>
            </div>
          </div>

          {/* Step 3 */}
          <div className="p-3.5 rounded-xl bg-slate-800/80 border border-slate-700/60 flex items-start gap-3">
            <div className="w-6 h-6 rounded-lg bg-amber-500/20 text-amber-400 font-bold flex items-center justify-center shrink-0 text-xs">
              3
            </div>
            <div>
              <div className="font-semibold text-white flex items-center gap-1.5">
                <span>Tap &quot;Add&quot; in the top-right</span>
                <CheckCircle className="w-3.5 h-3.5 text-amber-400 inline" />
              </div>
              <p className="text-slate-400 mt-1 leading-relaxed">
                Confirm the title &quot;Simple-Trader&quot; and tap <span className="text-white font-medium">&quot;Add&quot;</span>. The app icon will appear immediately on your home screen!
              </p>
            </div>
          </div>
        </div>

        <div className="mt-4 pt-4 border-t border-slate-800 flex justify-end">
          <button
            onClick={onClose}
            className="w-full py-2.5 px-4 bg-sky-500 hover:bg-sky-400 active:bg-sky-600 text-slate-950 font-bold text-xs rounded-xl transition-colors shadow-sm"
          >
            Got it, take me to trading!
          </button>
        </div>
      </div>
    </div>
  );
};
