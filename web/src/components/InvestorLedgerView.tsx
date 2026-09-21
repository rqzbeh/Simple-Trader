import React, { useState, useEffect } from 'react';
import { Users, Plus, ArrowDownRight, ArrowUpRight, DollarSign, AlertCircle, RefreshCw, Layers, ShieldAlert } from 'lucide-react';
import { Investor, TierAllocationBreakdown } from '../types';

export const InvestorLedgerView: React.FC = () => {
  const [investors, setInvestors] = useState<Investor[]>([]);
  const [tierBreakdown, setTierBreakdown] = useState<TierAllocationBreakdown | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  // Modal states
  const [showCreateModal, setShowCreateModal] = useState<boolean>(false);
  const [showDepositModal, setShowDepositModal] = useState<boolean>(false);
  const [showWithdrawModal, setShowWithdrawModal] = useState<boolean>(false);
  const [selectedInvestor, setSelectedInvestor] = useState<Investor | null>(null);

  // Form states
  const [formName, setFormName] = useState('');
  const [formContact, setFormContact] = useState('');
  const [formNotes, setFormNotes] = useState('');
  const [formInitialDeposit, setFormInitialDeposit] = useState<number>(10000);
  const [txAmount, setTxAmount] = useState<number>(1000);
  const [txNotes, setTxNotes] = useState('');
  const [isSubmitting, setIsSubmitting] = useState<boolean>(false);

  const fetchData = async () => {
    setIsLoading(true);
    setErrorMsg(null);
    try {
      const [invRes, tierRes] = await Promise.all([
        fetch('/api/v1/investors').catch(() => null),
        fetch('/api/v1/allocator/tiers').catch(() => null),
      ]);

      if (invRes && invRes.ok) {
        const data = await invRes.json();
        setInvestors(Array.isArray(data) ? data : []);
      } else {
        // Fallback demo data if offline/fresh instance
        setInvestors([
          {
            id: 'inv-001',
            name: 'Aethelgard Capital LP',
            contact_tag: '@aethelgard_partners',
            notes: 'Institutional seed allocator - priority settlement',
            total_deposited: 50000.0,
            total_withdrawn: 0.0,
            pool_units: 50000.0,
            status: 'ACTIVE',
            current_equity: 54225.5,
            roi: 0.0845,
            pool_share_pct: 54.2,
            created_at: new Date(Date.now() - 30 * 86400000).toISOString(),
            updated_at: new Date().toISOString(),
          },
          {
            id: 'inv-002',
            name: 'Valhalla Family Office',
            contact_tag: 'ops@valhalla-fo.ch',
            notes: 'Core gold/silver focus',
            total_deposited: 40000.0,
            total_withdrawn: 5000.0,
            pool_units: 35000.0,
            status: 'ACTIVE',
            current_equity: 37958.0,
            roi: 0.0845,
            pool_share_pct: 37.9,
            created_at: new Date(Date.now() - 20 * 86400000).toISOString(),
            updated_at: new Date().toISOString(),
          },
        ]);
      }

      if (tierRes && tierRes.ok) {
        const tData = await tierRes.json();
        setTierBreakdown(tData);
      } else {
        setTierBreakdown({
          tier1_cash: 15000,
          tier1_pct: 15.0,
          tier2_core: 45000,
          tier2_pct: 45.0,
          tier3_alpha: 40000,
          tier3_pct: 40.0,
          total_capital: 100000,
        });
      }
    } catch (e: any) {
      setErrorMsg(e?.message || 'Failed fetching ledger data');
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, []);

  const handleCreateInvestor = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!formName || !formContact) return;
    setIsSubmitting(true);
    setErrorMsg(null);
    try {
      const res = await fetch('/api/v1/investors', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: formName,
          contact_tag: formContact,
          notes: formNotes,
          initial_deposit: Number(formInitialDeposit),
        }),
      });

      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err.error || 'Failed to create investor');
      }

      setShowCreateModal(false);
      setFormName('');
      setFormContact('');
      setFormNotes('');
      setFormInitialDeposit(10000);
      await fetchData();
    } catch (err: any) {
      setErrorMsg(err.message);
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleDeposit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedInvestor || txAmount <= 0) return;
    setIsSubmitting(true);
    setErrorMsg(null);
    try {
      const res = await fetch(`/api/v1/investors/${selectedInvestor.id}/deposit`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          amount: Number(txAmount),
          notes: txNotes,
        }),
      });

      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err.error || 'Deposit rejected');
      }

      setShowDepositModal(false);
      setTxAmount(1000);
      setTxNotes('');
      await fetchData();
    } catch (err: any) {
      setErrorMsg(err.message);
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleWithdrawal = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedInvestor || txAmount <= 0) return;
    setIsSubmitting(true);
    setErrorMsg(null);
    try {
      const res = await fetch(`/api/v1/investors/${selectedInvestor.id}/withdraw`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          amount: Number(txAmount),
          notes: txNotes,
        }),
      });

      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err.error || 'Withdrawal rejected');
      }

      setShowWithdrawModal(false);
      setTxAmount(1000);
      setTxNotes('');
      await fetchData();
    } catch (err: any) {
      setErrorMsg(err.message);
    } finally {
      setIsSubmitting(false);
    }
  };

  const totalFundEquity = investors.reduce((sum, inv) => sum + (inv.current_equity || 0), 0);
  const totalFundDeposited = investors.reduce((sum, inv) => sum + (inv.total_deposited || 0), 0);

  return (
    <div className="space-y-6">
      {/* 3-Tier Multi-Horizon Liquidity Allocator Status */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {/* Tier 1 Cash Card */}
        <div className="p-4 rounded-xl border border-emerald-500/20 bg-emerald-500/5 dark:bg-emerald-950/20 shadow-sm flex flex-col justify-between">
          <div className="flex items-center justify-between text-xs text-emerald-600 dark:text-emerald-400 font-mono mb-2">
            <span className="flex items-center space-x-1.5 font-bold uppercase">
              <DollarSign className="w-3.5 h-3.5" />
              <span>Tier 1: Instant Cash Buffer (15%)</span>
            </span>
            <span className="px-2 py-0.5 rounded text-[10px] bg-emerald-500/20 text-emerald-600 dark:text-emerald-300 font-bold">
              Unencumbered
            </span>
          </div>
          <div className="text-2xl font-bold font-mono text-emerald-600 dark:text-emerald-400">
            ${(tierBreakdown?.tier1_cash ?? 15000).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
          </div>
          <div className="text-xs text-slate-500 dark:text-slate-400 mt-2 font-mono flex items-center justify-between">
            <span>Guarantees instant redemptions</span>
            <span>Target: {tierBreakdown?.tier1_pct ?? 15.0}%</span>
          </div>
        </div>

        {/* Tier 2 Core Assets Card */}
        <div className="p-4 rounded-xl border border-amber-500/20 bg-amber-500/5 dark:bg-amber-950/20 shadow-sm flex flex-col justify-between">
          <div className="flex items-center justify-between text-xs text-amber-600 dark:text-amber-400 font-mono mb-2">
            <span className="flex items-center space-x-1.5 font-bold uppercase">
              <Layers className="w-3.5 h-3.5" />
              <span>Tier 2: Core Wealth (45%)</span>
            </span>
            <span className="px-2 py-0.5 rounded text-[10px] bg-amber-500/20 text-amber-600 dark:text-amber-300 font-bold">
              Macro Horizon
            </span>
          </div>
          <div className="text-2xl font-bold font-mono text-amber-600 dark:text-amber-400">
            ${(tierBreakdown?.tier2_core ?? 45000).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
          </div>
          <div className="text-xs text-slate-500 dark:text-slate-400 mt-2 font-mono flex items-center justify-between">
            <span>Gold (XAU) & Silver (XAG)</span>
            <span>Target: {tierBreakdown?.tier2_pct ?? 45.0}%</span>
          </div>
        </div>

        {/* Tier 3 Tactical Alpha Card */}
        <div className="p-4 rounded-xl border border-indigo-500/20 bg-indigo-500/5 dark:bg-indigo-950/20 shadow-sm flex flex-col justify-between">
          <div className="flex items-center justify-between text-xs text-indigo-600 dark:text-indigo-400 font-mono mb-2">
            <span className="flex items-center space-x-1.5 font-bold uppercase">
              <ArrowUpRight className="w-3.5 h-3.5" />
              <span>Tier 3: Tactical Alpha (40%)</span>
            </span>
            <span className="px-2 py-0.5 rounded text-[10px] bg-indigo-500/20 text-indigo-600 dark:text-indigo-300 font-bold">
              2h Swing Bars
            </span>
          </div>
          <div className="text-2xl font-bold font-mono text-indigo-600 dark:text-indigo-400">
            ${(tierBreakdown?.tier3_alpha ?? 40000).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
          </div>
          <div className="text-xs text-slate-500 dark:text-slate-400 mt-2 font-mono flex items-center justify-between">
            <span>Sweeps profit to Tier 1 cash</span>
            <span>Target: {tierBreakdown?.tier3_pct ?? 40.0}%</span>
          </div>
        </div>
      </div>

      {/* Error / Alert banner */}
      {errorMsg && (
        <div className="p-3 rounded-xl border border-rose-500/30 bg-rose-500/10 text-rose-600 dark:text-rose-400 text-xs font-mono flex items-center space-x-2">
          <AlertCircle className="w-4 h-4 flex-shrink-0" />
          <span>{errorMsg}</span>
        </div>
      )}

      {/* Header with Actions */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 border-b border-slate-200 dark:border-slate-800 pb-4">
        <div>
          <h2 className="text-base font-bold font-mono flex items-center space-x-2 text-slate-900 dark:text-slate-100">
            <Users className="w-4 h-4 text-sky-500" />
            <span>INVESTOR CAPITAL LEDGER</span>
          </h2>
          <p className="text-xs text-slate-500 dark:text-slate-400 font-mono mt-0.5">
            Institutional Unitized NAV Share Accounting • Non-Diluting Pro-Rata Allocations • Protected Liquid Cash Buffer
          </p>
          <div className="flex items-center space-x-4 mt-2 text-xs font-mono text-slate-500 dark:text-slate-400">
            <span>Total Pool Equity: <strong className="text-emerald-500">${totalFundEquity.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}</strong></span>
            <span>Total Capital Inflow: <strong className="text-sky-500">${totalFundDeposited.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}</strong></span>
          </div>
        </div>

        <div className="flex items-center space-x-2">
          <button
            onClick={fetchData}
            disabled={isLoading}
            className="px-3 py-1.5 rounded-lg border border-slate-200 dark:border-slate-800 text-xs font-mono text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors flex items-center space-x-1.5"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${isLoading ? 'animate-spin' : ''}`} />
            <span>Refresh</span>
          </button>
          <button
            onClick={() => setShowCreateModal(true)}
            className="px-3 py-1.5 rounded-lg bg-sky-500 hover:bg-sky-600 text-white text-xs font-mono font-semibold shadow-sm transition-colors flex items-center space-x-1.5"
          >
            <Plus className="w-3.5 h-3.5" />
            <span>Record New Investor</span>
          </button>
        </div>
      </div>

      {/* Investors Table */}
      <div className="border border-slate-200 dark:border-slate-800 rounded-xl overflow-hidden bg-white dark:bg-slate-900/60 shadow-sm">
        <div className="overflow-x-auto">
          <table className="w-full text-left border-collapse text-xs font-mono">
            <thead>
              <tr className="border-b border-slate-200 dark:border-slate-800 bg-slate-50/50 dark:bg-slate-800/30 text-slate-500 dark:text-slate-400">
                <th className="py-3 px-4 font-semibold">Investor Profile</th>
                <th className="py-3 px-4 font-semibold text-right">Deposited</th>
                <th className="py-3 px-4 font-semibold text-right">Current Equity</th>
                <th className="py-3 px-4 font-semibold text-right">ROI (%)</th>
                <th className="py-3 px-4 font-semibold text-right">Pool Share</th>
                <th className="py-3 px-4 font-semibold text-center">Status</th>
                <th className="py-3 px-4 font-semibold text-right">Capital Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800/60">
              {investors.length === 0 ? (
                <tr>
                  <td colSpan={7} className="py-8 text-center text-slate-400 font-mono">
                    No investors recorded yet. Click "Record New Investor" to initialize the capital pool.
                  </td>
                </tr>
              ) : (
                investors.map((inv) => {
                  const roiPct = (inv.roi || 0) * 100;
                  const isPositive = roiPct >= 0;
                  return (
                    <tr key={inv.id} className="hover:bg-slate-50/50 dark:hover:bg-slate-800/30 transition-colors">
                      <td className="py-3 px-4">
                        <div className="font-semibold text-slate-900 dark:text-slate-100">{inv.name}</div>
                        <div className="text-[11px] text-slate-400">{inv.contact_tag}</div>
                        {inv.notes && <div className="text-[10px] text-slate-500 italic mt-0.5">{inv.notes}</div>}
                      </td>
                      <td className="py-3 px-4 text-right font-bold text-slate-700 dark:text-slate-300">
                        ${inv.total_deposited.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
                      </td>
                      <td className="py-3 px-4 text-right font-bold text-sky-600 dark:text-sky-400 text-sm">
                        ${(inv.current_equity ?? inv.total_deposited).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
                      </td>
                      <td className="py-3 px-4 text-right font-bold">
                        <span className={`inline-flex items-center px-1.5 py-0.5 rounded text-[11px] ${
                          isPositive
                            ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20'
                            : 'bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/20'
                        }`}>
                          {isPositive ? '+' : ''}{roiPct.toFixed(2)}%
                        </span>
                      </td>
                      <td className="py-3 px-4 text-right font-semibold text-slate-600 dark:text-slate-300">
                        {(inv.pool_share_pct || 0).toFixed(1)}%
                      </td>
                      <td className="py-3 px-4 text-center">
                        <span className="px-2 py-0.5 rounded text-[10px] bg-emerald-500/10 text-emerald-500 font-bold border border-emerald-500/20">
                          {inv.status}
                        </span>
                      </td>
                      <td className="py-3 px-4 text-right space-x-1.5">
                        <button
                          onClick={() => {
                            setSelectedInvestor(inv);
                            setShowDepositModal(true);
                          }}
                          className="px-2.5 py-1 rounded bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-emerald-600 dark:text-emerald-400 text-[11px] font-semibold transition-colors"
                        >
                          + Deposit
                        </button>
                        <button
                          onClick={() => {
                            setSelectedInvestor(inv);
                            setShowWithdrawModal(true);
                          }}
                          className="px-2.5 py-1 rounded bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-rose-600 dark:text-rose-400 text-[11px] font-semibold transition-colors"
                        >
                          - Withdraw
                        </button>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Modal: Create Investor */}
      {showCreateModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/60 backdrop-blur-sm">
          <div className="w-full max-w-md bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl p-6 shadow-2xl space-y-4">
            <h3 className="text-sm font-bold font-mono text-slate-900 dark:text-slate-100 flex items-center space-x-2">
              <Users className="w-4 h-4 text-sky-500" />
              <span>Record New Investor & Initial Capital</span>
            </h3>
            <form onSubmit={handleCreateInvestor} className="space-y-3 font-mono text-xs">
              <div>
                <label className="block text-slate-500 dark:text-slate-400 mb-1">Investor / Entity Legal Name</label>
                <input
                  type="text"
                  required
                  placeholder="e.g. Apex Strategic Fund LP"
                  value={formName}
                  onChange={(e) => setFormName(e.target.value)}
                  className="w-full px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/60 text-slate-900 dark:text-slate-100 focus:outline-none focus:border-sky-500"
                />
              </div>
              <div>
                <label className="block text-slate-500 dark:text-slate-400 mb-1">Contact Reference / Handle</label>
                <input
                  type="text"
                  required
                  placeholder="e.g. ops@apexcapital.io or @apex_fund"
                  value={formContact}
                  onChange={(e) => setFormContact(e.target.value)}
                  className="w-full px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/60 text-slate-900 dark:text-slate-100 focus:outline-none focus:border-sky-500"
                />
              </div>
              <div>
                <label className="block text-slate-500 dark:text-slate-400 mb-1">Initial Capital Contribution ($ USD)</label>
                <input
                  type="number"
                  min="1"
                  step="0.01"
                  required
                  value={formInitialDeposit}
                  onChange={(e) => setFormInitialDeposit(parseFloat(e.target.value) || 0)}
                  className="w-full px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/60 text-slate-900 dark:text-slate-100 focus:outline-none focus:border-sky-500"
                />
              </div>
              <div>
                <label className="block text-slate-500 dark:text-slate-400 mb-1">Accounting Notes</label>
                <textarea
                  placeholder="e.g. Seed tranche under Series A partnership terms"
                  value={formNotes}
                  onChange={(e) => setFormNotes(e.target.value)}
                  rows={2}
                  className="w-full px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/60 text-slate-900 dark:text-slate-100 focus:outline-none focus:border-sky-500"
                />
              </div>
              <div className="flex items-center justify-end space-x-2 pt-2">
                <button
                  type="button"
                  onClick={() => setShowCreateModal(false)}
                  className="px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-800 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={isSubmitting}
                  className="px-4 py-2 rounded-lg bg-sky-500 hover:bg-sky-600 text-white font-semibold"
                >
                  {isSubmitting ? 'Recording...' : 'Register Capital'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Modal: Record Deposit */}
      {showDepositModal && selectedInvestor && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/60 backdrop-blur-sm">
          <div className="w-full max-w-md bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl p-6 shadow-2xl space-y-4">
            <h3 className="text-sm font-bold font-mono text-slate-900 dark:text-slate-100 flex items-center space-x-2">
              <ArrowDownRight className="w-4 h-4 text-emerald-500" />
              <span>Record Capital Deposit: {selectedInvestor.name}</span>
            </h3>
            <form onSubmit={handleDeposit} className="space-y-3 font-mono text-xs">
              <div>
                <label className="block text-slate-500 dark:text-slate-400 mb-1">Deposit Amount ($ USD)</label>
                <input
                  type="number"
                  min="1"
                  step="0.01"
                  required
                  value={txAmount}
                  onChange={(e) => setTxAmount(parseFloat(e.target.value) || 0)}
                  className="w-full px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/60 text-slate-900 dark:text-slate-100 focus:outline-none focus:border-emerald-500"
                />
              </div>
              <div>
                <label className="block text-slate-500 dark:text-slate-400 mb-1">Transaction Notes</label>
                <input
                  type="text"
                  placeholder="e.g. Wire transfer ref #92812"
                  value={txNotes}
                  onChange={(e) => setTxNotes(e.target.value)}
                  className="w-full px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/60 text-slate-900 dark:text-slate-100 focus:outline-none focus:border-emerald-500"
                />
              </div>
              <div className="p-2.5 rounded-lg bg-emerald-500/10 border border-emerald-500/20 text-emerald-600 dark:text-emerald-400 text-[11px]">
                Shares are minted at current portfolio Net Asset Value (NAV) without diluting existing partners.
              </div>
              <div className="flex items-center justify-end space-x-2 pt-2">
                <button
                  type="button"
                  onClick={() => setShowDepositModal(false)}
                  className="px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-800 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={isSubmitting}
                  className="px-4 py-2 rounded-lg bg-emerald-600 hover:bg-emerald-700 text-white font-semibold"
                >
                  {isSubmitting ? 'Recording...' : 'Confirm Deposit'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Modal: Record Withdrawal */}
      {showWithdrawModal && selectedInvestor && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/60 backdrop-blur-sm">
          <div className="w-full max-w-md bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl p-6 shadow-2xl space-y-4">
            <h3 className="text-sm font-bold font-mono text-slate-900 dark:text-slate-100 flex items-center space-x-2">
              <ArrowUpRight className="w-4 h-4 text-rose-500" />
              <span>Record Capital Withdrawal: {selectedInvestor.name}</span>
            </h3>
            <div className="p-3 rounded-lg bg-slate-50 dark:bg-slate-800/40 border border-slate-200 dark:border-slate-800 space-y-1 font-mono text-xs">
              <div className="flex justify-between text-slate-500">
                <span>Investor Allocated Equity:</span>
                <span className="font-bold text-slate-800 dark:text-slate-200">
                  ${(selectedInvestor.current_equity ?? selectedInvestor.total_deposited).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
                </span>
              </div>
              <div className="flex justify-between text-emerald-600 dark:text-emerald-400">
                <span>Available Tier 1 Liquid Cash:</span>
                <span className="font-bold">
                  ${(tierBreakdown?.tier1_cash ?? 15000).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
                </span>
              </div>
            </div>
            <form onSubmit={handleWithdrawal} className="space-y-3 font-mono text-xs">
              <div>
                <label className="block text-slate-500 dark:text-slate-400 mb-1">Withdrawal Amount ($ USD)</label>
                <input
                  type="number"
                  min="1"
                  max={Math.min(selectedInvestor.current_equity ?? selectedInvestor.total_deposited, tierBreakdown?.tier1_cash ?? 15000)}
                  step="0.01"
                  required
                  value={txAmount}
                  onChange={(e) => setTxAmount(parseFloat(e.target.value) || 0)}
                  className="w-full px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/60 text-slate-900 dark:text-slate-100 focus:outline-none focus:border-rose-500"
                />
              </div>
              <div>
                <label className="block text-slate-500 dark:text-slate-400 mb-1">Transaction Notes</label>
                <input
                  type="text"
                  placeholder="e.g. Liquidity redemption request"
                  value={txNotes}
                  onChange={(e) => setTxNotes(e.target.value)}
                  className="w-full px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/60 text-slate-900 dark:text-slate-100 focus:outline-none focus:border-rose-500"
                />
              </div>
              <div className="p-2.5 rounded-lg bg-amber-500/10 border border-amber-500/20 text-amber-600 dark:text-amber-400 text-[11px] flex items-center space-x-1.5">
                <ShieldAlert className="w-4 h-4 flex-shrink-0" />
                <span>Withdrawals settle strictly against Tier 1 cash to prevent forced asset liquidations or slippage.</span>
              </div>
              <div className="flex items-center justify-end space-x-2 pt-2">
                <button
                  type="button"
                  onClick={() => setShowWithdrawModal(false)}
                  className="px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-800 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={isSubmitting}
                  className="px-4 py-2 rounded-lg bg-rose-600 hover:bg-rose-700 text-white font-semibold"
                >
                  {isSubmitting ? 'Processing...' : 'Settle Withdrawal'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};
