-- +migrate Up
-- Create delegations table
CREATE TABLE IF NOT EXISTS delegations (
    id BIGINT PRIMARY KEY,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
    amount BIGINT NOT NULL,
    delegator TEXT NOT NULL,
    level BIGINT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_delegations_timestamp ON delegations (timestamp); 