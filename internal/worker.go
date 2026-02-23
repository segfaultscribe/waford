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
						// Safely abort. Can write to DLQ here, but I'm too lazy to do that
						return
					case <-time.After(d):
						s.JM.RetryBuffer <- j
					}
				}(current, delay)
			}
		}

	}
}

// 	for current := range s.JM.JobBuffer {
// 		// we are guarenteed that the work that arrives in this buffer is a
// 		// fresh job so we don't have to be bothered by RetryCount

// 		// create context
// 		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

// 		//send the request
// 		err := sendRequest(ctx, current.EventID, current.Destination, current.Payload)
// 		cancel()

// 		if err != nil {
// 			current.LastError = err.Error()
// 			delay := expBackoff(current.RetryCount)

// 			// non blocking wait
// 			time.AfterFunc(delay, func() {
// 				s.JM.RetryBuffer <- current
// 			})
// 		}
// 	}
// }

func (s *Server) handleRetries(ctx context.Context) {
	defer s.WG.Done()

	for {
		select {
		case <-ctx.Done():
			return

		case current := <-s.JM.RetryBuffer:
			if current.RetryCount > 3 {
				s.JM.DLQBuffer <- current
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
					}
				}(current, delay)

			}
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

	// separate fleet just for retries so they
	// don't block fresh traffic
	for i := 0; i < numRetryWorkers; i++ {
		s.WG.Add(1)
		go s.handleRetries(appCtx)
	}

	// DLQ workers: this would be just 1 but it's in a loop because,
	// I might update the file reading to be mutex Locked  so that multiple writers can safely access it
	// right now, as you might have guessed that's not the case.
	for i := 0; i < numDLQWorkers; i++ {
		s.WG.Add(1)
		go s.handleDLQ(appCtx)
	}

	fmt.Printf("Started %d fresh workers; %d retry workers; %d DLQ workers\n", numFreshWorkers, numRetryWorkers, numDLQWorkers)
}
