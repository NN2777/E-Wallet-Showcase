package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

// Scenario 1: 100 Concurrent Requests attempting to spend Rp80.000 from a Rp100.000 balance
func TestConcurrentSpending_Scenario1(t *testing.T) {
	app := setupTestApp(t) // Helper to initialize test DB connection
	ctx := context.Background()

	// 1. Setup Test Wallets (Sender: 100,000, Receiver: 0) returning uuid.UUID types
	senderWalletID, receiverWalletID, ownerUserID := seedTestWallets(t, app, 100000, 0)

	req := CreateTransferRequest{
		ToWalletID: receiverWalletID.String(), // Payload DTO expects string representation
		Amount:     80000,
	}

	concurrentRequests := 100
	var wg sync.WaitGroup
	var successCount int64
	var failureCount int64

	// 2. Fire 100 goroutines simultaneously
	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Unique idempotency key per distinct request
			idempotencyKey := uuid.New().String()

			// Updated createTransfer accepting native uuid.UUID for fromWalletID and toWalletID
			_, err := app.createTransfer(ctx, req, ownerUserID, senderWalletID, receiverWalletID, idempotencyKey)
			if err == nil {
				atomic.AddInt64(&successCount, 1)
			} else {
				atomic.AddInt64(&failureCount, 1)
			}
		}()
	}

	wg.Wait()

	// 3. Assertions
	if successCount != 1 {
		t.Fatalf("Expected exactly 1 success, got %d", successCount)
	}
	if failureCount != 99 {
		t.Fatalf("Expected 99 failures due to balance limit, got %d", failureCount)
	}

	// 4. Verify Final Balance in DB using native uuid.UUID
	finalSenderBalance := getWalletBalance(t, app, senderWalletID)
	if finalSenderBalance != 20000 {
		t.Fatalf("Expected final sender balance of 20000, got %d", finalSenderBalance)
	}
}
// Scenario 2: Duplicate Request (Idempotency Test)
func TestDuplicateRequest_Scenario2(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()

	senderID, receiverID, ownerUserID := seedTestWallets(t, app, 100000, 0)

	req := CreateTransferRequest{
		ToWalletID:   receiverID.String(),
		Amount:       30000,
	}

	fixedIdempotencyKey := uuid.New().String()
	var firstTxID string

	// Send the exact same request 10 times
	for i := 0; i < 10; i++ {
		res, err := app.createTransfer(ctx, req, ownerUserID, senderID, receiverID, fixedIdempotencyKey)
		if err != nil {
			t.Fatalf("Iteration %d failed unexpectedly: %v", i+1, err)
		}

		if i == 0 {
			firstTxID = res.TransactionID
		} else {
			// Ensure response transaction_id matches the first execution
			if res.TransactionID != firstTxID {
				t.Fatalf("Iteration %d created a new transaction ID instead of returning cached response", i+1)
			}
		}
	}

	// Sender should be debited 30,000 ONCE (100,000 - 30,000 = 70,000)
	finalSenderBalance := getWalletBalance(t, app, senderID)
	if finalSenderBalance != 70000 {
		t.Fatalf("Expected sender balance of 70000, got %d (debited multiple times)", finalSenderBalance)
	}
}

// Scenario 3: Database Failure / Transactional Rollback Test
func TestDatabaseFailure_Scenario3(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()

	// Seed sender with 100,000, receiver with 0
	senderID, _, ownerUserID := seedTestWallets(t, app, 100000, 0)

	// Deliberately use a fake/non-existent UUID for ToWalletID
	fakeReceiverID := uuid.New().String()

	req := CreateTransferRequest{
		ToWalletID:   fakeReceiverID, // This will trigger a failure
		Amount:       50000,
	}

	// Attempt the transfer (expecting an error)
	_, err := app.createTransfer(ctx, req, ownerUserID, senderID, uuid.New(), uuid.New().String())
	if err == nil {
		t.Fatalf("Expected transfer to fail due to non-existent receiver wallet, but it succeeded")
	}

	// ASSERTION 1: Verify sender balance is completely untouched (Rollback check)
	finalSenderBalance := getWalletBalance(t, app, senderID)
	if finalSenderBalance != 100000 {
		t.Fatalf("Transaction failed to roll back! Expected sender balance of 100000, got %d", finalSenderBalance)
	}

	// ASSERTION 2: Ensure no partial transactions or ledger entries were created
	var count int
	err = app.db.QueryRow(ctx, `SELECT COUNT(*) FROM ledger_entries WHERE wallet_id = $1`, senderID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query ledger entries: %v", err)
	}
	if count != 0 {
		t.Fatalf("Expected 0 ledger entries after rollback, found %d", count)
	}
}

// Scenario 4: Refund Feature Test
func TestRefund_Scenario4(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()

	// Seed Sender with 100,000, Receiver with 0
	senderWalletID, receiverWalletID, senderUserID := seedTestWallets(t, app, 100000, 0)

	// Step 1: Initial Payment of 25,000
	transferReq := CreateTransferRequest{
		ToWalletID: receiverWalletID.String(),
		Amount:     25000,
	}

	idempotencyKey := uuid.New().String()

	// Fixed: Correct argument order (senderWalletID, receiverWalletID)
	transferRes, err := app.createTransfer(ctx, transferReq, senderUserID, senderWalletID, receiverWalletID, idempotencyKey)
	if err != nil {
		t.Fatalf("Initial transfer failed: %v", err)
	}

	// Get Receiver's UserID to authorize the refund
	var receiverUserID string
	err = app.db.QueryRow(ctx, `SELECT user_id FROM wallets WHERE id = $1`, receiverWalletID).Scan(&receiverUserID)
	if err != nil {
		t.Fatalf("Failed to fetch receiver user ID: %v", err)
	}

	// Step 2: Request Refund Twice with the Same Idempotency Key
	refundKey := uuid.New().String()
	var firstRefundTxID string

	for i := 0; i < 2; i++ {
		// If refundTransfer expects uuid.UUID for transaction ID, use transferRes.TransactionID directly (if typed uuid.UUID)
		// or parse it using uuid.MustParse if it's currently a string:
		refundRes, err := app.refundTransfer(ctx, uuid.MustParse(transferRes.TransactionID), receiverUserID, refundKey)
		if err != nil {
			t.Fatalf("Refund iteration %d failed: %v", i+1, err)
		}

		if i == 0 {
			firstRefundTxID = refundRes.TransactionID// or string value
		} else {
			if refundRes.TransactionID != firstRefundTxID {
				t.Fatalf("Iteration %d created duplicate refund transaction ID!", i+1)
			}
		}
	}

	// Step 3: Verify Balances Returned to Original Values
	finalSenderBalance := getWalletBalance(t, app, senderWalletID)
	finalReceiverBalance := getWalletBalance(t, app, receiverWalletID)

	if finalSenderBalance != 100000 {
		t.Fatalf("Expected sender balance to return to 100000, got %d", finalSenderBalance)
	}
	if finalReceiverBalance != 0 {
		t.Fatalf("Expected receiver balance to return to 0, got %d", finalReceiverBalance)
	}
}


//HELPERS
// 1. Initialize DB Connection for Testing
func setupTestApp(t *testing.T) *Application {
	t.Helper()

	// Connect to your local test database
	_ = godotenv.Load("../../.env") // Load .env if needed
	connStr := os.Getenv("TEST_DATABASE_URL")
	db, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}

	app := &Application{db: db}

	// Register cleanup to close pool when test finishes
	t.Cleanup(func() {
		db.Close()
	})

	return app
}

// 2. Seed Test Wallets with Pre-configured Balances
func seedTestWallets(t *testing.T, app *Application, senderBalance, receiverBalance int64) (uuid.UUID, uuid.UUID, string) {
	t.Helper()
	ctx := context.Background()

	senderUserID := uuid.New()
	receiverUserID := uuid.New()
	senderWalletID := uuid.New()
	receiverWalletID := uuid.New()

	// 1. Insert Sender User
	_, err := app.db.Exec(ctx, `
		INSERT INTO users (id, name, email, password_hash) 
		VALUES ($1, $2, $3, 'hashed_pass')`,
		senderUserID, "Sender User", fmt.Sprintf("sender-%s@example.com", senderUserID),
	)
	if err != nil {
		t.Fatalf("failed to seed sender user: %v", err)
	}

	// 2. Insert Recipient User
	_, err = app.db.Exec(ctx, `
		INSERT INTO users (id, name, email, password_hash) 
		VALUES ($1, $2, $3, 'hashed_pass')`,
		receiverUserID, "Recipient User", fmt.Sprintf("receiver-%s@example.com", receiverUserID),
	)
	if err != nil {
		t.Fatalf("failed to seed receiver user: %v", err)
	}

	// 3. Insert Sender Wallet
	_, err = app.db.Exec(ctx, `
		INSERT INTO wallets (id, user_id, balance, currency, status) 
		VALUES ($1, $2, $3, 'IDR', 'ACTIVE')`,
		senderWalletID, senderUserID, senderBalance,
	)
	if err != nil {
		t.Fatalf("failed to seed sender wallet: %v", err)
	}

	// 4. Insert Recipient Wallet
	_, err = app.db.Exec(ctx, `
		INSERT INTO wallets (id, user_id, balance, currency, status) 
		VALUES ($1, $2, $3, 'IDR', 'ACTIVE')`,
		receiverWalletID, receiverUserID, receiverBalance,
	)
	if err != nil {
		t.Fatalf("failed to seed receiver wallet: %v", err)
	}

	// 5. Automatic Cleanup in reverse order of foreign keys
	t.Cleanup(func() {
		app.db.Exec(ctx, `DELETE FROM ledger_entries WHERE wallet_id IN ($1, $2)`, senderWalletID, receiverWalletID)
		app.db.Exec(ctx, `DELETE FROM idempotency_keys WHERE user_id IN ($1, $2)`, senderUserID, receiverUserID)
		app.db.Exec(ctx, `DELETE FROM wallets WHERE id IN ($1, $2)`, senderWalletID, receiverWalletID)
		app.db.Exec(ctx, `DELETE FROM users WHERE id IN ($1, $2)`, senderUserID, receiverUserID)
	})

	return senderWalletID, receiverWalletID, senderUserID.String()
}


// 3. Query DB directly to check final state
func getWalletBalance(t *testing.T, app *Application, walletID uuid.UUID) int64 {
	t.Helper()
	var balance int64
	err := app.db.QueryRow(context.Background(), `SELECT balance FROM wallets WHERE id = $1`, walletID).Scan(&balance)
	if err != nil {
		t.Fatalf("failed to fetch wallet balance: %v", err)
	}
	return balance
}