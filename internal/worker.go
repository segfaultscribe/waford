package internal

import (
	"bytes"
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

func sendRequest(eventId string, url string, body json.RawMessage) error {
	req, err := http.NewRequest(
		"POST",
		url,
		bytes.NewReader(body),
	)

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
	for current := range s.jm.JobBuffer {
		// we are guarenteed that the work that arrives in this buffer is a
		// fresh job so we don't have to be bothered by RetryCount

		//send the request
		if err := sendRequest(current.EventID, current.Destination, current.Payload); err != nil {
			current.LastError = err.Error()
			// add this job to the retry after upping the count
			delay := expBackoff(current.RetryCount)
			// 3. Fire-and-forget the requeue so this worker isn't blocked waiting
			time.AfterFunc(delay, func() {
				s.jm.RetryBuffer <- current
			})
		}
	}
}

func (s *Server) handleRetries() {
	for current := range s.jm.RetryBuffer {

		if current.RetryCount > 3 {
			s.jm.DLQBuffer <- current
			continue
		}

		current.RetryCount++

		if err := sendRequest(current.EventID, current.Destination, current.Payload); err != nil {
			current.LastError = err.Error()
			// add this job to the retry after upping the count
			delay := expBackoff(current.RetryCount)
			// 3. Fire-and-forget the requeue so this worker isn't blocked waiting
			time.AfterFunc(delay, func() {
				s.jm.RetryBuffer <- current
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
		go s.handleFreshJob()
	}

	// Spin up a smaller, separate fleet just for retries so they
	// don't block fresh traffic
	for i := 0; i < numRetryWorkers; i++ {
		go s.handleRetries()
	}

	for i := 0; i < numDLQWorkers; i++ {
		go s.handleDLQ()
	}

	fmt.Printf("Started %d fresh workers; %d retry workers; %d DLQ workers\n", numFreshWorkers, numRetryWorkers, numDLQWorkers)
}
