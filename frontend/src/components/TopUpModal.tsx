"use client";

import { useState } from "react";
import { useAuth } from "../context/AuthContext";

interface TopUpModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: () => void; // Triggered to refresh dashboard wallet balance
}

export default function TopUpModal({
  isOpen,
  onClose,
  onSuccess,
}: TopUpModalProps) {
  const { user, token } = useAuth();
  const [amount, setAmount] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState("");

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");

    const parsedAmount = parseFloat(amount);
    if (isNaN(parsedAmount) || parsedAmount < 10000) {
      setError("Minimum top-up amount is IDR 10,000.");
      return;
    }

    if (!token || !user?.wallet_id) {
      setError("Authentication or wallet information missing.");
      return;
    }

    try {
      setIsSubmitting(true);

      const res = await fetch(
        `http://localhost:8080/wallets/${user.wallet_id}/topups`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${token}`,
          },
          body: JSON.stringify({ amount: parsedAmount }),
        },
      );

      const data = await res.json();
      if (!res.ok) throw new Error(data.error || "Failed to initiate top-up");

      // Check if Snap script is available
      if (typeof window !== "undefined" && window.snap) {
        window.snap.pay(data.snap_token, {
          onSuccess: (result: any) => {
            setAmount("");
            setIsSubmitting(false);
            onSuccess();
            onClose();
          },
          onPending: (result: any) => {
            setIsSubmitting(false);
            onClose();
          },
          onError: (result: any) => {
            setError("Payment failed.");
            setIsSubmitting(false);
          },
          onClose: () => {
            setIsSubmitting(false); // Reset button when user manually closes Snap iframe
          },
        });
      } else {
        // Fallback: If snap.js fails to load in browser, open redirect_url in a new tab
        setIsSubmitting(false);
        window.open(data.snap_url, "_blank");
      }
    } catch (err: any) {
      setError(err.message || "Failed to process top-up");
      setIsSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4">
      <div className="w-full max-w-md rounded-2xl bg-slate-900 border border-slate-800 p-6 shadow-2xl space-y-5">
        {/* Modal Header */}
        <div className="flex justify-between items-center border-b border-slate-800 pb-3">
          <h2 className="text-lg font-bold text-white">Top Up Wallet</h2>
          <button
            onClick={onClose}
            className="text-slate-400 hover:text-white transition text-sm font-semibold"
          >
            ✕
          </button>
        </div>

        {/* Feedback Messages */}
        {error && (
          <div className="p-3 bg-rose-500/10 border border-rose-500/20 text-rose-400 text-xs rounded-lg">
            {error}
          </div>
        )}

        {/* Top-Up Form */}
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-xs font-semibold uppercase text-slate-400 mb-1">
              Amount (IDR)
            </label>
            <input
              type="number"
              required
              min="10000"
              placeholder="e.g. 50000"
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              className="w-full rounded-lg bg-slate-950 border border-slate-800 px-3 py-2 text-sm text-white placeholder-slate-600 focus:border-blue-500 focus:outline-none"
            />
          </div>

          <div className="flex gap-3 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="w-1/2 py-2.5 bg-slate-800 hover:bg-slate-700 text-slate-300 text-sm font-medium rounded-lg transition"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={isSubmitting}
              className="w-1/2 py-2.5 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white text-sm font-medium rounded-lg transition"
            >
              {isSubmitting ? "Processing..." : "Pay Now"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
