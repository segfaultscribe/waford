package internal

import "encoding/json"

type Job struct {
	EventID    string
	Payload    json.RawMessage
	RetryCount int
}
