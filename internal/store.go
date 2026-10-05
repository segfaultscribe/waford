package internal

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type Store struct {
	db     *sql.DB
	idg    *Generator
	logger *slog.Logger
}

func NewStore(db *sql.DB, logger *slog.Logger) *Store {
	return &Store{
		db:     db,
		logger: logger,
		idg:    NewGenerator(),
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
        SELECT e.id, e.payload, d.id 
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
		if err := rows.Scan(&e.ID, &e.Payload, &e.DestinationID); err != nil {
			return nil, err
		}
		// After scanning each row, log it
		s.logger.Info("[chronos] Unregistered event",
			"event_id", e.ID,
			"destination_id", e.DestinationID)
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (s *Store) SaveDeliveries(ctx context.Context, events []Event) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}

	valueStrings := make([]string, 0, len(events))
	valueArgs := make([]any, 0, len(events)*5)
	eventIDsToUpdate := make(map[string]struct{})

	for _, item := range events {
		deliveryID := s.idg.New().String()
		valueStrings = append(valueStrings, "(?, ?, ?, ?)")
		valueArgs = append(valueArgs, deliveryID, item.ID, item.DestinationID, "pending")
		eventIDsToUpdate[item.ID] = struct{}{}
	}

	batchInsertQuery := fmt.Sprintf(
		"INSERT INTO deliveries (id, event_id, destination_id, status) VALUES %s",
		strings.Join(valueStrings, ","),
	)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// 1. Batch Insert Deliveries
	res, err := tx.ExecContext(ctx, batchInsertQuery, valueArgs...)
	if err != nil {
		return 0, err
	}

	insertedRows, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}

	s.logger.Info("[chronos] SaveDeliveries result",
		"attempted", len(events),
		"inserted", insertedRows,
		"skipped", len(events)-int(insertedRows))

	// 2. Single Batch Update Events
	eventIDs := make([]any, 0, len(eventIDsToUpdate))
	placeholders := make([]string, 0, len(eventIDsToUpdate))

	for id := range eventIDsToUpdate {
		placeholders = append(placeholders, "?")
		eventIDs = append(eventIDs, id)
	}

	batchUpdateQuery := fmt.Sprintf(
		"UPDATE events SET fanned_out = 1 WHERE id IN (%s)",
		strings.Join(placeholders, ","),
	)

	if _, err := tx.ExecContext(ctx, batchUpdateQuery, eventIDs...); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	// Temporary debug only — remove after diagnosis
	var count int
	tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM deliveries").Scan(&count)
	s.logger.Info("[chronos] Deliveries table count after insert", "total", count)

	return int(insertedRows), nil
}

// func (s *Store) JobFinder(ctx context.Context) ([]Delivery, error) {
// 	query := `
// 		WITH claimed AS (
// 			UPDATE deliveries
// 			SET status = 'processing',
// 			    updated_at = CURRENT_TIMESTAMP
// 			WHERE id IN (
// 				SELECT id FROM (
// 					SELECT id FROM deliveries
// 					WHERE status = 'pending' AND retry_count < 3
// 					ORDER BY created_at ASC
// 					LIMIT 40
// 				)
// 				UNION ALL
// 				SELECT id FROM (
// 					SELECT id FROM deliveries
// 					WHERE status = 'retry'
// 					  AND retry_count < 3
// 					  AND (next_retry_at IS NULL OR next_retry_at <= CURRENT_TIMESTAMP)
// 					ORDER BY created_at ASC
// 					LIMIT 10
// 				)
// 			)
// 			RETURNING id, event_id, destination_id, retry_count, created_at
// 		)
// 		SELECT
// 			c.id,
// 			dest.url,
// 			c.retry_count,
// 			e.payload
// 		FROM claimed c
// 		JOIN events e ON c.event_id = e.id
// 		JOIN destinations dest ON c.destination_id = dest.id
// 		ORDER BY c.created_at ASC;
// 	`
// 	rows, err := s.db.QueryContext(ctx, query)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer rows.Close()

// 	var deliveries []Delivery
// 	for rows.Next() {
// 		var d Delivery
// 		if err := rows.Scan(&d.ID, &d.Url, &d.retryCount, &d.Payload); err != nil {
// 			return nil, err
// 		}
// 		deliveries = append(deliveries, d)
// 	}

// 	return deliveries, nil
// }

func (s *Store) ExecuteQuery(ctx context.Context, query string, args ...any) error {
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

// BuildBatchUpdateQuery constructs a dynamic, parameterized SQL query to update a batch of delivery outcomes
// (both successful and failed) in a single SQLite statement using CASE expressions.
//
// This approach minimizes I/O overhead by executing a single batch UPDATE query per worker batch instead
// of issuing individual UPDATE statements inside a transaction loop.
//
// State Changes Handled per Delivery:
//   - Success: Sets status to 'success', clears next_retry_at, dead_at, and last_error.
//   - Failure (< MaxRetries): Increments retry_count, sets status to 'retry', calculates next_retry_at backoff,
//     and logs last_error.
//   - Failure (>= MaxRetries): Sets status to 'dlq', marks dead_at timestamp, clears next_retry_at backoff,
//     and logs last_error.
//   - Universal: Always sets updated_at to CURRENT_TIMESTAMP across all processed rows in the batch.
func (s *Store) BuildBatchUpdateQuery(results []DeliveryBatchData) (string, []any) {
	const MaxRetries = 3

	if len(results) == 0 {
		return "", nil
	}

	// Pre-allocate slice capacity to prevent dynamic re-allocations during argument collection.
	// Average arguments per result: ~8 parameters across CASE conditions + 1 for WHERE IN clause.
	args := make([]any, 0, len(results)*9)

	var statusCase strings.Builder
	var retryCase strings.Builder
	var nextRetryCase strings.Builder
	var deadAtCase strings.Builder
	var lastErrorCase strings.Builder
	var inClause strings.Builder

	statusCase.WriteString("status = CASE id ")
	retryCase.WriteString("retry_count = CASE id ")
	nextRetryCase.WriteString("next_retry_at = CASE id ")
	deadAtCase.WriteString("dead_at = CASE id ")
	lastErrorCase.WriteString("last_error = CASE id ")
	inClause.WriteString("WHERE id IN (")

	for i, res := range results {
		if i > 0 {
			inClause.WriteString(", ")
		}

		if res.Success {
			// SUCCESS PATH: Transition delivery to 'success' state and clear transient error tracking columns.
			statusCase.WriteString("WHEN ? THEN 'success' ")
			args = append(args, res.ID)

			nextRetryCase.WriteString("WHEN ? THEN NULL ")
			args = append(args, res.ID)

			deadAtCase.WriteString("WHEN ? THEN NULL ")
			args = append(args, res.ID)

			lastErrorCase.WriteString("WHEN ? THEN NULL ")
			args = append(args, res.ID)
		} else {
			// FAILURE PATH: Evaluate current retry threshold against MaxRetries.
			// If (retry_count + 1 >= MaxRetries), route job to Dead Letter Queue ('dlq'); otherwise mark 'retry'.
			statusCase.WriteString(fmt.Sprintf("WHEN ? THEN CASE WHEN retry_count + 1 >= %d THEN 'dlq' ELSE 'retry' END ", MaxRetries))
			args = append(args, res.ID)

			// Increment failure count monotonically.
			retryCase.WriteString("WHEN ? THEN retry_count + 1 ")
			args = append(args, res.ID)

			// Calculate next execution timestamp if eligible for retry; clear schedule if terminal failure.
			nextRetryCase.WriteString("WHEN ? THEN CASE WHEN retry_count + 1 >= ? THEN NULL ELSE ? END ")
			args = append(args, res.ID, MaxRetries, res.BackoffTime)

			// Mark dead_at timestamp upon moving to DLQ state for auditability.
			deadAtCase.WriteString("WHEN ? THEN CASE WHEN retry_count + 1 >= ? THEN CURRENT_TIMESTAMP ELSE NULL END ")
			args = append(args, res.ID, MaxRetries)

			// Capture error message returned by Hermes transport worker.
			lastErrorCase.WriteString("WHEN ? THEN ? ")
			args = append(args, res.ID, res.ErrMessage)
		}

		inClause.WriteString("?")
	}

	// Finalize conditional CASE clauses with appropriate fallbacks for unhandled cases.
	statusCase.WriteString("END")
	retryCase.WriteString("ELSE retry_count END")
	nextRetryCase.WriteString("END")
	deadAtCase.WriteString("END")
	lastErrorCase.WriteString("END")
	inClause.WriteString(")")

	// Bind target primary keys to populate the terminal 'WHERE id IN (?, ...)' predicate filter.
	for _, res := range results {
		args = append(args, res.ID)
	}

	// Assemble final parameterized SQL statement including mandatory updated_at touch.
	query := fmt.Sprintf(
		"UPDATE deliveries SET %s, %s, %s, %s, %s, updated_at = CURRENT_TIMESTAMP %s;",
		statusCase.String(),
		retryCase.String(),
		nextRetryCase.String(),
		deadAtCase.String(),
		lastErrorCase.String(),
		inClause.String(),
	)

	return query, args
}

func (s *Store) RecoverStaleJobs(ctx context.Context, timeout time.Duration) (int64, error) {
	query := `
		UPDATE deliveries 
		SET status = 'retry',
		    updated_at = CURRENT_TIMESTAMP
		WHERE status = 'processing' 
		  AND updated_at <= DATETIME('now', ?);
	`

	threshold := fmt.Sprintf("-%d seconds", int(timeout.Seconds()))
	res, err := s.db.ExecContext(ctx, query, threshold)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

// reference

// // 1. Start the transaction via Go's driver
// tx, err := db.Begin()
// if err != nil {
//     return err
// }
// // Automatically rollback if anything fails before tx.Commit()
// defer tx.Rollback()

// // 2. Step 1: Create the temporary table
// _, err = tx.Exec(`
//     CREATE TEMP TABLE ids_to_claim AS
//     SELECT id FROM (
//         SELECT id FROM deliveries
//         WHERE status = 'pending' AND retry_count < 3
//         ORDER BY created_at ASC LIMIT 40
//     )
//     UNION ALL
//     SELECT id FROM (
//         SELECT id FROM deliveries
//         WHERE status = 'retry'
//           AND retry_count < 3
//           AND (next_retry_at IS NULL OR next_retry_at <= CURRENT_TIMESTAMP)
//         ORDER BY created_at ASC LIMIT 10
//     );
// `)
// if err != nil {
//     return err
// }

// // 3. Step 2: Update the statuses
// _, err = tx.Exec(`
//     UPDATE deliveries
//     SET status = 'processing', updated_at = CURRENT_TIMESTAMP
//     WHERE id IN (SELECT id FROM ids_to_claim);
// `)
// if err != nil {
//     return err
// }

// // 4. Step 3: Query the final payloads
// rows, err := tx.Query(`
//     SELECT d.id, dest.url, d.retry_count, e.payload
//     FROM ids_to_claim tmp
//     JOIN deliveries d ON tmp.id = d.id
//     JOIN events e ON d.event_id = e.id
//     JOIN destinations dest ON d.destination_id = dest.id
//     ORDER BY d.created_at ASC;
// `)
// if err != nil {
//     return err
// }
// defer rows.Close()

// // ... Scan your rows here ...

// // 5. Commit the transaction
// return tx.Commit()

func (s *Store) JobFinder(ctx context.Context, retryCount, pendingLimit, retryLimit int) ([]Delivery, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Triple shot process
	// get the ids to process
	// set stauts to in_flight
	// get all required data

	getIdsQuery := `
			SELECT id FROM (
			SELECT id FROM deliveries
			WHERE status = 'pending' AND retry_count < ?
			ORDER BY created_at ASC
			LIMIT ?
		)

		UNION ALL

		SELECT id FROM (
			SELECT id FROM deliveries
			WHERE status = 'retry'
			AND retry_count < ?
			AND (next_retry_at IS NULL OR next_retry_at <= CURRENT_TIMESTAMP)
			ORDER BY created_at ASC
			LIMIT ?
		)
	`

	setStatusQuery := `
		UPDATE deliveries SET status = "in_flight"
		WHERE id IN (%s)
	`

	getActualDeliveryQuery := `
		SELECT d.id, dest.url, d.retry_count, e.payload
		FROM deliveries d
		JOIN events e on d.event_id = e.id
		JOIN destinations dest on d.destination_id = dest.id
		WHERE d.id in (%s)`

	// ids store strings exclusively, BUT
	// ExecContext requires []any for variadic arguments
	var ids []any
	var params []any
	params = append(params, retryCount, pendingLimit, retryCount, retryLimit)
	rows, err := tx.QueryContext(ctx, getIdsQuery, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return nil, nil
	}

	queryPlaceholder := strings.Repeat("?,", len(ids))
	queryPlaceholder = strings.TrimRight(queryPlaceholder, ",")

	setStatusQuery = fmt.Sprintf(setStatusQuery, queryPlaceholder)

	_, err = tx.ExecContext(ctx, setStatusQuery, ids...)
	if err != nil {
		return nil, err
	}

	actualDeliveriesQuery := fmt.Sprintf(getActualDeliveryQuery, queryPlaceholder)

	deliveryRows, err := tx.QueryContext(ctx, actualDeliveriesQuery, ids...)
	if err != nil {
		return nil, err
	}

	var deliveries []Delivery
	for deliveryRows.Next() {
		var d Delivery
		if err := deliveryRows.Scan(&d.ID, &d.Url, &d.retryCount, &d.Payload); err != nil {
			return nil, err
		}
		deliveries = append(deliveries, d)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return deliveries, nil
}
