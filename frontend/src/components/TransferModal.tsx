"use client";

import { useState } from "react";
import { useAuth } from "../context/AuthContext";
import { transferFunds } from "../lib/api";

interface TransferModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: () => void; // Triggered to refresh dashboard wallet balance
}

export default function TransferModal({ isOpen, onClose, onSuccess }: TransferModalProps) {
  const { token } = useAuth();
  const [recipientID, setRecipientID] = useState("");
  const [amount, setAmount] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState("");
  const [successMsg, setSuccessMsg] = useState("");

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setSuccessMsg("");

    const parsedAmount = parseFloat(amount);
    if (isNaN(parsedAmount) || parsedAmount <= 0) {
      setError("Please enter a valid transfer amount.");
      return;
    }

    if (!token) {
      setError("Authentication token missing.");
      return;
    }

    try {
      setIsSubmitting(true);
      await transferFunds(recipientID, parsedAmount, token);
      
      setSuccessMsg("Transfer successful!");
      setRecipientID("");
      setAmount("");
      
      // Notify parent page to reload balance
      onSuccess();
      
      // Automatically close modal after 1.5 seconds
      setTimeout(() => {
        setSuccessMsg("");
        onClose();
      }, 1500);
    } catch (err: any) {
      setError(err.message || "Failed to execute transfer");
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4">
      <div className="w-full max-w-md rounded-2xl bg-slate-900 border border-slate-800 p-6 shadow-2xl space-y-5">
        
        {/* Modal Header */}
        <div className="flex justify-between items-center border-b border-slate-800 pb-3">
          <h2 className="text-lg font-bold text-white">Transfer Funds</h2>
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
        {successMsg && (
          <div className="p-3 bg-emerald-500/10 border border-emerald-500/20 text-emerald-400 text-xs rounded-lg">
            {successMsg}
          </div>
        )}

        {/* Transfer Form */}
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-xs font-semibold uppercase text-slate-400 mb-1">
              Recipient Wallet ID
            </label>
            <input
              type="text"
              required
              placeholder="e.g. 7ccea464-af79-499e-b969-587ffb16f766"
              value={recipientID}
              onChange={(e) => setRecipientID(e.target.value)}
              className="w-full rounded-lg bg-slate-950 border border-slate-800 px-3 py-2 text-sm text-white placeholder-slate-600 focus:border-blue-500 focus:outline-none"
            />
          </div>

          <div>
            <label className="block text-xs font-semibold uppercase text-slate-400 mb-1">
              Amount (IDR)
            </label>
            <input
              type="number"
              required
              min="1"
              placeholder="50000"
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
              {isSubmitting ? "Processing..." : "Send Money"}
            </button>
          </div>
        </form>

      </div>
    </div>
  );
}