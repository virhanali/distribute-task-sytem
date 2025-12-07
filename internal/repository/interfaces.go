package repository

import "context"

type HealthRepository interface {
	PingDB(ctx context.Context) error
}
