package service

import (
	"context"

	"distribute-task-sytem/internal/domain"
)

type HealthService interface {
	CheckHealth(ctx context.Context) domain.HealthStatus
}
