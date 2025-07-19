-- +migrate Up
-- Create scraper checkpoint table (singleton table with one row)
CREATE TABLE IF NOT EXISTS scraper_checkpoint (
    single_row BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (single_row = TRUE),
    last_id BIGINT NOT NULL
); 