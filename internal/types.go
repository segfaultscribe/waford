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
	ID            string    `db:"id"`
	DistributorID string    `db:"distributor_id"`
	Payload       string    `db:"payload"` // Or []byte if you prefer
	CreatedAt     time.Time `db:"created_at"`
	FannedOut     bool      `db:"fanned_out"`
	URL           string    `db:"url"` // The destination URL for the event
}
