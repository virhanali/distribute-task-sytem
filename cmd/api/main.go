package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mirage-boilerplate/internal/config"
	"mirage-boilerplate/internal/database"
	"mirage-boilerplate/internal/handler/health"
	"mirage-boilerplate/internal/middleware"
	"mirage-boilerplate/internal/repository"
	"mirage-boilerplate/internal/service"
)

func main() {
	cfg := config.Load()

	setupLogger(cfg.App.Env)

	db, err := database.NewPostgresDB(cfg.Database)
	if err != nil {
		slog.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Dependency Injection
	healthRepo := repository.NewHealthRepository(db)

	healthService := service.NewHealthService(healthRepo)

	healthHandler := health.NewHealthHandler(healthService)

	mux := http.NewServeMux()

	// Register Routes
	mux.HandleFunc("GET /health", healthHandler.Check)
	mux.HandleFunc("GET /api/v1/health", healthHandler.Check)

	handler := middleware.Logger(mux)

	server := &http.Server{
		Addr:         ":" + cfg.App.Port,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("Starting server", "port", cfg.App.Port, "env", cfg.App.Env)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	slog.Info("Server is shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("Server exited properly")
}

func setupLogger(env string) {
	var handler slog.Handler
	if env == "production" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	}
	slog.SetDefault(slog.New(handler))
}
