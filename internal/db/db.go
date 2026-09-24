package db

import (
	"database/sql"
	"fmt"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	instance *sql.DB
	once     sync.Once
)

// InitDB initializes the SQLite database connection and returns the instance.
func InitDB(dbPath string) (*sql.DB, error) {
	var err error

	once.Do(func() {
		// - journal_mode=WAL: Enable Write-Ahead Logging
		// - busy_timeout=5000: Wait 5s for locks to clear
		// - foreign_keys=1: Enforce foreign key constraints
		dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", dbPath)

		instance, err = sql.Open("sqlite", dsn)
		if err != nil {
			err = fmt.Errorf("failed to open database: %w", err)
			return
		}

		// prevents "database is locked" errors during heavy load.
		instance.SetMaxOpenConns(1)
		instance.SetMaxIdleConns(1)

		if err = instance.Ping(); err != nil {
			err = fmt.Errorf("failed to ping database: %w", err)
			return
		}

		fmt.Print("Database online")
	})
	return instance, err
}

// Singleton accessor for the database instance.
// Panics if InitDB hasn't been called yet.
func GetDB() *sql.DB {
	if instance == nil {
		panic("database not initialized. Call InitDB first.")
	}
	return instance
}
