package internal

import (
	"log/slog"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type JobManager struct {
	JobBuffer   chan Job
	RetryBuffer chan Job
	DLQBuffer   chan Job
}

type Server struct {
	Router *chi.Mux
	JM     *JobManager // The buffer lives here!
	Logger *slog.Logger
	WG     *sync.WaitGroup
	Store  *Store
}

func CreateServer(bufferSize int, lg *slog.Logger, store *Store) *Server {
	jm := &JobManager{
		JobBuffer:   make(chan Job, bufferSize),
		RetryBuffer: make(chan Job, bufferSize),
		DLQBuffer:   make(chan Job, bufferSize),
	}

	s := &Server{
		Router: chi.NewRouter(),
		JM:     jm,
		Logger: lg,
		WG:     &sync.WaitGroup{},
		Store:  store,
	}

	s.Router.Use(middleware.Logger)
	s.routes()
	return s
}

func (s *Server) routes() {
	s.Router.Post("/ingress", s.handleIngress)
	s.Router.Post("/distributors", s.createDistributorHandler)
	s.Router.Post("/distributors/{distributorID}/events", s.handleRegisterEvent)
}
