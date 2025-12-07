package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type TaskStatus string

const (
	TaskStatusQueued     TaskStatus = "queued"
	TaskStatusProcessing TaskStatus = "processing"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
	TaskStatusCancelled  TaskStatus = "cancelled"
)

type TaskPriority string

const (
	TaskPriorityHigh   TaskPriority = "high"
	TaskPriorityMedium TaskPriority = "medium"
	TaskPriorityLow    TaskPriority = "low"
)

type Task struct {
	ID           uuid.UUID       `json:"id" db:"id"`
	Name         string          `json:"name" db:"name"`
	Payload      json.RawMessage `json:"payload" db:"payload"`
	Priority     TaskPriority    `json:"priority" db:"priority"`
	Status       TaskStatus      `json:"status" db:"status"`
	ScheduledAt  *time.Time      `json:"scheduled_at,omitempty" db:"scheduled_at"`
	StartedAt    *time.Time      `json:"started_at,omitempty" db:"started_at"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty" db:"completed_at"`
	RetryCount   int             `json:"retry_count" db:"retry_count"`
	MaxRetry     int             `json:"max_retry" db:"max_retry"`
	ErrorMessage *string         `json:"error_message,omitempty" db:"error_message"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at" db:"updated_at"`
}

func NewTask(name string, payload json.RawMessage, priority TaskPriority, scheduledAt *time.Time) (*Task, error) {
	if name == "" {
		return nil, ErrInvalidTaskType
	}
	if len(payload) == 0 {
		return nil, ErrEmptyPayload
	}

	validPriorities := map[TaskPriority]bool{
		TaskPriorityHigh:   true,
		TaskPriorityMedium: true,
		TaskPriorityLow:    true,
	}
	if !validPriorities[priority] {
		return nil, ErrInvalidPriority
	}

	return &Task{
		ID:          uuid.New(),
		Name:        name,
		Payload:     payload,
		Priority:    priority,
		Status:      TaskStatusQueued,
		ScheduledAt: scheduledAt,
		MaxRetry:    3, // Default max retry, can be configured
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}, nil
}

func (t *Task) IsRetryable() bool {
	return t.RetryCount < t.MaxRetry
}

func (t *Task) CanExecute() bool {
	if t.Status != TaskStatusQueued {
		return false
	}
	if t.ScheduledAt != nil && time.Now().Before(*t.ScheduledAt) {
		return false
	}
	return true
}

func (t *Task) MarkAsProcessing() error {
	if t.Status != TaskStatusQueued && t.Status != TaskStatusFailed {
		return ErrInvalidTaskStatus
	}

	now := time.Now()
	t.Status = TaskStatusProcessing
	t.StartedAt = &now
	t.UpdatedAt = now
	return nil
}

func (t *Task) MarkAsCompleted() error {
	if t.Status != TaskStatusProcessing {
		return ErrInvalidTaskStatus
	}

	now := time.Now()
	t.Status = TaskStatusCompleted
	t.CompletedAt = &now
	t.UpdatedAt = now
	return nil
}

func (t *Task) MarkAsFailed(errMsg string) error {
	if t.Status != TaskStatusProcessing {
		return ErrInvalidTaskStatus
	}

	now := time.Now()
	t.RetryCount++
	t.ErrorMessage = &errMsg
	t.UpdatedAt = now

	if t.IsRetryable() {
		t.Status = TaskStatusFailed
	} else {
		t.Status = TaskStatusFailed
	}

	return nil
}

func (t *Task) MarkAsCancelled() error {
	if t.Status == TaskStatusCompleted {
		return ErrInvalidTaskStatus
	}

	now := time.Now()
	t.Status = TaskStatusCancelled
	t.UpdatedAt = now
	return nil
}
