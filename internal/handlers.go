package internal

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/google/uuid"
)

func (s *Server) handleIngress(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)

	if !json.Valid(body) || err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	incomingEventId := uuid.New().String()

	for _, dest := range Destinations {

		// jobId := uuid.New().String()
		newJob := Job{
			EventID:     incomingEventId,
			Payload:     body,
			RetryCount:  0,
			Destination: dest,
		}

		select {
		case s.JM.JobBuffer <- newJob:
		default:
			// The channel is 100% full. SHED THE LOAD!
			// instantly reject the request so the client can backoff
			s.Logger.Warn("System at capacity, shedding load", "event_id", incomingEventId)
			http.Error(w, "server is at capacity, please retry later", http.StatusTooManyRequests)
			return
		}

		s.Logger.Info("[Job] New Job registered JobId:", "jobId", incomingEventId)
	}

	w.WriteHeader(http.StatusAccepted)
}
