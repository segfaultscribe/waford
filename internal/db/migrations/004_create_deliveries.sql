-- +goose Up
CREATE TABLE IF NOT EXISTS deliveries (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL,
    destination_id TEXT NOT NULL,
    status TEXT DEFAULT 'pending',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    retry_count INTEGER DEFAULT 0,
    next_retry_at DATETIME,
    last_error TEXT,
    dead_at DATETIME,
    FOREIGN KEY (event_id) REFERENCES events(id) ON DELETE CASCADE,
    FOREIGN KEY (destination_id) REFERENCES destinations(id) ON DELETE CASCADE
);

CREATE INDEX idx_deliveries_status ON deliveries(status, next_retry_at);
CREATE INDEX idx_deliveries_event_id ON deliveries(event_id);
CREATE INDEX idx_deliveries_destination_id ON deliveries(destination_id);

CREATE INDEX IF NOT EXISTS idx_deliveries_pending 
ON deliveries (status, retry_count, created_at) 
WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_deliveries_retry 
ON deliveries (status, retry_count, next_retry_at, created_at) 
WHERE status = 'retry';