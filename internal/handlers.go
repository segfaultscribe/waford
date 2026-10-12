package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/segfaultscribe/waford/internal/metrcs"
	"github.com/segfaultscribe/waford/internal/templates"
)

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

	metaStruct := extractMetadata(r)

	metaBlob, err := json.Marshal(metaStruct)
	if err != nil {
		s.Logger.Error("[Event] Failed to marshal metadata", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	id, err := s.Store.SaveEvent(r.Context(), distributorID, payload, metaBlob)
	if err != nil {
		s.Logger.Error("[Event] Failed to save event", "error", err, "distributorID", distributorID)
		http.Error(w, "Failed to save event", http.StatusInternalServerError)
		return
	}
	metrcs.EventsReceived.Inc()

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

func (s *Server) getDistributorInfo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		s.Logger.Error("Received empty id")
		http.Error(w, "Missing 'id' query parameter", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	distributorInfo, err := s.Store.GetDistributors(ctx, id)
	if err != nil {
		s.Logger.Error("Failed to fetch distributor", "error", err)
		http.Error(w, "Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if distributorInfo == nil {
		http.Error(w, "Distributor not found", http.StatusNotFound)
		return
	}

	// Success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(distributorInfo); err != nil {
		s.Logger.Error("Failed to encode distributor response", "error", err)
	}
}

func (s *Server) deleteDistributor(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		s.Logger.Error("Received empty id for deletion")
		http.Error(w, "Missing 'id' query parameter", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Call repository layer
	err := s.Store.DeleteDistributors(ctx, id)
	if err != nil {
		s.Logger.Error("Failed to delete distributor", "error", err)
		http.Error(w, "Error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Success response: 204 No Content is standard for successful deletions
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.Logger.Error("Failed to encode JSON response", "error", err)
	}
}

func extractMetadata(r *http.Request) EventMeta {
	capturedHeaders := make(map[string]string)

	for key, values := range r.Header {
		if _, ok := HopByHopHeaders[strings.ToLower(key)]; ok {
			continue
		}
		capturedHeaders[http.CanonicalHeaderKey(key)] = values[0]
	}

	return EventMeta{
		Method:      r.Method,
		Path:        r.URL.Path,
		QueryParams: r.URL.Query(),
		Headers:     capturedHeaders,
	}
}

func (s *Server) DashboardHandler(w http.ResponseWriter, r *http.Request) {
	data, err := s.getDashboardData()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get dashboard data: %v", err), http.StatusInternalServerError)
	}

	templates.Pages["dashboard.html"].ExecuteTemplate(w, "layout", data)
}

func (s *Server) DashboardStatsHandler(w http.ResponseWriter, r *http.Request) {
	stats, err := s.getDashboardData()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get dashboard data: %w", err), http.StatusInternalServerError)
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(stats); err != nil {
		s.Logger.Error("failed to encode dashboard stats", "error", err)
	}
}

func (s *Server) DistributorViewHandler(w http.ResponseWriter, r *http.Request) {
	data := DistributorPageData{
		Title: "Distributors",
		Distributors: []DistributorView{
			{
				ID:   "dist_123",
				Name: "Payments",
				Destinations: []DestinationView{
					{URL: "https://service-a.com/webhook"},
					{URL: "https://service-b.com/webhook"},
				},
			},
		},
	}

	templates.Pages["distributors.html"].ExecuteTemplate(w, "layout", data)
}
