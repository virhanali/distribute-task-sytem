package repository

import (
	"context"
	"database/sql"
	"time"

	"distribute-task-sytem/internal/domain"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

type PostgresTaskRepository struct {
	db *sqlx.DB
}

func NewTaskRepository(db *sqlx.DB) domain.TaskRepository {
	return &PostgresTaskRepository{
		db: db,
	}
}

func (r *PostgresTaskRepository) Create(ctx context.Context, task *domain.Task) error {
	query := `
		INSERT INTO tasks (
			id, name, payload, priority, status, scheduled_at, 
			retry_count, max_retry, created_at, updated_at
		) VALUES (
			:id, :name, :payload, :priority, :status, :scheduled_at,
			:retry_count, :max_retry, :created_at, :updated_at
		)
	`
	_, err := r.db.NamedExecContext(ctx, query, task)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID.String()).Msg("failed to create task")
		return err
	}
	return nil
}

func (r *PostgresTaskRepository) BulkInsert(ctx context.Context, tasks []*domain.Task) error {
	if len(tasks) == 0 {
		return nil
	}

	const batchSize = 100
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
		INSERT INTO tasks (
			id, name, payload, priority, status, scheduled_at,
			retry_count, max_retry, created_at, updated_at
		) VALUES (
			:id, :name, :payload, :priority, :status, :scheduled_at,
			:retry_count, :max_retry, :created_at, :updated_at
		)
	`

	// Process in batches
	for i := 0; i < len(tasks); i += batchSize {
		end := i + batchSize
		if end > len(tasks) {
			end = len(tasks)
		}

		batch := tasks[i:end]
		_, err := tx.NamedExecContext(ctx, query, batch)
		if err != nil {
			log.Error().Err(err).Int("batch_start", i).Msg("failed to insert batch")
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		log.Error().Err(err).Msg("failed to commit bulk insert transaction")
		return err
	}

	return nil
}

func (r *PostgresTaskRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Task, error) {
	var task domain.Task
	query := "SELECT * FROM tasks WHERE id = $1"

	err := r.db.GetContext(ctx, &task, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrTaskNotFound
		}
		log.Error().Err(err).Str("task_id", id.String()).Msg("failed to find task")
		return nil, err
	}

	return &task, nil
}

func (r *PostgresTaskRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.TaskStatus) error {
	query := `
		UPDATE tasks 
		SET status = $1, updated_at = $2 
		WHERE id = $3
	`

	result, err := r.db.ExecContext(ctx, query, status, time.Now(), id)
	if err != nil {
		log.Error().Err(err).Str("task_id", id.String()).Str("status", string(status)).Msg("failed to update task status")
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrTaskNotFound
	}

	return nil
}

func (r *PostgresTaskRepository) FindPendingTasks(ctx context.Context, limit int) ([]*domain.Task, error) {
	tasks := []*domain.Task{}

	// Pending tasks logic:
	// 1. Status is Queued OR (Failed but retryable)
	// 2. ScheduledAt is NULL OR ScheduledAt <= Now
	// Sort by Priority (High > Medium > Low), then CreatedAt
	query := `
		SELECT * FROM tasks 
		WHERE (status = 'queued' OR (status = 'failed' AND retry_count < max_retry))
		AND (scheduled_at IS NULL OR scheduled_at <= NOW())
		ORDER BY 
			CASE priority 
				WHEN 'high' THEN 1 
				WHEN 'medium' THEN 2 
				WHEN 'low' THEN 3 
				ELSE 4 
			END,
			created_at ASC
		LIMIT $1
	`

	err := r.db.SelectContext(ctx, &tasks, query, limit)
	if err != nil {
		log.Error().Err(err).Msg("failed to find pending tasks")
		return nil, err
	}

	return tasks, nil
}

func (r *PostgresTaskRepository) GetMetrics(ctx context.Context) (*domain.TaskMetrics, error) {
	metrics := &domain.TaskMetrics{}

	query := `
		SELECT 
			COUNT(*) as total,
			COUNT(CASE WHEN status = 'queued' THEN 1 END) as queued,
			COUNT(CASE WHEN status = 'processing' THEN 1 END) as processing,
			COUNT(CASE WHEN status = 'completed' THEN 1 END) as completed,
			COUNT(CASE WHEN status = 'failed' THEN 1 END) as failed,
			COUNT(CASE WHEN status = 'cancelled' THEN 1 END) as cancelled
		FROM tasks
	`

	// Since we are mapping to a struct that doesn't have db tags matching this exact query output
	// we will likely need to scan manually or define a temporary struct.
	// sqlx can map standard columns, but COUNT expressions needs aliases.
	// The struct fields are Start Case, db is usually snake_case.
	// Let's rely on explicit struct scan or simple logic.
	// Actually domain.TaskMetrics has json tags but not db tags.
	// Let's add db tags to the query aliases if possible or use a temp struct.

	// Better approach: Use a row scan.
	row := r.db.QueryRowContext(ctx, query)
	err := row.Scan(
		&metrics.Total,
		&metrics.Queued,
		&metrics.Processing,
		&metrics.Completed,
		&metrics.Failed,
		&metrics.Cancelled,
	)

	if err != nil {
		log.Error().Err(err).Msg("failed to get metrics")
		return nil, err
	}

	return metrics, nil
}
