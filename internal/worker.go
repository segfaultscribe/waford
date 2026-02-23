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

func (s *Server) handleFreshJob() {
	defer s.WG.Done()

	for current := range s.JM.JobBuffer {
		// we are guarenteed that the work that arrives in this buffer is a
		// fresh job so we don't have to be bothered by RetryCount

		// create context
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		//send the request
		err := sendRequest(ctx, current.EventID, current.Destination, current.Payload)
		cancel()

		if err != nil {
			current.LastError = err.Error()
			// add this job to the retry after upping the count
			delay := expBackoff(current.RetryCount)
			// 3. Fire-and-forget the requeue so this worker isn't blocked waiting
			time.AfterFunc(delay, func() {
				s.JM.RetryBuffer <- current
			})
		}
	}
}

func (s *Server) handleRetries() {
	defer s.WG.Done()

	for current := range s.JM.RetryBuffer {

		if current.RetryCount > 3 {
			s.JM.DLQBuffer <- current
			continue
		}

		current.RetryCount++

		// create context
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		err := sendRequest(ctx, current.EventID, current.Destination, current.Payload)
		cancel()

		if err != nil {
			current.LastError = err.Error()
			// add this job to the retry after upping the count
			delay := expBackoff(current.RetryCount)
			// 3. Fire-and-forget the requeue so this worker isn't blocked waiting
			time.AfterFunc(delay, func() {
				s.JM.RetryBuffer <- current
			})
		}
	}
}

func expBackoff(count int) time.Duration {
	// Calculate the raw exponential backoff: Base * (2 ^ retryCount)
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

func (s *Server) StartWorkers(numFreshWorkers int, numRetryWorkers int, numDLQWorkers int) {
	// Spin up a fleet of workers for fresh incoming webhooks
	for i := 0; i < numFreshWorkers; i++ {
		s.WG.Add(1)
		go s.handleFreshJob()
	}

	// Spin up a smaller, separate fleet just for retries so they
	// don't block fresh traffic
	for i := 0; i < numRetryWorkers; i++ {
		s.WG.Add(1)
		go s.handleRetries()
	}

	for i := 0; i < numDLQWorkers; i++ {
		s.WG.Add(1)
		go s.handleDLQ()
	}

	fmt.Printf("Started %d fresh workers; %d retry workers; %d DLQ workers\n", numFreshWorkers, numRetryWorkers, numDLQWorkers)
}
