package metrcs

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	EventsReceived = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "waford_events_received_total",
			Help: "Total number of events received.",
		},
	)

	EventsFannedOut = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "waford_events_fanned_out_total",
			Help: "Total number of events successfully fanned out.",
		},
	)

	DeliveriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "waford_deliveries_total",
			Help: "Total number of delivery attempts by outcome.",
		},
		[]string{"status"},
	)

	DeliveryDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name: "waford_delivery_duration_seconds",
			Help: "Time taken for a delivery attempt.",
		},
	)

	QueueDepth = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "waford_queue_depth",
			Help: "Current number of pending and retry deliveries.",
		},
	)

	DLQDepth = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "waford_dlq_depth",
			Help: "Current number of dead deliveries.",
		},
	)

	HermesWorkersActive = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "waford_hermes_workers_active",
			Help: "Number of Hermes workers currently processing deliveries.",
		},
	)

	ChronosPollTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "waford_chronos_poll_total",
			Help: "Total number of Chronos polling cycles.",
		},
	)
)

func Init() {
	prometheus.MustRegister(DeliveryDuration)
}
