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
