package main

import (
	"log"
	"math/rand"
	"net/http"
	"time"
)

func chaosHandler(w http.ResponseWriter, r *http.Request) {
	// random number to control probability of response type
	chance := rand.Intn(100) + 1

	log.Printf("Received request from forwarder. Rolling the dice: %d", chance)

	// The Happy Path (33% chance)
	// return an http.StatusOK (200)
	if chance >= 1 && chance <= 33 {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// The Flaky Target (33% chance)
	// Return an http.StatusInternalServerError (500)
	if chance > 33 && chance <= 66 {
		http.Error(w, "Chaos Server Error!", http.StatusInternalServerError)
		return
	}

	// The Tar Pit (34% chance)
	// Force goroutine to sleep for 6 seconds.
	// Webhook Forwarder has a 5-second context timeout,
	// so our forwarder should aggressively sever this connection before the sleep finishes
	if chance > 66 && chance <= 100 {
		time.Sleep(6 * time.Second)
		w.WriteHeader(http.StatusAccepted)
		return
	}
}

func main() {
	http.HandleFunc("/dest", chaosHandler)
	log.Println("😈 Chaos Server listening on :4000/dest")
	log.Fatal(http.ListenAndServe(":4000", nil))
}
