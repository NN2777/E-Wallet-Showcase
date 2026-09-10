package main

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/google/uuid"
	"github.com/midtrans/midtrans-go"
	"github.com/midtrans/midtrans-go/snap"
)

type CreateTopUpRequest struct {
	Amount int64 `json:"amount"`
}

type CreateTopUpResponse struct {
	TransactionID string `json:"transaction_id"`
	SnapToken     string `json:"snap_token"`
	SnapURL       string `json:"snap_url"`
}

// 1. INITIATION: Called by frontend via JWT auth
func (app *Application) createTopUpHandler(w http.ResponseWriter, r *http.Request) {
	_, ok := r.Context().Value(userIDContextKey).(string)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Extract walletID directly from context (stored as uuid.UUID in middleware)
	walletID, ok := r.Context().Value(walletIDContextKey).(uuid.UUID)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var request CreateTopUpRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if request.Amount < 10000 {
		writeError(w, http.StatusBadRequest, "minimum top-up amount is 10000")
		return
	}

	// We no longer need to execute a SELECT query to find the wallet ID here.
	result, err := app.initiateTopUp(r.Context(), walletID, request.Amount)
	if err != nil {
		log.Printf("initiate top-up error: %v", err)
		writeError(w, http.StatusInternalServerError, "could not initiate top-up")
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

func (app *Application) initiateTopUp(
	ctx context.Context,
	walletID uuid.UUID,
	amount int64,
) (CreateTopUpResponse, error) {
	tx, err := app.db.Begin(ctx)
	if err != nil {
		return CreateTopUpResponse{}, err
	}
	defer tx.Rollback(ctx)

	transactionID := uuid.New()
	reference := "TOPUP-" + transactionID.String()[:8]

	// Create PENDING transaction
	_, err = tx.Exec(ctx, `
		INSERT INTO transactions (id, reference, type, status, amount)
		VALUES ($1, $2, 'TOP_UP', 'PENDING', $3)`,
		transactionID, reference, amount,
	)
	if err != nil {
		return CreateTopUpResponse{}, err
	}

	// Record pending ledger entry linked to the wallet
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger_entries (id, transaction_id, wallet_id, direction, amount)
		VALUES ($1, $2, $3, 'CREDIT', $4)`,
		uuid.New(), transactionID, walletID, amount,
	)
	if err != nil {
		return CreateTopUpResponse{}, err
	}

	// Request Midtrans Snap payment token
	var s snap.Client
	s.New(os.Getenv("MIDTRANS_SERVER_KEY"), midtrans.Sandbox)

	snapResp, snapErr := s.CreateTransaction(&snap.Request{
		TransactionDetails: midtrans.TransactionDetails{
			OrderID:  reference,
			GrossAmt: amount,
		},
	})
	if snapErr != nil {
		return CreateTopUpResponse{}, snapErr
	}

	if err := tx.Commit(ctx); err != nil {
		return CreateTopUpResponse{}, err
	}

	return CreateTopUpResponse{
		TransactionID: transactionID.String(),
		SnapToken:     snapResp.Token,
		SnapURL:       snapResp.RedirectURL,
	}, nil
}

// 2. SETTLEMENT: Public Webhook called by Midtrans
func (app *Application) midtransWebhookHandler(w http.ResponseWriter, r *http.Request) {
	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	orderID, _ := payload["order_id"].(string)
	statusCode, _ := payload["status_code"].(string)
	transactionStatus, _ := payload["transaction_status"].(string)
	fraudStatus, _ := payload["fraud_status"].(string)
	signatureKey, _ := payload["signature_key"].(string)

	// 1. Ignore Midtrans Dashboard test button pings (dummy order IDs)
	if orderID == "" || signatureKey == "" || orderID == "12345" || orderID == "payment-12345" {
		log.Println("[Webhook] Received Midtrans test ping successfully.")
		w.WriteHeader(http.StatusOK)
		return
	}

	// 2. Handle gross_amount whether string or float64
	var grossAmountStr string
	switch v := payload["gross_amount"].(type) {
	case string:
		grossAmountStr = v
	case float64:
		grossAmountStr = fmt.Sprintf("%.2f", v)
	}

	// 3. Validate Signature
	serverKey := os.Getenv("MIDTRANS_SERVER_KEY")
	hasher := sha512.New()
	hasher.Write([]byte(orderID + statusCode + grossAmountStr + serverKey))
	expectedSignature := hex.EncodeToString(hasher.Sum(nil))

	if signatureKey != expectedSignature {
		log.Printf("[Webhook Error] Signature mismatch. Received: %s, Expected: %s", signatureKey, expectedSignature)
		writeError(w, http.StatusUnauthorized, "invalid signature")
		return
	}

	// 4. Process Settlement
	if transactionStatus == "settlement" || (transactionStatus == "capture" && fraudStatus == "accept") {
		if err := app.fulfillTopUp(r.Context(), orderID); err != nil {
			log.Printf("Fulfill topup error: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to fulfill topup")
			return
		}
		log.Printf("[Success] Wallet top-up settled for Order: %s", orderID)
	}

	w.WriteHeader(http.StatusOK)
}

func (app *Application) fulfillTopUp(ctx context.Context, reference string) error {
	tx, err := app.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var transactionID uuid.UUID
	var status string
	var amount int64

	// Lock transaction row and check status
	err = tx.QueryRow(ctx, `
		SELECT id, status, amount FROM transactions 
		WHERE reference = $1 FOR UPDATE`, reference,
	).Scan(&transactionID, &status, &amount)
	if err != nil {
		return err
	}

	if status == "COMPLETED" {
		return nil // Idempotency check: avoid crediting balance twice
	}

	// Fetch target wallet ID from ledger
	var walletID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT wallet_id FROM ledger_entries 
		WHERE transaction_id = $1 AND direction = 'CREDIT'`, transactionID,
	).Scan(&walletID)
	if err != nil {
		return err
	}

	// Credit wallet balance
	_, err = tx.Exec(ctx, `
		UPDATE wallets SET balance = balance + $1 
		WHERE id = $2 AND status = 'ACTIVE'`, amount, walletID,
	)
	if err != nil {
		return err
	}

	// Mark transaction as COMPLETED
	_, err = tx.Exec(ctx, `
		UPDATE transactions SET status = 'COMPLETED' 
		WHERE id = $1`, transactionID,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}