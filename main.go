package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/segfaultscribe/waford/internal"
	"github.com/segfaultscribe/waford/internal/db"
)

func main() {
	// initialize logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	database, err := db.InitDB("waford_001.db")
	if err != nil {
		slog.Error("Initialization error: %v", err)
	}
	defer database.Close()

	if err := goose.SetDialect("sqlite3"); err != nil {
		log.Fatalf("Failed to set goose dialect: %v", err)
	}

	if err := goose.Up(database, "./internal/db/migrations"); err != nil {
		log.Fatalf("Failed to run database migrations: %v", err)
	}
	logger.Info("Database migrations applied successfully")

	store := internal.NewStore(database)
	// create the server
	app := internal.CreateServer(10000, logger, store)

	appCtx, stopApp := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopApp()
	// start all workers before server goes up
	app.StartWorkers(appCtx, 1000, 200, 1)

	srvr := &http.Server{
		Addr:    ":3000",
		Handler: app.Router,
	}

	slog.Info(
		"[server] server starting",
		"addr", srvr.Addr,
		"url", "http://localhost"+srvr.Addr,
	)

	// Starting the server in a BACKGROUND goroutine
	go func() {
		// slog.Info("server starting", "addr", srvr.Addr)
		if err := srvr.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("[server] could not start server", "error", err)
			os.Exit(1)
		}
	}()

	// Block the main thread until the OS signal cancels the context
	<-appCtx.Done()
	slog.Info("[server] Shutdown signal received. Commencing graceful shutdown...")

	// Shutdown the HTTP server so no new webhooks arrive
	shutdownCtx, cancelHTTP := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelHTTP()
	srvr.Shutdown(shutdownCtx)

	// Wait for the workers to finish their current requests
	// Because appCtx was canceled, the 'select' loops in the workers will hit
	// case <-ctx.Done() and exit, eventually calling wg.Done().
	// we now handle the panic problem
	slog.Info("[server] Waiting for background workers to finish...")
	app.WG.Wait()

	slog.Info("[server] Graceful shutdown complete. Goodnight!")
}
