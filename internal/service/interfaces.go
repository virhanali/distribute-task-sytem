package service

import (
	"context"

	"mirage-boilerplate/internal/domain"
)

type HealthService interface {
	CheckHealth(ctx context.Context) domain.HealthStatus
}
