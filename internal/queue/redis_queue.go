package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"distribute-task-sytem/internal/domain"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

const (
	tasksQueueKey   = "tasks:queue"
	dedupKeyPrefix  = "tasks:dedup:"
	lockKeyPrefix   = "tasks:lock:"
	defaultDedupTTL = 24 * time.Hour
	defaultLockTTL  = 5 * time.Minute
	maxTimestamp    = 9999999999 // Future timestamp for score calculation
)

var (
	ErrTaskAlreadyQueued = errors.New("task already queued")
	ErrLockAcquireFailed = errors.New("failed to acquire lock")
)

type QueueConfig struct {
	RedisAddr     string
	RedisPassword string
	RedisDB       int
}

type RedisQueue struct {
	client *redis.Client
}

func NewRedisQueue(client *redis.Client) *RedisQueue {
	return &RedisQueue{
		client: client,
	}
}

func (q *RedisQueue) Push(ctx context.Context, taskID string, priority domain.TaskPriority) error {
	dedupKey := dedupKeyPrefix + taskID

	isNew, err := q.client.SetNX(ctx, dedupKey, 1, defaultDedupTTL).Result()
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("redis error checking idempotency")
		return fmt.Errorf("redis error: %w", err)
	}
	if !isNew {
		return ErrTaskAlreadyQueued
	}

	score := calculateScore(priority)

	err = q.client.ZAdd(ctx, tasksQueueKey, redis.Z{
		Score:  score,
		Member: taskID,
	}).Err()

	if err != nil {
		q.client.Del(ctx, dedupKey)
		log.Error().Err(err).Str("task_id", taskID).Msg("failed to push task to queue")
		return fmt.Errorf("failed to queue task: %w", err)
	}

	return nil
}

func (q *RedisQueue) BulkPush(ctx context.Context, tasks map[string]domain.TaskPriority) error {
	if len(tasks) == 0 {
		return nil
	}

	pipe := q.client.Pipeline()


	for taskID, priority := range tasks {
		dedupKey := dedupKeyPrefix + taskID
		score := calculateScore(priority)


		pipe.SetNX(ctx, dedupKey, 1, defaultDedupTTL)
		pipe.ZAdd(ctx, tasksQueueKey, redis.Z{
			Score:  score,
			Member: taskID,
		})
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		log.Error().Err(err).Msg("bulk push failed")
		return err
	}

	return nil
}

func (q *RedisQueue) Pop(ctx context.Context, count int64) ([]string, error) {

	results, err := q.client.ZPopMax(ctx, tasksQueueKey, count).Result()
	if err != nil {
		log.Error().Err(err).Msg("failed to pop tasks")
		return nil, err
	}

	if len(results) == 0 {
		return []string{}, nil
	}

	taskIDs := make([]string, len(results))
	for i, z := range results {
		taskIDs[i] = z.Member.(string)
	}

	return taskIDs, nil
}

func (q *RedisQueue) Remove(ctx context.Context, taskID string) error {
	pipe := q.client.Pipeline()
	pipe.ZRem(ctx, tasksQueueKey, taskID)
	pipe.Del(ctx, dedupKeyPrefix+taskID)
	pipe.Del(ctx, lockKeyPrefix+taskID)

	_, err := pipe.Exec(ctx)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("failed to remove task")
		return err
	}
	return nil
}

func (q *RedisQueue) GetQueueSize(ctx context.Context) (int64, error) {
	return q.client.ZCard(ctx, tasksQueueKey).Result()
}

func (q *RedisQueue) AcquireLock(ctx context.Context, taskID string, ttl time.Duration) (bool, error) {
	key := lockKeyPrefix + taskID
	success, err := q.client.SetNX(ctx, key, 1, ttl).Result()
	if err != nil {
		return false, err
	}
	return success, nil
}

func (q *RedisQueue) ReleaseLock(ctx context.Context, taskID string) error {
	key := lockKeyPrefix + taskID
	return q.client.Del(ctx, key).Err()
}

func calculateScore(priority domain.TaskPriority) float64 {
	var pScore float64
	switch priority {
	case domain.TaskPriorityHigh:
		pScore = 3.0
	case domain.TaskPriorityMedium:
		pScore = 2.0
	case domain.TaskPriorityLow:
		pScore = 1.0
	default:
		pScore = 1.0
	}


	now := float64(time.Now().Unix())
	timePart := float64(maxTimestamp) - now

	return (pScore * 10000000000.0) + timePart
}
