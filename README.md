# 💳 High-Concurrency Fintech Wallet & Ledger Backend

A robust, ACID-compliant e-wallet backend engine built with **Go** and **PostgreSQL**. Designed to handle concurrent transfer requests safely using double-entry ledger accounting, pessimistic database locking (`FOR UPDATE`), and strict idempotency key enforcement. Integrated with the **Midtrans API** for seamless payment top-ups.

---

## 🌟 Key Features

* **Double-Entry Ledger System:** Every financial transaction writes balanced debit and credit entries to maintain strict auditability.
* **Concurrency & Race Condition Prevention:** Employs PostgreSQL row-level locks (`SELECT ... FOR UPDATE`) to prevent double-spending under high concurrent load.
* **Strict Idempotency:** Implements centralized idempotency storage to safely suppress duplicate API retries and gateway webhooks.
* **Midtrans Sandbox Integration:** Fully verified webhook processing flow with signature verification.
* **Zero-Framework Architecture:** Built exclusively with Go's standard library (`net/http`) to demonstrate core HTTP and middleware mastery.
* **Strongly Typed Identifiers:** Fully standardized around `uuid.UUID` across handlers, domain logic, and database entities.

---

## 🏗 System Architecture
```
+-----------------------+         +-----------------------+
|  Next.js Frontend UI  |  ---->  |    Go Backend API     |
| React / TS / Tailwind |         |   (net/http Router)   |
+-----------------------+         +-----------------------+
                                              |
                                              v
                                  +-----------------------+
                                  |  PostgreSQL Database  |
                                  | (Ledger, Locks, Keys) |
                                  +-----------------------+
```
---

## 🧪 Concurrency & Safety Verification

The system includes a rigorous automated test suite testing severe race conditions, duplicate request suppression, and atomic refund flows.

Race Condition & Stress Tests Covered:
- Scenario 1: 100 concurrent goroutines attempting to spend Rp80.000 simultaneously from a Rp100.000 balance (verifies exactly 1 succeeds and 99 fail safely).
- Scenario 2: Duplicate request execution under identical idempotency keys (verifies cached execution without double debiting).
- Scenario 3: Webhook settlement retries and edge cases.
- Scenario 4: Atomic transaction refunds verifying total balance integrity.

To execute the test suite:

```
cd backend
go test -v ./cmd/api
```

---

🚀 Quick Start Guide

Prerequisites

- Go (1.21 or higher)
- PostgreSQL running locally or via Docker
- Midtrans Sandbox Account (optional for live webhooks)

1. Clone & Configure
```
git clone [https://github.com/YOUR_USERNAME/E-Wallet-Showcase.git](https://github.com/YOUR_USERNAME/E-Wallet-Showcase.git)
cd E-Wallet-Showcase/backend
```
2. Set Up Environment Variables
Copy the example environment file and update your PostgreSQL and Midtrans credentials:
```
cp .env.example .env
```
3. Run Migrations & Start Application
```
# Apply migrations to your database
go run ./cmd/api
```
The API server will launch on http://localhost:8080.

## 🛠 Tech Stack

### Core Engine (Primary Focus)
* **Language:** Go (Golang) — `net/http` standard library
* **Database:** PostgreSQL (`pgx` driver)
* **Concurrency & Safety:** Pessimistic Row Locking (`FOR UPDATE`), Double-Entry Accounting
* **Payment Gateway:** Midtrans Sandbox API (Payment Links & Webhooks)

### Dashboard Client (Supporting)
* **Framework:** Next.js (React), TypeScript, Tailwind CSS