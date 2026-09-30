-- +goose Up
CREATE TABLE IF NOT EXISTS deliveries (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL,
    distributor_id TEXT NOT NULL,
    url TEXT NOT NULL,
    status TEXT DEFAULT 'pending',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    retry_count INTEGER DEFAULT 0,
    next_retry_at DATETIME,
    last_error TEXT,
    dead_at DATETIME,
    FOREIGN KEY (event_id) REFERENCES events(id) ON DELETE CASCADE,
    FOREIGN KEY (distributor_id) REFERENCES distributors(id) ON DELETE CASCADE
);

CREATE INDEX idx_deliveries_status ON deliveries(status, next_retry_at);
CREATE INDEX idx_deliveries_event_id ON deliveries(event_id);