package internal

const (
	StatusPending   = "pending"
	StatusRetry     = "retry"
	StatusInFlight  = "in_flight"
	StatusDelivered = "delivered"
	StatusDLQ       = "dlq"
)

var HopByHopHeaders = map[string]struct{}{
	"host":              {},
	"content-length":    {},
	"transfer-encoding": {},
	"connection":        {},
	"te":                {},
	"trailer":           {},
	"upgrade":           {},
	"user-agent":        {},
	"cookie":            {},
	"accept-encoding":   {},
}
