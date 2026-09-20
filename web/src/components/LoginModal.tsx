import React, { useState } from 'react';
import { Lock, ShieldCheck, AlertTriangle, Eye, EyeOff, Loader2 } from 'lucide-react';
import { useAuth } from '../context/AuthContext';

export const LoginModal: React.FC = () => {
  const { login } = useAuth();
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!password.trim()) return;

    setError(null);
    setLoading(true);

    const result = await login(password);
    setLoading(false);

    if (!result.success) {
      setError(result.error || 'Invalid credentials');
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-md p-4 animate-in fade-in duration-200">
      <div className="w-full max-w-md bg-zinc-900 border border-zinc-800 rounded-2xl shadow-2xl p-6 text-zinc-100 flex flex-col items-center">
        {/* Shield Icon Header */}
        <div className="w-16 h-16 rounded-2xl bg-amber-500/10 border border-amber-500/20 flex items-center justify-center mb-4 text-amber-400">
          <ShieldCheck className="w-8 h-8" />
        </div>

        <h2 className="text-xl font-bold tracking-tight text-white mb-1">Simple-Trader Access Gate</h2>
        <p className="text-xs text-zinc-400 text-center mb-6">
          Institutional Algorithmic Quantitative Platform. Enter administrative credentials to access execution engines.
        </p>

        {error && (
          <div className="w-full mb-4 p-3 bg-red-950/40 border border-red-800/60 rounded-xl flex items-start gap-2 text-xs text-red-300">
            <AlertTriangle className="w-4 h-4 shrink-0 text-red-400 mt-0.5" />
            <span>{error}</span>
          </div>
        )}

        <form onSubmit={handleSubmit} className="w-full space-y-4">
          <div>
            <label className="block text-xs font-semibold text-zinc-400 mb-1.5 uppercase tracking-wider">
              Admin Password
            </label>
            <div className="relative">
              <div className="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none text-zinc-500">
                <Lock className="w-4 h-4" />
              </div>
              <input
                type={showPassword ? 'text' : 'password'}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Enter password..."
                required
                className="w-full pl-10 pr-10 py-2.5 bg-zinc-950 border border-zinc-700/80 rounded-xl text-sm text-white placeholder-zinc-500 focus:outline-none focus:border-amber-500 focus:ring-1 focus:ring-amber-500 transition-colors"
              />
              <button
                type="button"
                onClick={() => setShowPassword(!showPassword)}
                className="absolute inset-y-0 right-0 pr-3 flex items-center text-zinc-500 hover:text-zinc-300"
              >
                {showPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
              </button>
            </div>
          </div>

          <button
            type="submit"
            disabled={loading || !password.trim()}
            className="w-full py-2.5 px-4 bg-amber-500 hover:bg-amber-400 active:bg-amber-600 disabled:bg-zinc-800 disabled:text-zinc-500 text-zinc-950 font-semibold text-sm rounded-xl transition-all duration-150 flex items-center justify-center gap-2 shadow-lg shadow-amber-500/10 cursor-pointer disabled:cursor-not-allowed"
          >
            {loading ? (
              <>
                <Loader2 className="w-4 h-4 animate-spin" />
                <span>Verifying Authentication...</span>
              </>
            ) : (
              <span>Authorize Session</span>
            )}
          </button>
        </form>

        <div className="mt-6 text-[10px] text-zinc-500 text-center space-y-1">
          <div>Protected by Bcrypt (Cost 12) &amp; Sliding Window Rate Limiting</div>
          <div>Max 5 failed attempts per 15-minute window before lockout.</div>
        </div>
      </div>
    </div>
  );
};
