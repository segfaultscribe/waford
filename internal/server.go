package internal

import (
	"log/slog"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type JobManager struct {
	JobBuffer     chan Job
	RetryBuffer   chan Job
	DLQBuffer     chan Job
	ChronosSignal chan struct{}
	HermesSignal  chan struct{}
}

type Server struct {
	Router *chi.Mux
	JM     *JobManager // The buffer lives here!
	Logger *slog.Logger
	WG     *sync.WaitGroup
	Store  *Store
	Config *Config
}

func CreateServer(bufferSize int, lg *slog.Logger, store *Store, config *Config) *Server {
	jm := &JobManager{
		JobBuffer:     make(chan Job, bufferSize),
		RetryBuffer:   make(chan Job, bufferSize),
		DLQBuffer:     make(chan Job, bufferSize),
		ChronosSignal: make(chan struct{}),
		HermesSignal:  make(chan struct{}),
	}

	s := &Server{
		Router: chi.NewRouter(),
		JM:     jm,
		Logger: lg,
		WG:     &sync.WaitGroup{},
		Store:  store,
		Config: config,
	}

	s.Router.Use(middleware.Logger)
	s.routes()
	return s
}

func (s *Server) routes() {
	s.Router.Post("/distributors", s.createDistributorHandler)
	s.Router.Post("/distributors/{distributorID}/events", s.handleRegisterEvent)
	s.Router.Get("/distributors/{id}", s.getDistributorInfo)
	s.Router.Delete("/distributors/{id}", s.deleteDistributor)
	s.Router.Handle("/metrics", promhttp.Handler())
}
