package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/segfaultscribe/waford/internal"
)

func main() {
	// initialize logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// create the server
	app := internal.CreateServer(100, logger)

	// start all workers before server goes up
	app.StartWorkers(100, 10, 1)

	srvr := &http.Server{
		Addr:    ":3000",
		Handler: app.Router,
	}

	slog.Info("server starting",
		"addr", srvr.Addr,
		"url", "http://localhost"+srvr.Addr,
	)

	// Starting the server in a BACKGROUND goroutine
	go func() {
		slog.Info("server starting", "addr", srvr.Addr)
		if err := srvr.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("could not start server", "error", err)
			os.Exit(1)
		}
	}()

	// Channel to listen for OS signals for graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Block the main thread here until a signal is received
	<-stop
	slog.Info("Shutdown signal received. Commencing graceful shutdown...")

	// Give the HTTP time to finish sending 202 Accepted to active clients; current: 5 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srvr.Shutdown(ctx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}

	// Close the channels; break loop
	close(app.JM.JobBuffer)
	close(app.JM.RetryBuffer)
	close(app.JM.DLQBuffer)

	slog.Info("Waiting for background workers to finish...")
	app.WG.Wait()

	slog.Info("Graceful shutdown complete. Goodbye!")

}
