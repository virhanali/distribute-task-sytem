package domain

import (
	"context"

	"github.com/google/uuid"
)

type TaskRepository interface {
	Create(ctx context.Context, task *Task) error

	BulkInsert(ctx context.Context, tasks []*Task) error

	FindByID(ctx context.Context, id uuid.UUID) (*Task, error)

	UpdateStatus(ctx context.Context, id uuid.UUID, status TaskStatus) error

	FindPendingTasks(ctx context.Context, limit int) ([]*Task, error)

	GetMetrics(ctx context.Context) (*TaskMetrics, error)
}
