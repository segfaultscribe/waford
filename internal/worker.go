package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/segfaultscribe/waford/internal/metrcs"
)

var httpClient = &http.Client{
	Timeout: 5 * time.Second,
}

func sendRequest(ctx context.Context, eventId string, url string, body json.RawMessage) error {
	// make request with context
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event-Id", eventId)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	return nil
}

func (s *Server) expBackoff(count int) time.Duration {
	// Calculate the raw exponential backoff
	multiplier := math.Pow(2, float64(count))
	backoff := float64(s.Config.Retry.BaseDelay) * multiplier
	// Cap the backoff to the maximum allowed delay
	if backoff > float64(s.Config.Retry.MaxDelay) {
		backoff = float64(s.Config.Retry.MaxDelay)
	}
	// full jitter
	jitteredBackoff := backoff * rand.Float64()
	return time.Duration(jitteredBackoff)
}

func (s *Server) Chronos(ctx context.Context, pollInterval time.Duration) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	metrcs.ChronosPollTotal.Inc()

	for {
		select {
		case <-ctx.Done():
			s.Logger.Info("[Chronos] Worker shutting down")
			return
		case <-ticker.C:
			s.FillDeliveries(ctx)
			continue
		case <-s.JM.ChronosSignal:
			s.Logger.Info("[Chornos] New event signal received, Polling events")
			s.FillDeliveries(ctx)
			continue
		}
	}
}

func (s *Server) FillDeliveries(ctx context.Context) {
	events, err := s.Store.GetUnregisteredEvents(ctx)
	if err != nil {
		s.Logger.Error("[chronos] Failed to fetch unfanned out events", "error", err)
		return
	}

	if len(events) == 0 {
		return
	}

	c, err := s.Store.SaveDeliveries(ctx, events)
	if err != nil {
		s.Logger.Error("[chronos] Failed to save deliveries", "error", err)
		return
	}

	s.Logger.Info("[Chronos] Deliveries succesfully added!", "count", c)
	select {
	case s.JM.HermesSignal <- struct{}{}:
		s.Logger.Info("[chronos] Hermes signal sent")
	default:
	}
}

func (s *Server) StartChronos(appCtx context.Context, pollInterval time.Duration) {
	s.Logger.Info("[worker] Starting Chronos")
	go s.Chronos(appCtx, pollInterval)
}

func (s *Server) Hermes(ctx context.Context) error {
	// Hermes must first construct a delivery struct for each
	// destination and then concurrently process each delivery
	for {
		// check if must proceed or exit
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// find work
		deliveries, err := s.Store.JobFinder(
			ctx,
			s.Config.Delivery.Retry.MaxRetries,
			s.Config.Delivery.PendingPullLimit,
			s.Config.Delivery.Retry.RetryPullLimit,
		)
		if err != nil {
			s.Logger.Error("[hermes] Failed to find jobs", "error", err)
			return err
		}

		if len(deliveries) == 0 {
			select {
			case <-s.JM.HermesSignal:
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}
		metrcs.HermesWorkersActive.Inc()
		s.ActiveWorkers.Add(1)

		s.Logger.Info("Hermes fetched deliveries", "count", len(deliveries))
		var wg sync.WaitGroup
		results := make(chan DeliverResult, len(deliveries))
		for _, delivery := range deliveries {
			wg.Go(func() {
				var dr DeliverResult
				dr.ID = delivery.ID
				dr.RetryCount = delivery.retryCount

				defer func() { results <- dr }()

				req, err := ReconstructRequest(ctx, delivery)

				// req, err := http.NewRequestWithContext(ctx, "POST", delivery.Url, bytes.NewReader(delivery.Payload))
				// if err != nil {
				// 	dr.Err = err
				// 	return
				// }
				// req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Waford-Delivery-Id", delivery.ID)
				start := time.Now()
				resp, err := httpClient.Do(req)
				if err != nil {
					dr.Err = err
					return
				}
				metrcs.DeliveryDuration.Observe(
					time.Since(start).Seconds(),
				)
				defer resp.Body.Close()

				if resp.StatusCode >= 300 {
					dr.Err = fmt.Errorf("bad status: %s", resp.Status)
					return
				}

				dr.Success = true
				dr.Err = nil
				dr.ID = delivery.ID
				metrcs.DeliveriesTotal.WithLabelValues("success").Inc()
				s.Logger.Info("Delivery Succes", "id", dr.ID)
			})
		}
		wg.Wait()
		close(results)

		// process the results one by one
		// bacth update the database with the results
		var batchData []DeliveryBatchData
		for res := range results {
			var bd DeliveryBatchData
			bd.ID = res.ID
			bd.Success = res.Success
			if res.Err != nil {
				bd.ErrMessage = res.Err.Error()
			}

			nextRetryAt := s.expBackoff(res.RetryCount + 1)
			bd.RetryCount = res.RetryCount

			bd.BackoffTime = time.Now().UTC().Add(nextRetryAt)
			batchData = append(batchData, bd)
		}

		s.ActiveWorkers.Add(-1)
		metrcs.HermesWorkersActive.Dec()

		count, err := s.Store.BatchUpdateDeliveries(ctx, batchData, 3)
		if err != nil {
			s.Logger.Error("[hermes] Failed to execute delivery batch update query", "error", err)
			return err
		}
		s.Logger.Info("Updated deliveries table", "count", count)

	}
}

func (s *Server) StartHermes(appCtx context.Context, n int) error {
	count, err := s.Store.RecoverStaleJobs(appCtx, s.Config.Worker.StaleRecoverytimeout)
	if err != nil {
		s.Logger.Error("[hermes] Failed to recover stale jobs", "error", err)
		return err
	}
	s.Logger.Info("[hermes] Recovered stale jobs", "count", count)
	s.Logger.Info("[worker] Starting Hermes cluster")

	for range n {
		s.WG.Go(func() {
			err := s.Hermes(appCtx)
			if err != nil {
				s.Logger.Error("[hermes] Hermes worker failed", "error", err)
				return
			}
		})
	}
	return nil
}

func ReconstructRequest(ctx context.Context, record Delivery) (*http.Request, error) {
	var meta EventMeta
	if err := json.Unmarshal(record.meta, &meta); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event meta: %w", err)
	}

	parsedURL, err := url.Parse(record.Url)
	if err != nil {
		return nil, fmt.Errorf("invalid target downstream URL: %w", err)
	}

	// Attach stored query parameters to the target URL
	if len(meta.QueryParams) > 0 {
		query := parsedURL.Query()
		for key, values := range meta.QueryParams {
			for _, val := range values {
				query.Add(key, val)
			}
		}
		parsedURL.RawQuery = query.Encode()
	}

	method := meta.Method
	if method == "" {
		method = http.MethodPost
	}

	req, err := http.NewRequestWithContext(ctx, method, parsedURL.String(), bytes.NewReader(record.Payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create outgoing request: %w", err)
	}

	for key, value := range meta.Headers {
		req.Header.Set(key, value)
	}

	return req, nil
}
