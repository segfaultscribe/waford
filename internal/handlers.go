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
		jobId := uuid.New().String()
		newJob := Job{
			EventID:     jobId,
			Payload:     body,
			RetryCount:  0,
			Destination: dest,
		}

		s.JM.JobBuffer <- newJob
		s.Logger.Info("[Job] New Job registered JobId:", "jobId", jobId)
	}

	w.WriteHeader(http.StatusAccepted)
}
