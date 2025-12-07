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

// Create handles the creation of a single task
func (s *TaskService) Create(ctx context.Context, dto domain.CreateTaskDTO) (*domain.Task, error) {
	if err := dto.Validate(); err != nil {
		return nil, err
	}

	task, err := domain.NewTask(dto.Name, dto.Payload, dto.Priority, dto.ScheduledAt)
	if err != nil {
		return nil, err
	}

	// 1. Save to PostgreSQL
	if err := s.repo.Create(ctx, task); err != nil {
		return nil, err
	}
	// Note: We don't rollback DB on Queue/Pub fail unless strict consistency is needed.
	// Usually we let background jobs pick up 'queued' tasks that are not in queue/broker.
	// But simply returning error here is fine for now; caller handles it (maybe retries).

	// 2. Push to Redis (Priority Queue / Scheduling)
	if err := s.queue.Push(ctx, task.ID.String(), task.Priority); err != nil {
		s.logger.Error().Err(err).Str("task_id", task.ID.String()).Msg("failed to push to redis queue")
		// Often we might not want to fail the whole request if DB persisted fine,
		// but if "Create" implies "Enqueued", we should probably return error or ensuring eventual consistency.
		// For this imperative flow, I'll log and return error?
		// Actually, if we fail here, the task exists in DB as 'queued' but not in Redis/Rabbit.
		// A separate "sweeper" process (FindPendingTasks) would pick it up.
		// So we can still return the task with success but maybe a warning log.
		// However, adhering to "strict" success usually demands all steps pass.
		// Let's log error and return partial success or error.
		// I will return error to signal something went wrong.
		return nil, err
	}

	// 3. Publish to RabbitMQ (Worker Distribution)
	// Only publish if it's ready to execute (not scheduled in future)
	// If it has ScheduledAt, Redis (or a scheduler) might handle delayed execution and then PUSH to RabbitMQ later.
	// BUT, requirement says "Publish ke RabbitMQ".
	// If ScheduledAt exists and is future, RabbitMQ might not support delay natively without plugin.
	// Redis ZSET is great for delay.
	// I'll assume if it's scheduled, we ONLY ensure it's in Redis (which we did).
	// If it's IMMEDIATE, we might want to pub to RabbitMQ now.
	// OR: Logic is: DB -> Redis. Separate "Dispatcher" reads Redis -> RabbitMQ.
	// BUT user says "Create" method must "Publish ke RabbitMQ".
	// I will do it. If it is scheduled, consumer must handle delay or we just publish.
	// Given checks in `domain.NewTask` and `CanExecute`, maybe we check here?

	if task.ScheduledAt == nil || task.ScheduledAt.Before(time.Now()) {
		if err := s.publisher.Publish(ctx, task); err != nil {
			s.logger.Error().Err(err).Str("task_id", task.ID.String()).Msg("failed to publish to rabbitmq")
			// We fallback to error.
			return nil, err
		}
	}

	s.logger.Info().Str("task_id", task.ID.String()).Msg("task created successfully")
	return task, nil
}

// CreateBulk handles batch creation of tasks
func (s *TaskService) CreateBulk(ctx context.Context, dtos domain.CreateBulkTaskDTO) (*domain.BulkTaskResponse, error) {
	if len(dtos.Tasks) == 0 {
		return &domain.BulkTaskResponse{}, nil
	}

	validTasks := make([]*domain.Task, 0, len(dtos.Tasks))
	taskErrorMessages := make([]string, 0)

	// Validate and instantiate
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

	// 1. Bulk Insert to DB
	if err := s.repo.BulkInsert(ctx, validTasks); err != nil {
		s.logger.Error().Err(err).Msg("failed bulk insert tasks")
		return nil, err
	}

	// 2. Bulk Push to Redis
	redisMap := make(map[string]domain.TaskPriority)
	// 3. Bulk Publish to RabbitMQ
	rabbitTasks := make([]*domain.Task, 0)

	for _, t := range validTasks {
		redisMap[t.ID.String()] = t.Priority
		if t.ScheduledAt == nil || t.ScheduledAt.Before(time.Now()) {
			rabbitTasks = append(rabbitTasks, t)
		}
	}

	if err := s.queue.BulkPush(ctx, redisMap); err != nil {
		s.logger.Error().Err(err).Msg("failed bulk push to redis")
		// Partial failure logic?
		// We'll return error for now.
		return nil, err
	}

	if len(rabbitTasks) > 0 {
		if err := s.publisher.PublishBatch(ctx, rabbitTasks); err != nil {
			s.logger.Error().Err(err).Msg("failed batch publish to rabbitmq")
			return nil, err
		}
	}

	// Prepare Response
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
	// check existence first or just update
	task, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}

	if task.Status == domain.TaskStatusCompleted || task.Status == domain.TaskStatusCancelled {
		return domain.ErrInvalidTaskStatus
	}

	// 1. Update DB
	if err := s.repo.UpdateStatus(ctx, id, domain.TaskStatusCancelled); err != nil {
		return err
	}

	// 2. Remove from Redis
	// We do this best effort. If it's already popped by worker but not acked,
	// worker logic needs to check status before processing.
	if err := s.queue.Remove(ctx, id.String()); err != nil {
		s.logger.Warn().Err(err).Str("task_id", id.String()).Msg("failed to remove from redis queue during cancel")
	}

	return nil
}

func (s *TaskService) GetMetrics(ctx context.Context) (*domain.TaskMetrics, error) {
	return s.repo.GetMetrics(ctx)
}
