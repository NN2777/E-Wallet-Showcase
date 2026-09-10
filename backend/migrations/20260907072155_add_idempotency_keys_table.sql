-- +goose Up
-- +goose StatementBegin
CREATE TABLE idempotency_keys (
    key UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    request_path TEXT NOT NULL,
    request_params JSONB NOT NULL,
    response_code INT NOT NULL,
    response_body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_idempotency_user_key ON idempotency_keys (user_id, key);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS idempotency_keys;
-- +goose StatementEnd