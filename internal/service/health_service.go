package service

import (
	"context"
	"fmt"
	"mirage-boilerplate/internal/domain"
	"mirage-boilerplate/internal/repository"
)

type healthService struct {
	repo repository.HealthRepository
}

func NewHealthService(repo repository.HealthRepository) HealthService {
	return &healthService{
		repo: repo,
	}
}

func (s *healthService) CheckHealth(ctx context.Context) domain.HealthStatus {
	status := domain.HealthStatus{
		System: "operational",
	}

	err := s.repo.PingDB(ctx)
	if err != nil {
		status.Database = fmt.Sprintf("down: %v", err)
	} else {
		status.Database = "up"
	}

	return status
}
