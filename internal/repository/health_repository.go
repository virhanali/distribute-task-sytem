package repository

import (
	"context"
	"database/sql"
	"time"
)

type healthRepository struct {
	db *sql.DB
}

func NewHealthRepository(db *sql.DB) HealthRepository {
	return &healthRepository{
		db: db,
	}
}

func (r *healthRepository) PingDB(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	return r.db.PingContext(ctx)
}
