package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"os"

	"github.com/google/uuid"
	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const (
	userIDContextKey contextKey = "userID"
	walletIDContextKey contextKey = "walletID"
)

func (app *Application) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeError(w, http.StatusUnauthorized, "authorization header required")
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			writeError(w, http.StatusUnauthorized, "invalid authorization header format")
			return
		}

		tokenString := parts[1]

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return getJWTSecret(), nil
		})

		if err != nil || !token.Valid {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			writeError(w, http.StatusUnauthorized, "invalid token claims")
			return
		}

		userID, ok := claims["sub"].(string)
		if !ok {
			writeError(w, http.StatusUnauthorized, "invalid user ID in token")
			return
		}

		walletIDStr, ok := claims["wallet_id"].(string)
		if !ok {
			writeError(w, http.StatusUnauthorized, "invalid wallet ID in token")
			return
		}

		walletID, err := uuid.Parse(walletIDStr)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "malformed wallet ID in token")
			return
		}

		// Save both userID and walletID into context
		ctx := context.WithValue(r.Context(), userIDContextKey, userID)
		ctx = context.WithValue(ctx, walletIDContextKey, walletID)

		next(w, r.WithContext(ctx))
	}
}

func (app *Application) enableCORS(next http.Handler) http.Handler {
	allowedOrigin := os.Getenv("ALLOWED_ORIGIN")
	if allowedOrigin == "" {
		allowedOrigin = "http://localhost:3000"
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Idempotency-Key")

		// Handle preflight OPTIONS request
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}