package main

import (
	"context"
	"log"
	"net/http"
	"github.com/google/uuid"
)

type CreateUserResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	WalletID string `json:"wallet_id"`
}

type UserWithWallet struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	WalletID string `json:"wallet_id"`
	Balance  int64  `json:"balance"`
	Currency string `json:"currency"`
}


func (app *Application) createUser(
	ctx context.Context,
	name string,
	email string,
	password string,
) (CreateUserResponse, error) {
	tx, err := app.db.Begin(ctx)
	if err != nil {
		return CreateUserResponse{}, err
	}
	defer tx.Rollback(ctx)

	userID := uuid.New()
	walletID := uuid.New()

	_, err = tx.Exec(
		ctx,
		`
            INSERT INTO users (
                id,
                name,
                email,
                password_hash
            )
            VALUES ($1, $2, $3, $4)
        `,
		userID,
		name,
		email,
		password,
	)
	if err != nil {
		return CreateUserResponse{}, err
	}

	_, err = tx.Exec(
		ctx,
		`
            INSERT INTO wallets (
                id,
                user_id,
                balance,
                currency,
                status
            )
            VALUES ($1, $2, 0, 'IDR', 'ACTIVE')
        `,
		walletID,
		userID,
	)
	if err != nil {
		return CreateUserResponse{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return CreateUserResponse{}, err
	}

	return CreateUserResponse{
		ID:       userID.String(),
		Name:     name,
		Email:    email,
		WalletID: walletID.String(),
	}, nil
}

func (app *Application) getUsersHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	rows, err := app.db.Query(
		r.Context(),
		`
			SELECT
				u.id,
				u.name,
				u.email,
				w.id,
				w.balance,
				w.currency
			FROM users u
			JOIN wallets w ON w.user_id = u.id
			ORDER BY u.created_at DESC
		`,
	)
	if err != nil {
		log.Printf("get users: %v", err)
		writeError(
			w,
			http.StatusInternalServerError,
			"could not get users",
		)
		return
	}
	defer rows.Close()

	users := make([]UserWithWallet, 0)

	for rows.Next() {
		var user UserWithWallet

		err := rows.Scan(
			&user.ID,
			&user.Name,
			&user.Email,
			&user.WalletID,
			&user.Balance,
			&user.Currency,
		)
		if err != nil {
			log.Printf("scan user: %v", err)
			writeError(
				w,
				http.StatusInternalServerError,
				"could not read user",
			)
			return
		}

		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		log.Printf("iterate users: %v", err)
		writeError(
			w,
			http.StatusInternalServerError,
			"could not read users",
		)
		return
	}

	writeJSON(w, http.StatusOK, users)
}
