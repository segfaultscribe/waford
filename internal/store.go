package internal

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/segfaultscribe/waford/internal/metrcs"
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

func (s *Store) SaveEvent(ctx context.Context, distributorID string, payload, meta []byte) (string, error) {
	ulid := s.idg.New().String()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO events (id, distributor_id, payload, meta, created_at) VALUES (?, ?, ?, ?, ?)`,
		ulid, distributorID, payload, meta, time.Now().UTC(),
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
		valueArgs = append(valueArgs, deliveryID, item.ID, item.DestinationID, StatusPending)
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

func (s *Store) ExecuteQuery(ctx context.Context, query string, args ...any) error {
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *Store) BatchUpdateDeliveries(ctx context.Context, batch []DeliveryBatchData, MaxRetries int) (int64, error) {
	if len(batch) == 0 {
		return 0, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		UPDATE deliveries 
		SET status = ?,
		    retry_count = ?,
		    next_retry_at = ?,
		    dead_at = ?,
		    last_error = ?,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`)
	if err != nil {
		return 0, fmt.Errorf("prepare stmt: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC()
	var totalRowsAffected int64

	for i := range batch {
		item := &batch[i]

		var (
			status        string
			nextRetryAt   string
			deadAt        *time.Time
			errStr        *string
			newRetryCount = item.RetryCount
		)

		if item.Success {
			status = StatusDelivered
		} else {
			if item.ErrMessage != "" {
				errStr = &item.ErrMessage
			}

			if item.RetryCount < MaxRetries {
				status = StatusRetry
				newRetryCount = item.RetryCount + 1
				nextRetryAt = item.BackoffTime.UTC().Format("2006-01-02 15:04:05")
				metrcs.DeliveriesTotal.WithLabelValues("retry").Inc()
			} else {
				status = StatusDLQ
				deadAt = &now
				metrcs.DeliveriesTotal.WithLabelValues("dlq").Inc()
			}
		}

		res, err := stmt.ExecContext(ctx,
			status,
			newRetryCount,
			nextRetryAt,
			deadAt,
			errStr,
			item.ID,
		)
		if err != nil {
			return 0, fmt.Errorf("exec update for delivery %s: %w", item.ID, err)
		}

		// Read rows affected for this iteration (0 overhead)
		n, err := res.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("get rows affected for %s: %w", item.ID, err)
		}
		totalRowsAffected += n
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}

	return totalRowsAffected, nil
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
			WHERE status = 'pending'
			ORDER BY created_at ASC
			LIMIT ?
		)

		UNION ALL

		SELECT id FROM (
			SELECT id FROM deliveries
			WHERE status = 'retry'
			AND retry_count <= ?
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
		SELECT d.id, dest.url, d.retry_count, e.payload, e.meta
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
		if err := deliveryRows.Scan(&d.ID, &d.Url, &d.retryCount, &d.Payload, &d.meta); err != nil {
			return nil, err
		}
		deliveries = append(deliveries, d)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return deliveries, nil
}

func (s *Store) GetDistributors(ctx context.Context, id string) (*Distributor, error) {
	query := `
		SELECT d.name, GROUP_CONCAT(dest.url, ',')
        FROM distributors d
        JOIN destinations dest ON dest.distributor_id = d.id
        WHERE d.id = ?
        GROUP BY d.id;
		`

	var D Distributor
	var rawURLs string

	err := s.db.QueryRowContext(ctx, query, id).Scan(&D.Name, &rawURLs)
	if err != nil {
		s.logger.Error("Could not retrieve distributor")
		return nil, err
	}

	if rawURLs != "" {
		D.Destinations = strings.Split(rawURLs, ",")
	} else {
		s.logger.Error("Unexpected error: empty destinations")
	}

	return &D, nil
}

func (s *Store) DeleteDistributors(ctx context.Context, id string) error {
	query := `
		DELETE FROM distributors WHERE id = ?
	`
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (s *Store) GetTotalEvents(ctx context.Context) (int, error) {
	query := `
		SELECT count(*) FROM events
	`
	var count int = 0
	err := s.db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (s *Store) GetDeliveryStat(ctx context.Context) (*DeliveryStats, error) {
	query := `
		SELECT
		COUNT(*) AS total,
		COUNT(*) FILTER (WHERE status = 'delivered') AS successful,
		COUNT(*) FILTER (WHERE status = 'pending') AS pending,
		COUNT(*) FILTER (WHERE status = 'retry') AS retry,
		COUNT(*) FILTER (WHERE status = 'dlq') AS dead
		FROM deliveries;
	`
	var dStats DeliveryStats
	err := s.db.QueryRowContext(ctx, query).Scan(
		&dStats.Total,
		&dStats.Successful,
		&dStats.Pending,
		&dStats.Retry,
		&dStats.Dead,
	)
	if err != nil {
		return nil, err
	}

	return &dStats, nil
}

func (s *Store) GetAllDistributors(ctx context.Context) ([]*DistributorView, error) {
	query := `
        SELECT d.id, d.name, dest.url
        FROM distributors d
        LEFT JOIN destinations dest ON d.id = dest.distributor_id
        ORDER BY d.name, dest.url
    `

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	distributorMap := make(map[string]*DistributorView)
	var ids []string

	for rows.Next() {
		var id, name string
		var url sql.NullString

		if err := rows.Scan(&id, &name, &url); err != nil {
			return nil, err
		}

		distributor, exists := distributorMap[id]
		if !exists {
			distributor = &DistributorView{
				ID:           id,
				Name:         name,
				Destinations: []string{},
			}
			distributorMap[id] = distributor
			ids = append(ids, id)
		}

		if url.Valid {
			distributor.Destinations = append(
				distributor.Destinations,
				url.String,
			)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]*DistributorView, 0, len(ids))
	for _, id := range ids {
		result = append(result, distributorMap[id])
	}

	return result, nil
}

func (s *Store) getAllEvents(ctx context.Context) ([]EventView, error) {
	query := `
		SELECT
		e.id,
		e.distributor_id,
		d.name AS distributor_name,
		e.fanned_out,
		e.created_at,

		COUNT(dl.id) AS total_deliveries,

		SUM(CASE
			WHEN dl.status = 'delivered' THEN 1
			ELSE 0
		END) AS successful_deliveries,

		SUM(CASE
			WHEN dl.status = 'dlq' THEN 1
			ELSE 0
		END) AS failed_deliveries

		FROM events e
		JOIN distributors d
			ON d.id = e.distributor_id
		LEFT JOIN deliveries dl
			ON dl.event_id = e.id

		GROUP BY
			e.id,
			e.distributor_id,
			d.name,
			e.fanned_out,
			e.created_at

		ORDER BY e.created_at DESC
		LIMIT 100;
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []EventView
	for rows.Next() {
		var event EventView
		var fannedOut bool

		if err := rows.Scan(
			&event.ID,
			&event.DistributorID,
			&event.DistributorName,
			&fannedOut,
			&event.CreatedAt,
			&event.TotalDeliveries,
			&event.SuccessfulDeliveries,
			&event.FailedDeliveries,
		); err != nil {
			return nil, err
		}

		event.Status = deriveEventStatus(
			fannedOut,
			event.TotalDeliveries,
			event.SuccessfulDeliveries,
			event.FailedDeliveries,
		)

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return events, nil
}

func (s *Store) GetEventDetail(
	ctx context.Context,
	eventID string,
) (EventDetailView, error) {
	query := `
        SELECT
            e.id,
            e.distributor_id,
            d.name,
            e.fanned_out,
            e.created_at,
            e.payload,
            e.meta,
            COUNT(dl.id),
            COALESCE(SUM(CASE
                WHEN dl.status = 'delivered' THEN 1
                ELSE 0
            END), 0),
            COALESCE(SUM(CASE
                WHEN dl.status = 'dlq' THEN 1
                ELSE 0
            END), 0)
        FROM events e
        JOIN distributors d ON d.id = e.distributor_id
        LEFT JOIN deliveries dl ON dl.event_id = e.id
        WHERE e.id = ?
        GROUP BY
            e.id, e.distributor_id, d.name,
            e.fanned_out, e.created_at, e.payload, e.meta;
    `

	var event EventDetailView
	var fannedOut bool

	err := s.db.QueryRowContext(ctx, query, eventID).Scan(
		&event.ID,
		&event.DistributorID,
		&event.DistributorName,
		&fannedOut,
		&event.CreatedAt,
		&event.Payload,
		&event.Meta,
		&event.TotalDeliveries,
		&event.SuccessfulDeliveries,
		&event.FailedDeliveries,
	)
	if err != nil {
		return EventDetailView{}, err
	}

	event.Status = deriveEventStatus(
		fannedOut,
		event.TotalDeliveries,
		event.SuccessfulDeliveries,
		event.FailedDeliveries,
	)

	return event, nil
}

func (s *Store) GetDeliveriesByEventID(
	ctx context.Context,
	eventID string,
) ([]DeliveryDetailView, error) {
	query := `
        SELECT
            dl.id,
            dl.destination_id,
            dest.url,
            dl.status,
            dl.retry_count,
            dl.last_error,
            dl.created_at,
            dl.updated_at,
            dl.dead_at
        FROM deliveries dl
        JOIN destinations dest ON dest.id = dl.destination_id
        WHERE dl.event_id = ?
        ORDER BY dl.created_at ASC;
    `

	rows, err := s.db.QueryContext(ctx, query, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deliveries []DeliveryDetailView

	for rows.Next() {
		var delivery DeliveryDetailView
		var lastError, deadAt sql.NullString

		if err := rows.Scan(
			&delivery.ID,
			&delivery.DestinationID,
			&delivery.DestinationURL,
			&delivery.Status,
			&delivery.RetryCount,
			&lastError,
			&delivery.CreatedAt,
			&delivery.UpdatedAt,
			&deadAt,
		); err != nil {
			return nil, err
		}

		if lastError.Valid {
			delivery.LastError = lastError.String
		}

		if deadAt.Valid {
			delivery.DeadAt = deadAt.String
		}

		deliveries = append(deliveries, delivery)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return deliveries, nil
}
