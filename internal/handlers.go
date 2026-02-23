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

	for _, dest := range Destinations {
		newJob := Job{
			EventID:     uuid.New().String(),
			Payload:     body,
			RetryCount:  0,
			Destination: dest,
		}

		s.JM.JobBuffer <- newJob
	}

	w.WriteHeader(http.StatusAccepted)
}
