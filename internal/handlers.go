package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

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

type handlers struct {
	store Store
}

func NewHandlers(store Store) *handlers {
	return &handlers{store: store}
}

func (s *Server) createDistributorHandler(w http.ResponseWriter, r *http.Request) {
	distributor := &Distributor{}

	if err := distributor.decodeJSON(r, 1<<20); err != nil {
		http.Error(w, "Invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	id, err := s.Store.SaveDistributor(ctx, distributor.Name)
	if err != nil {
		s.Logger.Error("[Distributor] Failed to save distributor", "error", err, "name", distributor.Name)
		http.Error(w, "Failed to save distributor", http.StatusInternalServerError)
		return
	}
	s.Logger.Info("[Distributor] New Distributor registered", "name", distributor.Name, "id", id)

	if err := s.Store.SaveDestinations(ctx, id, distributor.Destinations); err != nil {
		s.Logger.Error("[Distributor] Failed to save destinations", "error", err, "name", distributor.Name)
		http.Error(w, "Failed to save destinations", http.StatusInternalServerError)
		return
	}
	s.Logger.Info("[Distributor] New Destinations registered", "name", distributor.Name, "count", len(distributor.Destinations))

	s.writeJSON(w, http.StatusCreated, map[string]any{
		"message":        "Distributor registered successfully",
		"distributor_id": id,
		"webhook_url":    fmt.Sprintf("https://waford.com/waford/%s/events", id),
	})
}

func (s *Server) handleRegisterEvent(w http.ResponseWriter, r *http.Request) {
	distributorID := r.PathValue("distributorID")

	if distributorID == "" {
		http.Error(w, "Missing distributor ID", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	defer r.Body.Close()

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	id, err := s.Store.SaveEvent(r.Context(), distributorID, payload)
	if err != nil {
		s.Logger.Error("[Event] Failed to save event", "error", err, "distributorID", distributorID)
		http.Error(w, "Failed to save event", http.StatusInternalServerError)
		return
	}
	// send a signal to wake up chronos
	select {
	case s.JM.ChronosSignal <- struct{}{}:
	default:
	}

	s.writeJSON(w, http.StatusAccepted, map[string]any{
		"message": "Event Created",
		"eventID": id,
	})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.Logger.Error("Failed to encode JSON response", "error", err)
	}
}
