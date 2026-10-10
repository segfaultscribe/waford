package internal

import (
	"encoding/json"
	"net/http"
	"time"
)

type Job struct {
	EventID     string          `json:"event_id"`
	Payload     json.RawMessage `json:"payload"`
	RetryCount  int             `json:"retry_count"`
	Destination string          `json:"destination"`
	LastError   string          `json:"last_error,omitempty"`
}

type Distributor struct {
	Name         string   `json:"name"`
	Destinations []string `json:"destinations"`
}

func (d *Distributor) decodeJSON(r *http.Request, maxBytes int64) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxBytes)
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	return decoder.Decode(d)
}

type Event struct {
	ID            string `db:"id"`
	Payload       []byte `db:"payload"`
	DestinationID string `db:"destination_id"`
}

type Delivery struct {
	ID         string `db:"id"`
	Url        string `db:"url"`
	Payload    []byte `db:"payload"`
	retryCount int    `db:"retry_count"`
	meta       []byte `db:"meta"`
}

type DeliverResult struct {
	ID         string
	Success    bool
	Err        error
	RetryCount int
}

type DeliveryBatchData struct {
	ID          string
	Success     bool
	ErrMessage  string
	BackoffTime time.Time
	RetryCount  int
}

type EventMeta struct {
	Method      string              `json:"method"`
	Path        string              `json:"path"`
	QueryParams map[string][]string `json:"query_params"`
	Headers     map[string]string   `json:"headers"`
}

var (
	defaultHeaderPrefixes = []string{"x-", "webhook-", "stripe-", "github-"}
	defaultHeaderKeys     = map[string]struct{}{
		"content-type":  {},
		"traceparent":   {},
		"authorization": {},
	}
)
