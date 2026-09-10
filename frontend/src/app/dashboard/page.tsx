// src/app/dashboard/page.tsx
"use client";

import { useEffect, useState, useCallback } from "react";
import { useAuth } from "../../context/AuthContext";
import { Wallet } from "../../types/api";
import { getWallet } from "../../lib/api";
import TransferModal from "../../components/TransferModal";
import TopUpModal from "../../components/TopUpModal";

export default function DashboardPage() {
  const { user, token, logout } = useAuth();
  const [wallet, setWallet] = useState<Wallet | null>(null);
  const [showBalance, setShowBalance] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [isTransferOpen, setIsTransferOpen] = useState(false);
  const [isTopUpOpen, setIsTopUpOpen] = useState(false);

  // Helper function to re-fetch latest wallet balance
  const fetchWallet = useCallback(() => {
    if (!token || !user?.wallet_id) return;

    getWallet(user.wallet_id, token)
      .then((data) => setWallet(data))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [user, token]);

  useEffect(() => {
    fetchWallet();
  }, [fetchWallet]);

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-slate-900 text-white">
        <p className="animate-pulse">Loading dashboard...</p>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100 p-6 font-sans">
      <div className="max-w-4xl mx-auto space-y-6">
        {/* Header Section */}
        <header className="flex justify-between items-center border-b border-slate-800 pb-4">
          <div>
            <h1 className="text-2xl font-bold text-white">Overview</h1>
            <p className="text-sm text-slate-400">
              Welcome back, {user?.name || "User"}
            </p>
          </div>

          <div className="flex items-center gap-3">
            <span className="px-3 py-1 bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 text-xs rounded-full font-medium">
              System Active
            </span>
            <button
              onClick={logout}
              className="px-3 py-1 bg-rose-500/10 hover:bg-rose-500/20 text-rose-400 border border-rose-500/20 text-xs rounded-lg font-medium transition cursor-pointer"
            >
              Logout
            </button>
          </div>
        </header>

        {/* Balance Card */}
        <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 shadow-xl space-y-4">
          <div className="flex justify-between items-center">
            <span className="text-xs uppercase tracking-wider text-slate-400 font-semibold">
              Total Balance
            </span>
            <button
              onClick={() => setShowBalance(!showBalance)}
              className="text-xs text-blue-400 hover:text-blue-300 transition"
            >
              {showBalance ? "Hide" : "Show"}
            </button>
          </div>

          <div className="text-3xl font-extrabold tracking-tight text-white">
            {showBalance
              ? `Rp ${wallet?.balance?.toLocaleString("id-ID") || "0"}`
              : "Rp ••••••••"}
          </div>

          {/* Action Grid */}
          <div className="grid grid-cols-2 gap-4 pt-2">
            <button
              onClick={() => setIsTopUpOpen(true)}
              className="w-full bg-blue-600 hover:bg-blue-500 text-white font-medium py-3 rounded-xl transition"
            >
              + Top Up
            </button>
            <button
              onClick={() => setIsTransferOpen(true)}
              className="w-full py-2.5 px-4 bg-slate-800 hover:bg-slate-700 text-slate-200 text-sm font-medium rounded-lg border border-slate-700 transition cursor-pointer"
            >
              Transfer
            </button>
          </div>
        </div>

        {error && (
          <div className="p-4 bg-rose-500/10 border border-rose-500/20 text-rose-400 text-sm rounded-lg">
            {error}
          </div>
        )}
      </div>

      {/* Transfer Modal Instance */}
      <TransferModal
        isOpen={isTransferOpen}
        onClose={() => setIsTransferOpen(false)}
        onSuccess={fetchWallet}
      />

      <TopUpModal
        isOpen={isTopUpOpen}
        onClose={() => setIsTopUpOpen(false)}
        onSuccess={fetchWallet}
      />
    </div>
  );
}
