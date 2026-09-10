-- +goose Up

CREATE TABLE users (
    id UUID PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE wallets (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    balance BIGINT NOT NULL DEFAULT 0,
    currency VARCHAR(3) NOT NULL DEFAULT 'IDR',
    status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT wallets_balance_not_negative
        CHECK (balance >= 0),

    CONSTRAINT wallets_valid_status
        CHECK (status IN ('ACTIVE', 'FROZEN', 'CLOSED')),

    CONSTRAINT wallets_user_currency_unique
        UNIQUE (user_id, currency)
);

CREATE TABLE transactions (
    id UUID PRIMARY KEY,
    reference VARCHAR(100) NOT NULL UNIQUE,
    type VARCHAR(30) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    amount BIGINT NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT transactions_amount_positive
        CHECK (amount > 0),

    CONSTRAINT transactions_valid_type
        CHECK (
            type IN (
                'TOP_UP',
                'TRANSFER',
                'PAYMENT',
                'REFUND'
            )
        ),

    CONSTRAINT transactions_valid_status
        CHECK (
            status IN (
                'PENDING',
                'COMPLETED',
                'FAILED',
                'REVERSED'
            )
        )
);

CREATE TABLE ledger_entries (
    id UUID PRIMARY KEY,
    transaction_id UUID NOT NULL REFERENCES transactions(id),
    wallet_id UUID NOT NULL REFERENCES wallets(id),
    direction VARCHAR(10) NOT NULL,
    amount BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT ledger_entries_amount_positive
        CHECK (amount > 0),

    CONSTRAINT ledger_entries_valid_direction
        CHECK (direction IN ('DEBIT', 'CREDIT')),

    CONSTRAINT ledger_entry_unique
        UNIQUE (transaction_id, wallet_id, direction)
);

CREATE INDEX ledger_entries_wallet_created_index
    ON ledger_entries(wallet_id, created_at DESC);

CREATE INDEX ledger_entries_transaction_index
    ON ledger_entries(transaction_id);


-- +goose Down

DROP TABLE ledger_entries;
DROP TABLE transactions;
DROP TABLE wallets;
DROP TABLE users;