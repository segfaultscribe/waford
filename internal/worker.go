package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"time"
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

func (s *Server) handleFreshJob(ctx context.Context) {
	defer s.WG.Done()

	for {
		select {
		case <-ctx.Done():
			return

		case current := <-s.JM.JobBuffer:
			// create context
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

			//send the request
			err := sendRequest(ctx, current.EventID, current.Destination, current.Payload)
			cancel()

			if err != nil {
				current.LastError = err.Error()
				delay := expBackoff(current.RetryCount)

				go func(j Job, d time.Duration) {
					select {
					case <-ctx.Done():
						// server shuting down
						// Safely abort.
						// Can write to DLQ here, but I'm too lazy
						return
					case <-time.After(d):
						s.JM.RetryBuffer <- j
						s.Logger.Warn(
							"[worker] Job moved to retry buffer",
							"jobId", current.EventID,
							"retry count", current.RetryCount,
						)
					}
				}(current, delay)
				continue
			}
			s.Logger.Info(
				"[worker] Fresh Job delivered successfully",
				"job_id", current.EventID,
				"destination", current.Destination,
			)
		}

	}
}

func (s *Server) handleRetries(ctx context.Context) {
	defer s.WG.Done()

	for {
		select {
		case <-ctx.Done():
			return

		case current := <-s.JM.RetryBuffer:
			if current.RetryCount > 3 {
				s.JM.DLQBuffer <- current
				s.Logger.Warn(
					"[worker] Job moved to DLQ Buffer",
					"jobId", current.EventID,
					"retry count", current.RetryCount,
				)
				continue
			}

			current.RetryCount++

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

			err := sendRequest(ctx, current.EventID, current.Destination, current.Payload)
			cancel()

			if err != nil {
				current.LastError = err.Error()
				delay := expBackoff(current.RetryCount)

				go func(j Job, d time.Duration) {
					select {
					case <-ctx.Done():
						return
					case <-time.After(d):
						s.JM.RetryBuffer <- j
						s.Logger.Warn(
							"[worker] Adding job to retry buffer",
							"jobId", j.EventID,
							"retry count", j.RetryCount,
						)
					}
				}(current, delay)
				continue
			}
			s.Logger.Info(
				"[worker] Retried Job delivered successfully",
				"job_id", current.EventID,
				"destination", current.Destination,
				"retry count", current.RetryCount,
			)
		}
	}

}

func expBackoff(count int) time.Duration {
	// Calculate the raw exponential backoff
	multiplier := math.Pow(2, float64(count))
	backoff := float64(BaseDelay) * multiplier
	// Cap the backoff to the maximum allowed delay
	if backoff > float64(MaxDelay) {
		backoff = float64(MaxDelay)
	}
	// apply full jitter
	jitteredBackoff := backoff * rand.Float64()
	return time.Duration(jitteredBackoff)
}

func (s *Server) StartWorkers(appCtx context.Context, numFreshWorkers int, numRetryWorkers int, numDLQWorkers int) {
	// fleet of workers for fresh incoming webhooks
	for i := 0; i < numFreshWorkers; i++ {
		s.WG.Add(1)
		go s.handleFreshJob(appCtx)
	}
	s.Logger.Info("[worker] Started fresh job workers", "count:", numFreshWorkers)

	// separate fleet just for retries so they
	// don't block fresh traffic
	for i := 0; i < numRetryWorkers; i++ {
		s.WG.Add(1)
		go s.handleRetries(appCtx)
	}
	s.Logger.Info("[worker] Started retry workers", "count:", numRetryWorkers)

	// DLQ workers: this would be just 1 but it's in a loop because,
	// I might update the file reading to be mutex Locked  so that multiple writers can safely access it
	// right now, as you might have guessed that's not the case.
	for i := 0; i < numDLQWorkers; i++ {
		s.WG.Add(1)
		go s.handleDLQ(appCtx)
	}
	s.Logger.Info(
		"[worker] Started DLQ workers",
		"count:",
		numDLQWorkers,
	)
	s.Logger.Info("[worker] NOTE: There should be only 1 DLQ worker running due to architectural reasons")

	// fmt.Printf("Started %d fresh workers; %d retry workers; %d DLQ workers\n", numFreshWorkers, numRetryWorkers, numDLQWorkers)
}
