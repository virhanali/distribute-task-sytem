package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"distribute-task-sytem/internal/config"
	"distribute-task-sytem/internal/handler"
	"distribute-task-sytem/internal/queue"
	"distribute-task-sytem/internal/repository"
	"distribute-task-sytem/internal/service"

	_ "github.com/jackc/pgx/v5/stdlib" // Import pgx driver for sqlx
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
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

	if err := db.Ping(); err != nil {
		log.Fatal().Err(err).Msg("failed to ping database")
	}
	log.Info().Msg("connected to database")

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatal().Err(err).Msg("failed to connect to redis")
	}
	defer rdb.Close()
	log.Info().Msg("connected to redis")

	publisher, err := queue.NewRabbitMQPublisher(cfg.RabbitMQ.URL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to setup rabbitmq publisher")
	}
	defer publisher.Close()
	log.Info().Msg("connected to rabbitmq publisher")

	taskRepo := repository.NewTaskRepository(db)
	redisQueue := queue.NewRedisQueue(rdb)

	taskService := service.NewTaskService(taskRepo, redisQueue, publisher, log.Logger)
	taskHandler := handler.NewTaskHandler(taskService, log.Logger)

	router := handler.SetupRouter(handler.RouterConfig{
		TaskHandler: taskHandler,
		Logger:      log.Logger,
	})

	serverAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:    serverAddr,
		Handler: router,
	}

	go func() {
		log.Info().Str("addr", serverAddr).Msg("starting http server")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("failed to start server")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("server forced to shutdown")
	}

	log.Info().Msg("server exited")
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
