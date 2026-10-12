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

func SuccessRate(stats *DeliveryStats) float64 {
	completed := stats.Successful + stats.Dead

	if completed == 0 {
		return 0
	}

	return float64(stats.Successful) / float64(completed) * 100
}
