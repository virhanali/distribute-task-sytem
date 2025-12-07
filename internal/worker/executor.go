package worker

import (
	"context"
	"distribute-task-sytem/internal/domain"
)

type TaskExecutor interface {
	Execute(ctx context.Context, task *domain.Task) error
}

type HandlerFunc func(ctx context.Context, task *domain.Task) error

type SimpleTaskExecutor struct {
	handlers map[string]HandlerFunc
}

func NewSimpleTaskExecutor() *SimpleTaskExecutor {
	return &SimpleTaskExecutor{
		handlers: make(map[string]HandlerFunc),
	}
}

func (e *SimpleTaskExecutor) Register(name string, handler HandlerFunc) {
	e.handlers[name] = handler
}

func (e *SimpleTaskExecutor) Execute(ctx context.Context, task *domain.Task) error {
	handler, exists := e.handlers[task.Name]
	if !exists {
		return domain.ErrInvalidTaskType
	}
	return handler(ctx, task)
}
