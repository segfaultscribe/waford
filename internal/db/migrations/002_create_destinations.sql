-- +goose Up
CREATE TABLE IF NOT EXISTS destinations (
		id TEXT PRIMARY KEY,
		distributor_id TEXT NOT NULL,
		url TEXT NOT NULL,
		is_active BOOLEAN DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (distributor_id) REFERENCES distributors(id) ON DELETE CASCADE
	);