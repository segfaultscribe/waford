package internal

import "time"

var Destinations = []string{
	"http://localhost:4000/here",
	"http://localhost:4000/here",
	"http://localhost:4000/here",
}

const (
	BaseDelay = 1 * time.Second
	MaxDelay  = 5 * time.Minute
)
