package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello World!"))
	})
	// r.Post("/ingress", internal.handleIngress)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	srvr := &http.Server{
		Addr:    ":3000",
		Handler: r,
	}

	slog.Info("server starting",
		"addr", srvr.Addr,
		"url", "http://localhost"+srvr.Addr,
	)

	if err := srvr.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("could not start server ", "error: ", err, "addr: ", srvr.Addr)
		os.Exit(1)
	}

}
