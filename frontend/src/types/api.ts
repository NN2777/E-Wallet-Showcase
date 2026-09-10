export interface User {
  id: string;
  email: string;
  name: string;
  wallet_id: string;
  created_at: string;
}

export interface Wallet {
  id: string;
  user_id: string;
  balance: number; // Stored in cents/smallest currency unit
  created_at: string;
}

export interface TransferRequest {
  from_wallet_id: string;
  to_wallet_id: string;
  amount: number;
}

export interface TransferResponse {
  transaction_id: string;
  from_wallet_id: string;
  to_wallet_id: string;
  amount: number;
  sender_balance: number;
  receiver_balance: number;
  status: string;
}

export interface ApiError {
  error: string;
}