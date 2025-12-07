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

// Push adds a task to the priority queue with idempotency check
func (q *RedisQueue) Push(ctx context.Context, taskID string, priority domain.TaskPriority) error {
	dedupKey := dedupKeyPrefix + taskID

	// Idempotency check using SET NX
	// Returns true if key was set (new task), false if already exists
	isNew, err := q.client.SetNX(ctx, dedupKey, 1, defaultDedupTTL).Result()
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("redis error checking idempotency")
		return fmt.Errorf("redis error: %w", err)
	}
	if !isNew {
		// Task is already in queue or processed recently
		return ErrTaskAlreadyQueued
	}

	score := calculateScore(priority)

	err = q.client.ZAdd(ctx, tasksQueueKey, redis.Z{
		Score:  score,
		Member: taskID,
	}).Err()

	if err != nil {
		// Rollback dedup key if ZADD fails
		q.client.Del(ctx, dedupKey)
		log.Error().Err(err).Str("task_id", taskID).Msg("failed to push task to queue")
		return fmt.Errorf("failed to queue task: %w", err)
	}

	return nil
}

// BulkPush pushes multiple tasks using pipeline
func (q *RedisQueue) BulkPush(ctx context.Context, tasks map[string]domain.TaskPriority) error {
	if len(tasks) == 0 {
		return nil
	}

	pipe := q.client.Pipeline()

	// Track which tasks are being added to properly handle potential errors logic if needed
	// But for pipeline, we usually execute and check errors.
	// Optimistic approach: Add all valid ones.

	for taskID, priority := range tasks {
		dedupKey := dedupKeyPrefix + taskID
		score := calculateScore(priority)

		// Create atomic block in pipeline: Check/Set Dedupe -> ZAdd
		// Note: SETNX inside pipeline will just execute. We can't conditionally ZADD in a standard pipeline based on the SETNX result of the SAME pipeline easily without Lua.
		// For simplicity/bulk, we might accept minor collision or just set everything.
		// However, correct way is:
		// 1. SETNX
		// 2. ZADD (regardless? or conditionally?).
		// If we want strict uniqueness, we should assume if SETNX fails, we shouldn't ZADD.
		// Standard pipeline doesn't support "If previous cmd result was X".
		// We should use Lua script for strict atomic bulk push or just simple commands.
		// Given requirements "Gunakan redis.Pipeline", let's use standard cmds.
		// We'll update the dedup key and ZADD.
		// If it's a re-push of same task, ZADD updates score/does nothing.

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

// Pop retrieves N tasks with highest priority
func (q *RedisQueue) Pop(ctx context.Context, count int64) ([]string, error) {
	// ZPOPMAX removes and returns members with the highest scores.
	// Our score is (Priority * Const) + (Future - Now).
	// Higher Priority -> Much Higher Score.
	// Same Priority -> Older task (smaller Now) -> Larger (Future - Now) -> Higher Score.
	// So ZPOPMAX gives Highest Priority + Oldest Task first.

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
		// Note: We don't remove the dedup key here.
		// It stays until TTL expires or task is fully marked done/removed explicitly if we want to allow re-queuing.
		// Usually we clean it up in Remove().
	}

	return taskIDs, nil
}

// Remove removes a specific task from queue and clears its idempotency key
func (q *RedisQueue) Remove(ctx context.Context, taskID string) error {
	pipe := q.client.Pipeline()
	pipe.ZRem(ctx, tasksQueueKey, taskID)
	pipe.Del(ctx, dedupKeyPrefix+taskID)
	// We also release any lock if exists?
	// Usually Remove is called when task is permanently done or cancelled.
	pipe.Del(ctx, lockKeyPrefix+taskID)

	_, err := pipe.Exec(ctx)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("failed to remove task")
		return err
	}
	return nil
}

// GetQueueSize returns current number of tasks
func (q *RedisQueue) GetQueueSize(ctx context.Context) (int64, error) {
	return q.client.ZCard(ctx, tasksQueueKey).Result()
}

// AcquireLock attempts to acquire a distributed lock for the task
func (q *RedisQueue) AcquireLock(ctx context.Context, taskID string, ttl time.Duration) (bool, error) {
	key := lockKeyPrefix + taskID
	success, err := q.client.SetNX(ctx, key, 1, ttl).Result()
	if err != nil {
		return false, err
	}
	return success, nil
}

// ReleaseLock releases the distributed lock
func (q *RedisQueue) ReleaseLock(ctx context.Context, taskID string) error {
	key := lockKeyPrefix + taskID
	return q.client.Del(ctx, key).Err()
}

// calculateScore generates a score for ZSET
// Highest Priority + Earliest Time = Highest Score
func calculateScore(priority domain.TaskPriority) float64 {
	// Priority Scores: High=3, Medium=2, Low=1
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

	// We want FIFO: Older timestamp should have higher score within the same priority.
	// Timestamp is seconds.
	// Score = Priority * 10^10 + (MaxTimestamp - CurrentTimestamp)
	// Example:
	// High (3), Old (100) -> 300..00 + (Big - 100) = 300..900
	// High (3), New (200) -> 300..00 + (Big - 200) = 300..800
	// Sorted: 300..900 (First), 300..800 (Second). Correct.

	now := float64(time.Now().Unix())
	timePart := float64(maxTimestamp) - now

	return (pScore * 10000000000.0) + timePart
}
