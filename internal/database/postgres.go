package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"distribute-task-sytem/internal/config"

	_ "github.com/jackc/pgx/v5/stdlib" // pgx driver
)

func NewPostgresDB(cfg config.DatabaseConfig) (*sql.DB, error) {
	db, err := sql.Open("pgx", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	slog.Info("Connected to database successfully")
	return db, nil
}
