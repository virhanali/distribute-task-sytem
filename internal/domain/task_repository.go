package domain

import (
	"context"

	"github.com/google/uuid"
)

// TaskRepository defines the interface for task persistence
type TaskRepository interface {
	// Create persists a new task
	Create(ctx context.Context, task *Task) error

	// BulkInsert persists multiple tasks efficiently
	BulkInsert(ctx context.Context, tasks []*Task) error

	// FindByID retrieves a task by its ID
	FindByID(ctx context.Context, id uuid.UUID) (*Task, error)

	// UpdateStatus updates the status of a specific task
	// Useful for state transitions like Queued -> Processing -> Completed/Failed
	UpdateStatus(ctx context.Context, id uuid.UUID, status TaskStatus) error

	// FindPendingTasks retrieves tasks that are ready to be processed
	// This includes Queued tasks (respecting ScheduledAt) and potentially retriable Failed tasks
	FindPendingTasks(ctx context.Context, limit int) ([]*Task, error)

	// GetMetrics returns aggregated task statistics
	GetMetrics(ctx context.Context) (*TaskMetrics, error)
}
