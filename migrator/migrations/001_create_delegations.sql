-- +migrate Up
-- Create delegations table
CREATE TABLE IF NOT EXISTS delegations (
    id BIGINT PRIMARY KEY,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
    amount BIGINT NOT NULL,
    delegator TEXT NOT NULL,
    level BIGINT NOT NULL,
    year INTEGER NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create standalone timestamp index for default queries without year filtering
CREATE INDEX IF NOT EXISTS idx_delegations_timestamp ON delegations (timestamp DESC); 

-- Create composite index for optimal year filtering and pagination
CREATE INDEX IF NOT EXISTS idx_delegations_year_timestamp ON delegations (year, timestamp DESC); 
