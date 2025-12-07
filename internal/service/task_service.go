package service

import (
	"context"
	"time"

	"distribute-task-sytem/internal/domain"
	"distribute-task-sytem/internal/queue" // Assuming queue package is in internal/queue

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type TaskService struct {
	repo      domain.TaskRepository
	queue     *queue.RedisQueue
	publisher *queue.RabbitMQPublisher
	logger    zerolog.Logger
}

func NewTaskService(
	repo domain.TaskRepository,
	queue *queue.RedisQueue,
	publisher *queue.RabbitMQPublisher,
	logger zerolog.Logger,
) *TaskService {
	return &TaskService{
		repo:      repo,
		queue:     queue,
		publisher: publisher,
		logger:    logger,
	}
}

func (s *TaskService) Create(ctx context.Context, dto domain.CreateTaskDTO) (*domain.Task, error) {
	if err := dto.Validate(); err != nil {
		return nil, err
	}

	task, err := domain.NewTask(dto.Name, dto.Payload, dto.Priority, dto.ScheduledAt)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, task); err != nil {
		return nil, err
	}

	if err := s.queue.Push(ctx, task.ID.String(), task.Priority); err != nil {
		s.logger.Error().Err(err).Str("task_id", task.ID.String()).Msg("failed to push to redis queue")
		return nil, err
	}


	if task.ScheduledAt == nil || task.ScheduledAt.Before(time.Now()) {
		if err := s.publisher.Publish(ctx, task); err != nil {
			s.logger.Error().Err(err).Str("task_id", task.ID.String()).Msg("failed to publish to rabbitmq")
			return nil, err
		}
	}

	s.logger.Info().Str("task_id", task.ID.String()).Msg("task created successfully")
	return task, nil
}

func (s *TaskService) CreateBulk(ctx context.Context, dtos domain.CreateBulkTaskDTO) (*domain.BulkTaskResponse, error) {
	if len(dtos.Tasks) == 0 {
		return &domain.BulkTaskResponse{}, nil
	}

	validTasks := make([]*domain.Task, 0, len(dtos.Tasks))
	taskErrorMessages := make([]string, 0)

	for idx, tDto := range dtos.Tasks {
		if err := tDto.Validate(); err != nil {
			msg := "index " + string(rune(idx)) + ": " + err.Error()
			taskErrorMessages = append(taskErrorMessages, msg)
			continue
		}
		t, err := domain.NewTask(tDto.Name, tDto.Payload, tDto.Priority, tDto.ScheduledAt)
		if err != nil {
			msg := "index " + string(rune(idx)) + ": " + err.Error()
			taskErrorMessages = append(taskErrorMessages, msg)
			continue
		}
		validTasks = append(validTasks, t)
	}

	if len(validTasks) == 0 {
		return &domain.BulkTaskResponse{
			FailureCount: len(dtos.Tasks),
			Errors:       taskErrorMessages,
		}, nil
	}

	if err := s.repo.BulkInsert(ctx, validTasks); err != nil {
		s.logger.Error().Err(err).Msg("failed bulk insert tasks")
		return nil, err
	}

	redisMap := make(map[string]domain.TaskPriority)
	rabbitTasks := make([]*domain.Task, 0)

	for _, t := range validTasks {
		redisMap[t.ID.String()] = t.Priority
		if t.ScheduledAt == nil || t.ScheduledAt.Before(time.Now()) {
			rabbitTasks = append(rabbitTasks, t)
		}
	}

	if err := s.queue.BulkPush(ctx, redisMap); err != nil {
		s.logger.Error().Err(err).Msg("failed bulk push to redis")
		return nil, err
	}

	if len(rabbitTasks) > 0 {
		if err := s.publisher.PublishBatch(ctx, rabbitTasks); err != nil {
			s.logger.Error().Err(err).Msg("failed batch publish to rabbitmq")
			return nil, err
		}
	}

	responses := make([]domain.TaskResponse, len(validTasks))
	for i, t := range validTasks {
		responses[i] = t.ToResponse()
	}

	return &domain.BulkTaskResponse{
		SuccessCount: len(validTasks),
		FailureCount: len(dtos.Tasks) - len(validTasks),
		Tasks:        responses,
		Errors:       taskErrorMessages,
	}, nil
}

func (s *TaskService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Task, error) {
	task, err := s.repo.FindByID(ctx, id)
	if err != nil {
		s.logger.Debug().Err(err).Str("task_id", id.String()).Msg("failed to get task")
		return nil, err
	}
	return task, nil
}

func (s *TaskService) Cancel(ctx context.Context, id uuid.UUID) error {
	task, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}

	if task.Status == domain.TaskStatusCompleted || task.Status == domain.TaskStatusCancelled {
		return domain.ErrInvalidTaskStatus
	}

	if err := s.repo.UpdateStatus(ctx, id, domain.TaskStatusCancelled); err != nil {
		return err
	}

	if err := s.queue.Remove(ctx, id.String()); err != nil {
		s.logger.Warn().Err(err).Str("task_id", id.String()).Msg("failed to remove from redis queue during cancel")
	}

	return nil
}

func (s *TaskService) GetMetrics(ctx context.Context) (*domain.TaskMetrics, error) {
	return s.repo.GetMetrics(ctx)
}
