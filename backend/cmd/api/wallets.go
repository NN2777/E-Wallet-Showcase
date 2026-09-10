package main

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type WalletResponse struct {
	ID       string `json:"id"`
	UserID   string `json:"user_id"`
	Owner    string `json:"owner"`
	Balance  int64  `json:"balance"`
	Currency string `json:"currency"`
	Status   string `json:"status"`
}

func (app *Application) getWalletHandler(w http.ResponseWriter, r *http.Request) {
	// Extract walletID directly from Context (populated by requireAuth middleware)
	walletID, ok := r.Context().Value(walletIDContextKey).(uuid.UUID)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	wallet, err := app.getWalletByID(r.Context(), walletID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "wallet not found")
			return
		}
		log.Printf("get wallet error: %v", err)
		writeError(w, http.StatusInternalServerError, "could not get wallet")
		return
	}

	writeJSON(w, http.StatusOK, wallet)
}

func (app *Application) getWalletByID(ctx context.Context, walletID uuid.UUID) (WalletResponse, error) {
	var wallet WalletResponse

	query := `
		SELECT
			w.id,
			w.user_id,
			u.name,
			w.balance,
			w.currency,
			w.status
		FROM wallets w
		JOIN users u ON u.id = w.user_id
		WHERE w.id = $1`

	err := app.db.QueryRow(ctx, query, walletID).Scan(
		&wallet.ID,
		&wallet.UserID,
		&wallet.Owner,
		&wallet.Balance,
		&wallet.Currency,
		&wallet.Status,
	)

	return wallet, err
}