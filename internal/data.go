package internal

import (
	"context"
	"fmt"
	"time"
)

func (s *Server) getDashboardData() (*DashboardData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	TotalEvents, err := s.Store.GetTotalEvents(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch events count %w", err)
	}

	DStats, err := s.Store.GetDeliveryStat(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch delivery stats %w", err)
	}

	successRate := SuccessRate(DStats)

	dashboardData := &DashboardData{
		EventsReceived:       TotalEvents,
		Deliveries:           DStats.Total,
		SuccessfulDeliveries: DStats.Successful,
		QueueDepth:           DStats.Pending + DStats.Retry,
		DeadDeliveries:       DStats.Dead,
		SuccessRate:          successRate,
		ActiveWorkers:        s.ActiveWorkers.Load(),
		TotalWorkers:         int32(s.Config.Worker.Count),
	}

	return dashboardData, nil
}

func (s *Server) getDistributorData(ctx context.Context) (*DistributorView, error) {
	return &DistributorView{}, nil
}

func (s *Server) GetEventDetailPageData(
	ctx context.Context,
	eventID string,
) (EventDetailPageData, error) {
	event, err := s.Store.GetEventDetail(ctx, eventID)
	if err != nil {
		return EventDetailPageData{}, err
	}

	deliveries, err := s.Store.GetDeliveriesByEventID(ctx, eventID)
	if err != nil {
		return EventDetailPageData{}, err
	}

	return EventDetailPageData{
		Title:      "Event Details",
		Event:      event,
		Deliveries: deliveries,
	}, nil
}

func SuccessRate(stats *DeliveryStats) float64 {
	completed := stats.Successful + stats.Dead

	if completed == 0 {
		return 0
	}

	return float64(stats.Successful) / float64(completed) * 100
}

func deriveEventStatus(
	fannedOut bool,
	total int,
	successful int,
	failed int,
) string {
	if !fannedOut {
		return "processing"
	}

	if total == 0 {
		return "pending"
	}

	if successful == total {
		return "success"
	}

	if failed == total {
		return "failed"
	}

	if failed > 0 && successful > 0 &&
		successful+failed == total {
		return "partial_failure"
	}

	return "processing"
}
