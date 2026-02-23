package internal

import (
	"log/slog"

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
	jm     *JobManager // The buffer lives here!
	logger *slog.Logger
}

func CreateServer(bufferSize int, lg *slog.Logger) *Server {
	jm := &JobManager{
		JobBuffer:   make(chan Job, bufferSize),
		RetryBuffer: make(chan Job, bufferSize),
		DLQBuffer:   make(chan Job, bufferSize),
	}

	s := &Server{
		Router: chi.NewRouter(),
		jm:     jm,
		logger: lg,
	}

	s.Router.Use(middleware.Logger)
	s.routes()
	return s
}

func (s *Server) routes() {
	s.Router.Post("/ingress", s.handleIngress)
}
