package executor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"distribute-task-sytem/internal/domain"

	"github.com/rs/zerolog"
)

type TaskExecutor interface {
	Execute(ctx context.Context, task *domain.Task) error
	Name() string
}

type Registry struct {
	mu        sync.RWMutex
	executors map[string]TaskExecutor
	logger    zerolog.Logger
}

func NewRegistry(logger zerolog.Logger) *Registry {
	r := &Registry{
		executors: make(map[string]TaskExecutor),
		logger:    logger,
	}
	return r
}

func (r *Registry) Register(e TaskExecutor) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.executors[e.Name()]; exists {
		r.logger.Warn().Str("name", e.Name()).Msg("executor overwritten")
	}

	r.executors[e.Name()] = e
	r.logger.Info().Str("name", e.Name()).Msg("registered task executor")
}

func (r *Registry) Execute(ctx context.Context, task *domain.Task) error {
	r.mu.RLock()
	executor, exists := r.executors[task.Name]
	r.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no executor registered for task type: %s", task.Name)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	start := time.Now()
	err := executor.Execute(timeoutCtx, task)
	duration := time.Since(start)

	logEvent := r.logger.Info()
	if err != nil {
		logEvent = r.logger.Error().Err(err)
	}

	logEvent.
		Str("task_id", task.ID.String()).
		Str("executor", executor.Name()).
		Dur("duration", duration).
		Msg("task execution finished")

	return err
}
