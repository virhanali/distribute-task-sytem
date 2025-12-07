package domain

import "errors"

var (
	ErrTaskNotFound      = errors.New("task not found")
	ErrTaskAlreadyExists = errors.New("task already exists")
	ErrInvalidTaskStatus = errors.New("invalid task status transition")
	ErrTaskMaxRetries    = errors.New("task has reached max retries")
	ErrTaskNotRetryable  = errors.New("task is not retryable")
	ErrInvalidPriority   = errors.New("invalid priority level")
	ErrInvalidTaskType   = errors.New("invalid task type")
	ErrEmptyPayload      = errors.New("task payload cannot be empty")
)
