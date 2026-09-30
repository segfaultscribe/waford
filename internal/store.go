package internal

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Store struct {
	db  *sql.DB
	idg *Generator
}

func NewStore(db *sql.DB) *Store {
	return &Store{
		db:  db,
		idg: NewGenerator(),
	}
}

func (s *Store) SaveDistributor(ctx context.Context, name string) (string, error) {
	ulid := s.idg.New().String()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO distributors (id, name, created_at) VALUES (?, ?, ?)`,
		ulid, name, time.Now().UTC(),
	)
	return ulid, err
}

func (s *Store) SaveDestinations(ctx context.Context, distributorID string, urls []string) error {
	if len(urls) == 0 {
		return nil
	}

	// Build the base query and placeholder groups dynamically
	// Batch insert for multiple urls
	valueStrings := make([]string, 0, len(urls))
	valueArgs := make([]any, 0, len(urls)*3)

	for _, url := range urls {
		ulid := s.idg.New().String()
		valueStrings = append(valueStrings, "(?, ?, ?)")
		valueArgs = append(valueArgs, ulid, distributorID, url)
	}

	query := fmt.Sprintf(
		"INSERT INTO destinations (id, distributor_id, url) VALUES %s",
		strings.Join(valueStrings, ","),
	)

	_, err := s.db.ExecContext(ctx, query, valueArgs...)
	return err
}

func (s *Store) SaveEvent(ctx context.Context, distributorID string, payload []byte) (string, error) {
	ulid := s.idg.New().String()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO events (id, distributor_id, payload, created_at) VALUES (?, ?, ?, ?)`,
		ulid, distributorID, payload, time.Now().UTC(),
	)
	return ulid, err
}

func (s *Store) GetUnregisteredEvents(ctx context.Context) ([]Event, error) {
	query := `
        SELECT e.id, e.distributor_id, e.payload, d.url 
        FROM events e
        JOIN destinations d ON e.distributor_id = d.distributor_id
        WHERE e.fanned_out = 0
    `
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.DistributorID, &e.FannedOut, &e.Payload, &e.URL); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (s *Store) SaveDeliveries(ctx context.Context, event []Event) error {
	if len(event) == 0 {
		return nil
	}

	// Build the dynamic batch insert query
	valueStrings := make([]string, 0, len(event))
	valueArgs := make([]any, 0, len(event)*5)
	eventIDsToUpdate := make(map[string]bool)

	for _, item := range event {
		deliveryID := s.idg.New().String()
		valueStrings = append(valueStrings, "(?, ?, ?, ?, ?)")
		valueArgs = append(valueArgs, deliveryID, item.ID, item.DistributorID, item.URL, "pending")
		eventIDsToUpdate[item.ID] = true
	}

	batchQuery := fmt.Sprintf(
		"INSERT INTO deliveries (id, event_id, distributor_id, url, status) VALUES %s",
		strings.Join(valueStrings, ","),
	)

	// transaction start
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Insert into deliveries
	if _, err := tx.ExecContext(ctx, batchQuery, valueArgs...); err != nil {
		return err
	}

	for eventID := range eventIDsToUpdate {
		if _, err := tx.ExecContext(ctx, `UPDATE events SET fanned_out = 1 WHERE id = ?`, eventID); err != nil {
			return err
		}
	}

	return tx.Commit()
}
