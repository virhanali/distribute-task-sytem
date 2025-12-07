package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"distribute-task-sytem/internal/domain"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
	"github.com/sony/gobreaker"
)

type WorkerConfig struct {
	NumWorkers  int
	RabbitMQURL string
	QueueNames  []string
}

type WorkerPool struct {
	config   WorkerConfig
	repo     domain.TaskRepository
	executor TaskExecutor
	logger   zerolog.Logger
	conn     *amqp.Connection
	ch       *amqp.Channel
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
	cb       *gobreaker.CircuitBreaker
	jobChan  chan amqp.Delivery
	metrics  *WorkerMetrics
}

type WorkerMetrics struct {
	mu           sync.RWMutex
	Processed    int64
	Failures     int64
	ProcessTime  int64 // Total milliseconds
	RequestCount int64 // To calc avg
}

type TaskMessage struct {
	TaskID     string          `json:"task_id"`
	Name       string          `json:"name"`
	Payload    json.RawMessage `json:"payload"`
	RetryCount int             `json:"retry_count"`
}

func NewWorkerPool(
	config WorkerConfig,
	repo domain.TaskRepository,
	executor TaskExecutor,
	logger zerolog.Logger,
) *WorkerPool {
	st := gobreaker.Settings{
		Name:        "WorkerCircuitBreaker",
		MaxRequests: 5,
		Interval:    10 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 5 && failureRatio >= 0.6
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &WorkerPool{
		config:   config,
		repo:     repo,
		executor: executor,
		logger:   logger,
		cb:       gobreaker.NewCircuitBreaker(st),
		jobChan:  make(chan amqp.Delivery, config.NumWorkers*2),
		ctx:      ctx,
		cancel:   cancel,
		metrics:  &WorkerMetrics{},
	}
}

func (wp *WorkerPool) Start() error {
	wp.logger.Info().Msg("starting worker pool...")

	var err error
	wp.conn, err = amqp.Dial(wp.config.RabbitMQURL)
	if err != nil {
		return fmt.Errorf("failed to connect to rabbitmq: %w", err)
	}

	wp.ch, err = wp.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open channel: %w", err)
	}

	if err := wp.ch.Qos(
		wp.config.NumWorkers*2, // prefetch count
		0,                      // prefetch size
		false,                  // global
	); err != nil {
		return err
	}

	for _, qName := range wp.config.QueueNames {
		msgs, err := wp.ch.Consume(
			qName, // queue
			"",    // consumer tag (auto-generated)
			false, // auto-ack (we manual ack)
			false, // exclusive
			false, // no-local
			false, // no-wait
			nil,   // args
		)
		if err != nil {
			return fmt.Errorf("failed to consume from queue %s: %w", qName, err)
		}

		wp.wg.Add(1)
		go func(q string, delivery <-chan amqp.Delivery) {
			defer wp.wg.Done()
			for {
				select {
				case msg, ok := <-delivery:
					if !ok {
						wp.logger.Warn().Str("queue", q).Msg("delivery channel closed")
						return
					}
					select {
					case wp.jobChan <- msg:
					case <-wp.ctx.Done():
						return
					}
				case <-wp.ctx.Done():
					return
				}
			}
		}(qName, msgs)
	}

	for i := 0; i < wp.config.NumWorkers; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}

	go wp.monitorConnection()

	return nil
}

func (wp *WorkerPool) Stop() {
	wp.logger.Info().Msg("stopping worker pool...")
	wp.cancel()

	if wp.ch != nil {
		wp.ch.Close()
	}
	if wp.conn != nil {
		wp.conn.Close()
	}
}

func (wp *WorkerPool) Wait() {
	wp.wg.Wait()
}

func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			wp.logger.Error().Interface("panic", r).Int("worker_id", id).Msg("worker panicked, recovering...")
		}
	}()

	for {
		select {
		case msg, ok := <-wp.jobChan:
			if !ok {
				return
			}
			wp.processMessage(msg)
		case <-wp.ctx.Done():
			return
		}
	}
}

func (wp *WorkerPool) processMessage(msg amqp.Delivery) {
	startTime := time.Now()

	_, err := wp.cb.Execute(func() (interface{}, error) {
		return nil, wp.handleTask(msg)
	})

	duration := time.Since(startTime).Milliseconds()
	wp.updateMetrics(err == nil, duration)

	if err != nil {
		if err == gobreaker.ErrOpenState {
			wp.logger.Warn().Msg("circuit breaker open, requeuing message")
			msg.Nack(false, true)
			time.Sleep(1 * time.Second) // prevent busy loop
		}
	} else {
		msg.Ack(false)
	}
}

func (wp *WorkerPool) handleTask(msg amqp.Delivery) error {
	var taskMsg TaskMessage
	if err := json.Unmarshal(msg.Body, &taskMsg); err != nil {
		wp.logger.Error().Err(err).Msg("failed to unmarshal task message")
		msg.Nack(false, false)
		return nil // Return nil so CB considers it "success" (as in handled), or error to trip?
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	taskID, err := uuid.Parse(taskMsg.TaskID)
	if err != nil {
		msg.Nack(false, false)
		return nil
	}

	task, err := wp.repo.FindByID(ctx, taskID)
	if err != nil {
		wp.logger.Error().Err(err).Str("task_id", taskMsg.TaskID).Msg("failed to find task in db")
		return err
	}

	if task.Status == domain.TaskStatusCancelled {
		return nil
	}

	wp.repo.UpdateStatus(ctx, taskID, domain.TaskStatusProcessing) // Use custom method if we want to set StartedAt

	execErr := wp.executor.Execute(ctx, task)

	if execErr != nil {
		wp.logger.Error().Err(execErr).Str("task_id", taskID.String()).Msg("task execution failed")

		task.MarkAsFailed(execErr.Error())


		isRetryable := task.IsRetryable()


		if isRetryable {
			msg.Nack(false, true)
		} else {
			msg.Nack(false, false)
		}

		return execErr // Trip breaker? depends on policy.
	}

	task.MarkAsCompleted()
	wp.repo.UpdateStatus(ctx, taskID, domain.TaskStatusCompleted)
	return nil
}

func (wp *WorkerPool) monitorConnection() {
	notifyClose := wp.conn.NotifyClose(make(chan *amqp.Error))

	select {
	case <-notifyClose:
		wp.logger.Warn().Msg("rabbitmq connection lost in worker pool")
	case <-wp.ctx.Done():
		return
	}
}

func (wp *WorkerPool) updateMetrics(success bool, duration int64) {
	wp.metrics.mu.Lock()
	defer wp.metrics.mu.Unlock()

	wp.metrics.RequestCount++
	wp.metrics.ProcessTime += duration
	if success {
		wp.metrics.Processed++
	} else {
		wp.metrics.Failures++
	}
}
