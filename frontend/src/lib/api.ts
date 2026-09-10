import { Wallet } from "../types/api";
import { TransferResponse } from "../types/api";

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

export async function apiFetch<T>(
  endpoint: string,
  options: RequestInit & { idempotencyKey?: string; token?: string } = {}
): Promise<T> {
  const { idempotencyKey, token, headers, ...customConfig } = options;

  const defaultHeaders: Record<string, string> = {
    "Content-Type": "application/json",
  };

  if (token) {
    defaultHeaders["Authorization"] = `Bearer ${token}`;
  }

  if (idempotencyKey) {
    defaultHeaders["X-Idempotency-Key"] = idempotencyKey;
  }

  const config: RequestInit = {
    method: customConfig.method || "GET",
    headers: {
      ...defaultHeaders,
      ...headers,
    },
    ...customConfig,
  };

  const response = await fetch(`${API_BASE_URL}${endpoint}`, config);
  const data = await response.json();

  if (!response.ok) {
    throw new Error(data.error || "An unexpected error occurred");
  }

  return data as T;
}

export async function getWallet(walletID: string, token: string): Promise<Wallet> {
  return apiFetch<Wallet>(`/wallets/${walletID}`, {
    token,
  });
}

export async function transferFunds(
  recipientWalletID: string,
  amount: number,
  token: string
): Promise<TransferResponse> {
  // Generate a unique idempotency key for this request
  const idempotencyKey = crypto.randomUUID();

  return apiFetch<TransferResponse>("/transfers", {
    method: "POST",
    token,
    idempotencyKey,
    body: JSON.stringify({
      to_wallet_id: recipientWalletID,
      amount: amount,
    }),
  });
}