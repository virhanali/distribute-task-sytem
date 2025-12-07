package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"distribute-task-sytem/internal/config"
	"distribute-task-sytem/internal/executor"
	"distribute-task-sytem/internal/repository"
	"distribute-task-sytem/internal/worker"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	setLogLevel(cfg.Log.Level)

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.DBName,
		cfg.Database.SSLMode,
	)

	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer db.Close()
	log.Info().Msg("connected to database")

	taskRepo := repository.NewTaskRepository(db)

	execRegistry := executor.NewExecutorRegistry(log.Logger)

	workerConfig := worker.WorkerConfig{
		NumWorkers:  cfg.Worker.NumWorkers,
		RabbitMQURL: cfg.RabbitMQ.URL,
		QueueNames:  cfg.Worker.Queues,
	}

	pool := worker.NewWorkerPool(
		workerConfig,
		taskRepo,
		execRegistry, // registry implements execute(ctx, task)
		log.Logger,
	)

	if err := pool.Start(); err != nil {
		log.Fatal().Err(err).Msg("failed to start worker pool")
	}
	log.Info().Int("workers", cfg.Worker.NumWorkers).Msg("worker pool started")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("shutting down worker pool...")

	pool.Stop()

	pool.Wait()

	log.Info().Msg("worker ended")
}

func setLogLevel(level string) {
	switch level {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}
