package internal

import "time"

var Destinations = []string{
	"http://localhost:4000/dest",
	"http://localhost:4000/dest",
	"http://localhost:4000/dest",
}

const (
	BaseDelay = 1 * time.Second
	MaxDelay  = 5 * time.Minute
)
