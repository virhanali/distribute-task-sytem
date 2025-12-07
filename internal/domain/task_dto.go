package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type CreateTaskDTO struct {
	Name        string          `json:"name" validate:"required"`
	Payload     json.RawMessage `json:"payload" validate:"required"`
	Priority    TaskPriority    `json:"priority" validate:"required,oneof=high medium low"`
	ScheduledAt *time.Time      `json:"scheduled_at,omitempty"`
}

func (d *CreateTaskDTO) Validate() error {
	if d.Name == "" {
		return ErrInvalidTaskType
	}
	if len(d.Payload) == 0 {
		return ErrEmptyPayload
	}
	switch d.Priority {
	case TaskPriorityHigh, TaskPriorityMedium, TaskPriorityLow:
		return nil
	default:
		return ErrInvalidPriority
	}
}

type CreateBulkTaskDTO struct {
	Tasks []CreateTaskDTO `json:"tasks" validate:"required,min=1"`
}

type TaskResponse struct {
	ID           uuid.UUID       `json:"id"`
	Name         string          `json:"name"`
	Payload      json.RawMessage `json:"payload"`
	Priority     TaskPriority    `json:"priority"`
	Status       TaskStatus      `json:"status"`
	ScheduledAt  *time.Time      `json:"scheduled_at,omitempty"`
	StartedAt    *time.Time      `json:"started_at,omitempty"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
	RetryCount   int             `json:"retry_count"`
	MaxRetry     int             `json:"max_retry"`
	ErrorMessage *string         `json:"error_message,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type BulkTaskResponse struct {
	SuccessCount int            `json:"success_count"`
	FailureCount int            `json:"failure_count"`
	Tasks        []TaskResponse `json:"tasks"`
	Errors       []string       `json:"errors,omitempty"`
}

type TaskMetrics struct {
	Total      int `json:"total"`
	Queued     int `json:"queued"`
	Processing int `json:"processing"`
	Completed  int `json:"completed"`
	Failed     int `json:"failed"`
	Cancelled  int `json:"cancelled"`
}

func (t *Task) ToResponse() TaskResponse {
	return TaskResponse{
		ID:           t.ID,
		Name:         t.Name,
		Payload:      t.Payload,
		Priority:     t.Priority,
		Status:       t.Status,
		ScheduledAt:  t.ScheduledAt,
		StartedAt:    t.StartedAt,
		CompletedAt:  t.CompletedAt,
		RetryCount:   t.RetryCount,
		MaxRetry:     t.MaxRetry,
		ErrorMessage: t.ErrorMessage,
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.UpdatedAt,
	}
}
