package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// Request & Response Structs
type RegisterRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string             `json:"token"`
	User  CreateUserResponse `json:"user"`
}

var ErrInvalidCredentials = errors.New("invalid email or password")

// Read JWT secret from environment or default for local development
func getJWTSecret() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return []byte("super-secret-dev-key-do-not-use-in-prod")
	}
	return []byte(secret)
}

// 1. Authenticate Business Logic
func (app *Application) authenticateUser(ctx context.Context, email, password string) (string, CreateUserResponse, error) {
	var user CreateUserResponse
	var hashedPassword string

	// Fetch user details and hashed password from DB
	err := app.db.QueryRow(ctx, `
		SELECT u.id, u.name, u.email, u.password_hash, w.id
		FROM users u
		JOIN wallets w ON w.user_id = u.id
		WHERE u.email = $1`,
		email,
	).Scan(&user.ID, &user.Name, &user.Email, &hashedPassword, &user.WalletID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", CreateUserResponse{}, ErrInvalidCredentials
		}
		return "", CreateUserResponse{}, fmt.Errorf("fetch user: %w", err)
	}

	// Compare incoming plain-text password against stored bcrypt hash
	err = bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	if err != nil {
		return "", CreateUserResponse{}, ErrInvalidCredentials
	}

	// Generate JWT Token (valid for 24 hours)
	claims := jwt.MapClaims{
		"sub": user.ID,
		"wallet_id": user.WalletID,
		"exp": time.Now().Add(24 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(getJWTSecret())
	if err != nil {
		return "", CreateUserResponse{}, fmt.Errorf("sign token: %w", err)
	}

	return signedToken, user, nil
}

// 2. Registration Handler
func (app *Application) registerHandler(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	if req.Name == "" || req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "name, email, and password are required")
		return
	}

	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters long")
		return
	}

	hashedPasswordBytes, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		log.Printf("hash password: %v", err)
		writeError(w, http.StatusInternalServerError, "could not process request")
		return
	}

	user, err := app.createUser(
		r.Context(),
		req.Name,
		req.Email,
		string(hashedPasswordBytes),
	)
	if err != nil {
		log.Printf("create user: %v", err)
		writeError(w, http.StatusInternalServerError, "could not create user")
		return
	}

	// Generate JWT immediately upon successful registration
	claims := jwt.MapClaims{
		"sub": user.ID,
		"wallet_id": user.WalletID,
		"exp": time.Now().Add(24 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(getJWTSecret())
	if err != nil {
		log.Printf("sign token: %v", err)
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}

	writeJSON(w, http.StatusCreated, AuthResponse{
		Token: signedToken,
		User:  user,
	})
}

// 3. Login Handler
func (app *Application) loginHandler(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	token, user, err := app.authenticateUser(r.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		log.Printf("login error: %v", err)
		writeError(w, http.StatusInternalServerError, "could not process login")
		return
	}

	writeJSON(w, http.StatusOK, AuthResponse{
		Token: token,
		User:  user,
	})
}