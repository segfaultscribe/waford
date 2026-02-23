package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/segfaultscribe/waford/internal"
)

type Server struct {
	Router    *chi.Mux
	JobBuffer chan internal.Job // The buffer lives here!
}

func main() {
	app := internal.CreateServer(100)

	srvr := &http.Server{
		Addr:    ":3000",
		Handler: app.Router,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	slog.SetDefault(logger)

	slog.Info("server starting",
		"addr", srvr.Addr,
		"url", "http://localhost"+srvr.Addr,
	)

	if err := srvr.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("could not start server ", "error: ", err, "addr: ", srvr.Addr)
		os.Exit(1)
	}

}
