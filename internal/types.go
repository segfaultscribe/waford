package internal

import "encoding/json"

type Job struct {
	EventID     string          `json:"event_id"`
	BatchId     string          `json:"batch_id"`
	Payload     json.RawMessage `json:"payload"`
	RetryCount  int             `json:"retry_count"`
	Destination string          `json:"destination"`
	LastError   string          `json:"last_error,omitempty"`
}
