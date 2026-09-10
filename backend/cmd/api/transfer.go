package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrWalletNotFound      = errors.New("wallet not found")
	ErrWalletInactive      = errors.New("wallet is inactive")
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrUnauthorizedWallet  = errors.New("unauthorized wallet ownership")
)

type CreateTransferRequest struct {
	ToWalletID string `json:"to_wallet_id"`
	Amount     int64  `json:"amount"`
}

type CreateTransferResponse struct {
	TransactionID   string `json:"transaction_id"`
	FromWalletID     string `json:"from_wallet_id"`
	ToWalletID       string `json:"to_wallet_id"`
	Amount          int64  `json:"amount"`
	SenderBalance   int64  `json:"sender_balance"`
	ReceiverBalance int64  `json:"receiver_balance"`
	Status          string `json:"status"`
}

type walletLockInfo struct {
	id      uuid.UUID // Updated to native uuid.UUID
	userID  string
	balance int64
	status  string
}

type RefundTransferRequest struct {
	TransactionID string `json:"transaction_id"`
}

type IdempotencyRecord struct {
	ResponseCode int
	ResponseBody []byte
}

func (app *Application) createTransferHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	// Extract authenticated user ID from context
	authUserID, ok := r.Context().Value(userIDContextKey).(string)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Native uuid.UUID from context
	fromWalletID, ok := r.Context().Value(walletIDContextKey).(uuid.UUID)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// 1. GATEKEEPING: Check for Idempotency-Key Header
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey != "" {
		if _, err := uuid.Parse(idempotencyKey); err != nil {
			writeError(w, http.StatusBadRequest, "invalid Idempotency-Key UUID format")
			return
		}

		// Check DB for existing record
		var record IdempotencyRecord
		err := app.db.QueryRow(r.Context(), `
			SELECT response_code, response_body 
			FROM idempotency_keys 
			WHERE key = $1 AND user_id = $2`,
			idempotencyKey, authUserID,
		).Scan(&record.ResponseCode, &record.ResponseBody)

		if err == nil {
			// CACHE HIT: Return previously processed response immediately
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(record.ResponseCode)
			w.Write(record.ResponseBody)
			return
		}
	}

	var req CreateTransferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.ToWalletID == "" {
		writeError(w, http.StatusBadRequest, "to_wallet_id is required")
		return
	}
	if req.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "Amount must be greater than zero")
		return
	}

	// 2. PARSE string input into native uuid.UUID
	toWalletID, err := uuid.Parse(req.ToWalletID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid to_wallet_id UUID format")
		return
	}

	// Prevent self-transfer early at boundary
	if fromWalletID == toWalletID {
		writeError(w, http.StatusBadRequest, "cannot transfer to the same wallet")
		return
	}

	// Pass typed uuid.UUID parameters into business logic
	res, err := app.createTransfer(r.Context(), req, authUserID, fromWalletID, toWalletID, idempotencyKey)
	if err != nil {
		// Map custom business errors to clear HTTP response codes
		switch {
		case errors.Is(err, ErrUnauthorizedWallet):
			writeError(w, http.StatusForbidden, "you cannot transfer from a wallet you do not own")
		case errors.Is(err, ErrInsufficientBalance):
			writeError(w, http.StatusBadRequest, "insufficient balance")
		case errors.Is(err, ErrWalletNotFound):
			writeError(w, http.StatusNotFound, "wallet not found")
		case errors.Is(err, ErrWalletInactive):
			writeError(w, http.StatusBadRequest, "one of the wallets is inactive")
		default:
			log.Printf("Transfer failed: %v", err)
			writeError(w, http.StatusInternalServerError, "Internal server error")
		}
		return
	}

	writeJSON(w, http.StatusCreated, res)
}

func (app *Application) createTransfer(
	ctx context.Context,
	req CreateTransferRequest,
	authUserID string,
	fromWalletID uuid.UUID,
	toWalletID uuid.UUID,
	idempotencyKey string,
) (*CreateTransferResponse, error) {

	if idempotencyKey != "" {
		var cachedResponseBody []byte
		err := app.db.QueryRow(ctx, `
			SELECT response_body 
			FROM idempotency_keys 
			WHERE key = $1 AND user_id = $2`,
			idempotencyKey, authUserID,
		).Scan(&cachedResponseBody)

		if err == nil {
			var cachedResponse CreateTransferResponse
			if err := json.Unmarshal(cachedResponseBody, &cachedResponse); err == nil {
				return &cachedResponse, nil
			}
		}
	}

	// 2. PREVENT SELF-TRANSFER
	if fromWalletID == toWalletID {
		return nil, fmt.Errorf("cannot transfer money to the same wallet")
	}

	tx, err := app.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 3. DETERMINISTIC LOCKING: Compare byte values of UUIDs to prevent deadlocks
	firstID, secondID := fromWalletID, toWalletID
	if firstID.String() > secondID.String() {
		firstID, secondID = secondID, firstID
	}

	// Lock the first wallet (alphabetically)
	firstWallet, err := fetchAndLockWallet(ctx, tx, firstID)
	if err != nil {
		return nil, err
	}

	// Lock the second wallet (alphabetically)
	secondWallet, err := fetchAndLockWallet(ctx, tx, secondID)
	if err != nil {
		return nil, err
	}

	// 4. Map locked records back to sender and receiver
	var sender, receiver walletLockInfo
	if fromWalletID == firstWallet.id {
		sender = firstWallet
		receiver = secondWallet
	} else {
		sender = secondWallet
		receiver = firstWallet
	}

	// 5. Ownership & Business Rule Checks
	if sender.userID != authUserID {
		return nil, ErrUnauthorizedWallet
	}
	if sender.status != "ACTIVE" || receiver.status != "ACTIVE" {
		return nil, ErrWalletInactive
	}
	if sender.balance < req.Amount {
		return nil, ErrInsufficientBalance
	}

	// 6. Update Balances
	newSenderBalance := sender.balance - req.Amount
	newReceiverBalance := receiver.balance + req.Amount

	_, err = tx.Exec(ctx, `UPDATE wallets SET balance = $1, updated_at = NOW() WHERE id = $2`, newSenderBalance, sender.id)
	if err != nil {
		return nil, fmt.Errorf("update sender balance: %w", err)
	}

	_, err = tx.Exec(ctx, `UPDATE wallets SET balance = $1, updated_at = NOW() WHERE id = $2`, newReceiverBalance, receiver.id)
	if err != nil {
		return nil, fmt.Errorf("update receiver balance: %w", err)
	}

	// 7. Write Ledger Records
	transactionID := uuid.New()
	reference := fmt.Sprintf("TRF-%d-%s", time.Now().UnixNano(), transactionID.String()[:8])

	_, err = tx.Exec(ctx, `
		INSERT INTO transactions (id, reference, type, status, amount, description)
		VALUES ($1, $2, 'TRANSFER', 'COMPLETED', $3, $4)`,
		transactionID, reference, req.Amount, "Wallet Transfer",
	)
	if err != nil {
		return nil, fmt.Errorf("insert transaction: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO ledger_entries (id, transaction_id, wallet_id, direction, amount)
		VALUES 
			($1, $2, $3, 'DEBIT', $4),
			($5, $2, $6, 'CREDIT', $4)`,
		uuid.New(), transactionID, sender.id, req.Amount,
		uuid.New(), receiver.id,
	)
	if err != nil {
		return nil, fmt.Errorf("insert ledger entries: %w", err)
	}

	res := &CreateTransferResponse{
		TransactionID:   transactionID.String(),
		FromWalletID:     sender.id.String(),
		ToWalletID:       receiver.id.String(),
		Amount:          req.Amount,
		SenderBalance:   newSenderBalance,
		ReceiverBalance: newReceiverBalance,
		Status:          "COMPLETED",
	}

	// SAVE IDEMPOTENCY KEY INSIDE TRANSACTION
	if idempotencyKey != "" {
		responseBytes, err := json.Marshal(res)
		if err != nil {
			return nil, fmt.Errorf("marshal response for idempotency: %w", err)
		}

		reqBytes, _ := json.Marshal(req)

		_, err = tx.Exec(ctx, `
		INSERT INTO idempotency_keys (key, user_id, request_path, request_params, response_code, response_body)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (key) DO NOTHING`,
			idempotencyKey, authUserID, "/transfers", reqBytes, http.StatusCreated, responseBytes,
		)
		if err != nil {
			return nil, fmt.Errorf("save idempotency key: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return res, nil
}

func (app *Application) handleRefundTransfer(w http.ResponseWriter, r *http.Request) {
	// Extract 'id' from path: /transfers/{id}/refunds
	txIDStr := r.PathValue("id")
	if txIDStr == "" {
		writeError(w, http.StatusBadRequest, "missing transaction id")
		return
	}

	txID, err := uuid.Parse(txIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid transaction id UUID format")
		return
	}

	idempotencyKey := r.Header.Get("X-Idempotency-Key")

	// Get authenticated user ID set by app.requireAuth middleware
	authUserID, ok := r.Context().Value(userIDContextKey).(string)
	if !ok || authUserID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	res, err := app.refundTransfer(r.Context(), txID, authUserID, idempotencyKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, res)
}

func (app *Application) refundTransfer(
	ctx context.Context,
	originalTxID uuid.UUID,
	authUserID string, // Original recipient initiating the refund
	idempotencyKey string,
) (*CreateTransferResponse, error) {

	// 0. IDEMPOTENCY LOOKUP CHECK
	if idempotencyKey != "" {
		var cachedResponseBody []byte
		err := app.db.QueryRow(ctx, `
			SELECT response_body FROM idempotency_keys 
			WHERE key = $1 AND user_id = $2`,
			idempotencyKey, authUserID,
		).Scan(&cachedResponseBody)

		if err == nil {
			var cachedResponse CreateTransferResponse
			if err := json.Unmarshal(cachedResponseBody, &cachedResponse); err == nil {
				return &cachedResponse, nil
			}
		}
	}

	tx, err := app.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. FETCH & GUARD ORIGINAL TRANSACTION
	var origAmount int64
	var origStatus string
	err = tx.QueryRow(ctx, `
		SELECT amount, status FROM transactions 
		WHERE id = $1 FOR UPDATE`, originalTxID,
	).Scan(&origAmount, &origStatus)

	if err != nil {
		return nil, ErrWalletNotFound // Or ErrTransactionNotFound
	}
	if origStatus == "REVERSED" {
		return nil, fmt.Errorf("transaction has already been refunded")
	}
	if origStatus != "COMPLETED" {
		return nil, fmt.Errorf("only completed transactions can be refunded")
	}

	// 2. FETCH ORIGINAL LEDGER ENTRIES TO IDENTIFY WALLETS
	var origSenderWalletID, origReceiverWalletID uuid.UUID
	rows, err := tx.Query(ctx, `
		SELECT wallet_id, direction FROM ledger_entries 
		WHERE transaction_id = $1`, originalTxID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var wID uuid.UUID
		var dir string
		if err := rows.Scan(&wID, &dir); err != nil {
			return nil, err
		}
		if dir == "DEBIT" {
			origSenderWalletID = wID
		} else if dir == "CREDIT" {
			origReceiverWalletID = wID
		}
	}

	// 3. DETERMINISTIC LOCKING (Sort Wallet UUIDs to prevent deadlocks)
	firstID, secondID := origSenderWalletID, origReceiverWalletID
	if firstID.String() > secondID.String() {
		firstID, secondID = secondID, firstID
	}

	w1, err := fetchAndLockWallet(ctx, tx, firstID)
	if err != nil {
		return nil, err
	}
	w2, err := fetchAndLockWallet(ctx, tx, secondID)
	if err != nil {
		return nil, err
	}

	var origSender, origReceiver walletLockInfo
	if origSenderWalletID == w1.id {
		origSender, origReceiver = w1, w2
	} else {
		origSender, origReceiver = w2, w1
	}

	// 4. VERIFY REFUND INITIATOR (Must be original receiver)
	if origReceiver.userID != authUserID {
		return nil, ErrUnauthorizedWallet
	}
	if origReceiver.balance < origAmount {
		return nil, ErrInsufficientBalance
	}

	// 5. UPDATE BALANCES (REVERSE DIRECTION)
	newReceiverBalance := origReceiver.balance - origAmount // Debited
	newSenderBalance := origSender.balance + origAmount     // Credited

	_, err = tx.Exec(ctx, `UPDATE wallets SET balance = $1, updated_at = NOW() WHERE id = $2`, newReceiverBalance, origReceiver.id)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `UPDATE wallets SET balance = $1, updated_at = NOW() WHERE id = $2`, newSenderBalance, origSender.id)
	if err != nil {
		return nil, err
	}

	// 6. MARK ORIGINAL TRANSACTION AS REVERSED
	_, err = tx.Exec(ctx, `UPDATE transactions SET status = 'REVERSED', updated_at = NOW() WHERE id = $1`, originalTxID)
	if err != nil {
		return nil, err
	}

	// 7. CREATE NEW REFUND TRANSACTION & LEDGERS
	refundTxID := uuid.New()
	ref := fmt.Sprintf("RFD-%d-%s", time.Now().UnixNano(), refundTxID.String()[:8])

	_, err = tx.Exec(ctx, `
		INSERT INTO transactions (id, reference, type, status, amount, description)
		VALUES ($1, $2, 'REFUND', 'COMPLETED', $3, $4)`,
		refundTxID, ref, origAmount, fmt.Sprintf("Refund for transaction %s", originalTxID.String()),
	)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO ledger_entries (id, transaction_id, wallet_id, direction, amount)
		VALUES 
			($1, $2, $3, 'DEBIT', $4),
			($5, $6, $7, 'CREDIT', $8)`,
		uuid.New(), refundTxID, origReceiver.id, origAmount,
		uuid.New(), refundTxID, origSender.id, origAmount,
	)
	if err != nil {
		return nil, err
	}

	res := &CreateTransferResponse{
		TransactionID:   refundTxID.String(),
		FromWalletID:     origReceiver.id.String(),
		ToWalletID:       origSender.id.String(),
		Amount:          origAmount,
		SenderBalance:   newReceiverBalance,
		ReceiverBalance: newSenderBalance,
		Status:          "COMPLETED",
	}

	// 8. SAVE IDEMPOTENCY KEY
	if idempotencyKey != "" {
		respBytes, _ := json.Marshal(res)
		_, _ = tx.Exec(ctx, `
			INSERT INTO idempotency_keys (key, user_id, request_path, request_params, response_code, response_body)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (key) DO NOTHING`,
			idempotencyKey, authUserID, fmt.Sprintf("/transfers/%s/refund", originalTxID.String()), []byte("{}"), 200, respBytes,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return res, nil
}

// Helper function to query and lock a wallet row
func fetchAndLockWallet(ctx context.Context, tx pgx.Tx, walletID uuid.UUID) (walletLockInfo, error) {
	var w walletLockInfo
	err := tx.QueryRow(ctx, `
		SELECT id, user_id, balance, status 
		FROM wallets 
		WHERE id = $1 FOR UPDATE`,
		walletID,
	).Scan(&w.id, &w.userID, &w.balance, &w.status)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return w, ErrWalletNotFound
		}
		return w, fmt.Errorf("fetch wallet %s: %w", walletID, err)
	}
	return w, nil
}