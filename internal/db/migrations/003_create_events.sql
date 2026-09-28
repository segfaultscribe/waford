-- +goose Up
CREATE TABLE IF NOT EXISTS events (
    id TEXT PRIMARY KEY,
    distributor_id TEXT NOT NULL,
    fanned_out BOOLEAN DEFAULT FALSE,
    payload TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (distributor_id) REFERENCES distributors(id) ON DELETE CASCADE
);

CREATE INDEX idx_events_fanned_out ON events(fanned_out);