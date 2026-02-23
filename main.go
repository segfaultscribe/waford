package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/segfaultscribe/waford/internal"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	app := internal.CreateServer(100, logger)
	app.StartWorkers(100, 10, 1)

	srvr := &http.Server{
		Addr:    ":3000",
		Handler: app.Router,
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
