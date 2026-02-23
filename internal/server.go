package internal

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type JobManager struct {
	JobBuffer   chan Job
	RetryBuffer chan Job
}

type Server struct {
	Router *chi.Mux
	jm     *JobManager // The buffer lives here!
}

func CreateServer(bufferSize int) *Server {
	jm := &JobManager{
		make(chan Job, bufferSize),
		make(chan Job, bufferSize),
	}
	s := &Server{
		chi.NewRouter(),
		jm,
	}

	s.Router.Use(middleware.Logger)
	s.routes()
	return s
}

func (s *Server) routes() {
	s.Router.Post("/ingress", s.handleIngress)
}
