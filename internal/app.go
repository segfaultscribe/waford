package internal

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/segfaultscribe/waford/internal/db"
	"github.com/segfaultscribe/waford/internal/metrcs"
)

func Run(cfg *Config) error {
	metrcs.Init()

	// Initialize logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Initialize database
	database, err := db.InitDB(cfg.Database.Path)
	if err != nil {
		return err
	}
	defer database.Close()

	var version string
	if err := database.QueryRow("SELECT sqlite_version();").Scan(&version); err != nil {
		return err
	}

	slog.Info("Underlying SQLite version", "version", version)

	// Run migrations
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}

	if err := goose.Up(database, "./internal/db/migrations"); err != nil {
		return err
	}

	logger.Info("Database migrations applied successfully")

	store := NewStore(database, logger)

	// Create application
	app := CreateServer(10000, logger, store, cfg)

	appCtx, stopApp := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stopApp()

	// Start workers
	app.StartChronos(appCtx, cfg.Worker.PollInterval)
	app.StartHermes(appCtx, cfg.Worker.Count)

	// Create HTTP server
	srvr := &http.Server{
		Addr:    ":" + strconv.Itoa(cfg.Server.Port),
		Handler: app.Router,
	}

	slog.Info(
		"Server starting",
		"addr", srvr.Addr,
		"url", "http://localhost"+srvr.Addr,
	)

	go func() {
		if err := srvr.ListenAndServe(); err != nil &&
			err != http.ErrServerClosed {
			slog.Error("Server failed", "error", err)
			stopApp()
		}
	}()

	// Wait for shutdown signal
	<-appCtx.Done()

	slog.Info("Shutdown signal received")

	// Gracefully shutdown HTTP server
	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	if err := srvr.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown failed", "error", err)
	}

	// Wait for workers
	slog.Info("Waiting for background workers...")
	app.WG.Wait()

	slog.Info("Graceful shutdown complete")

	return nil
}
