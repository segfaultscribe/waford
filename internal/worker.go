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
	current := <-s.jm.JobBuffer
	// we are guarenteed that the work that arrives in this buffer is a
	// fresh job so we don't have to be bothered by RetryCount

	//send the request
	if err := sendRequest(current.EventID, current.Destination, current.Payload); err != nil {
		// add this job to the retry after upping the count
		delay := expBackoff(current.RetryCount)
		// 3. Fire-and-forget the requeue so this worker isn't blocked waiting
		time.AfterFunc(delay, func() {
			s.jm.RetryBuffer <- current
		})
	}
}

func (s *Server) handleRetries() {
	current := <-s.jm.RetryBuffer

	if current.RetryCount > 3 {
		return
	}

	current.RetryCount++

	if err := sendRequest(current.EventID, current.Destination, current.Payload); err != nil {
		// add this job to the retry after upping the count
		delay := expBackoff(current.RetryCount)
		// 3. Fire-and-forget the requeue so this worker isn't blocked waiting
		time.AfterFunc(delay, func() {
			s.jm.RetryBuffer <- current
		})
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
