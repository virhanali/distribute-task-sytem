package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"distribute-task-sytem/internal/domain"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
)

const (
	ExchangeName      = "tasks"
	ExchangeType      = "topic"
	QueueHigh         = "tasks.high"
	QueueMedium       = "tasks.medium"
	QueueLow          = "tasks.low"
	RoutingKeyHigh    = "task.high"
	RoutingKeyMedium  = "task.medium"
	RoutingKeyLow     = "task.low"
	PublishMaxRetries = 3
)

type RabbitMQPublisher struct {
	url        string
	conn       *amqp.Connection
	ch         *amqp.Channel
	notifyChan chan *amqp.Error
	mu         sync.RWMutex
	isClosed   bool
}

type TaskMessage struct {
	TaskID     string          `json:"task_id"`
	Name       string          `json:"name"`
	Payload    json.RawMessage `json:"payload"`
	RetryCount int             `json:"retry_count"`
}

func NewRabbitMQPublisher(url string) (*RabbitMQPublisher, error) {
	p := &RabbitMQPublisher{
		url: url,
	}

	if err := p.connect(); err != nil {
		return nil, err
	}

	return p, nil
}

func (p *RabbitMQPublisher) connect() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isClosed {
		return errors.New("publisher is closed")
	}

	var err error
	log.Info().Msg("connecting to existing rabbitmq connection")

	// 1. Create Connection
	p.conn, err = amqp.Dial(p.url)
	if err != nil {
		return fmt.Errorf("failed to connect to rabbitmq: %w", err)
	}

	// 2. Create Channel
	p.ch, err = p.conn.Channel()
	if err != nil {
		p.conn.Close()
		return fmt.Errorf("failed to open channel: %w", err)
	}

	// 3. Setup Topology (Exchange and Queues)
	if err := p.setupTopology(); err != nil {
		p.ch.Close()
		p.conn.Close()
		return err
	}

	// 4. Enable Publisher Confirms
	if err := p.ch.Confirm(false); err != nil {
		p.ch.Close()
		p.conn.Close()
		return fmt.Errorf("failed to enable publisher confirms: %w", err)
	}

	// 5. Handle Reconnection
	p.notifyChan = make(chan *amqp.Error)
	p.conn.NotifyClose(p.notifyChan)

	go p.handleReconnection()

	return nil
}

func (p *RabbitMQPublisher) setupTopology() error {
	// Declare Exchange
	err := p.ch.ExchangeDeclare(
		ExchangeName, // name
		ExchangeType, // type
		true,         // durable
		false,        // auto-deleted
		false,        // internal
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare exchange: %w", err)
	}

	// Declare and Bind Queues
	queues := map[string]string{
		QueueHigh:   RoutingKeyHigh,
		QueueMedium: RoutingKeyMedium,
		QueueLow:    RoutingKeyLow,
	}

	for qName, rKey := range queues {
		_, err := p.ch.QueueDeclare(
			qName, // name
			true,  // durable
			false, // delete when unused
			false, // exclusive
			false, // no-wait
			nil,   // arguments
		)
		if err != nil {
			return fmt.Errorf("failed to declare queue %s: %w", qName, err)
		}

		err = p.ch.QueueBind(
			qName,        // queue name
			rKey,         // routing key
			ExchangeName, // exchange
			false,
			nil,
		)
		if err != nil {
			return fmt.Errorf("failed to bind queue %s: %w", qName, err)
		}
	}

	return nil
}

func (p *RabbitMQPublisher) handleReconnection() {
	for range p.notifyChan {
		p.mu.Lock()
		if p.isClosed {
			p.mu.Unlock()
			return
		}
		p.mu.Unlock()

		log.Warn().Msg("rabbitmq connection lost, attempting to reconnect...")

		for {
			err := p.connect()
			if err == nil {
				log.Info().Msg("rabbitmq reconnected successfully")
				return
			}

			log.Error().Err(err).Msg("failed to reconnect to rabbitmq, retrying in 5s...")
			time.Sleep(5 * time.Second)
		}
	}
}

func (p *RabbitMQPublisher) Publish(ctx context.Context, task *domain.Task) error {
	return p.publishWithRetry(ctx, task, 0)
}

func (p *RabbitMQPublisher) publishWithRetry(ctx context.Context, task *domain.Task, retries int) error {
	p.mu.RLock()
	if p.conn == nil || p.conn.IsClosed() {
		p.mu.RUnlock()
		// Wait for reconnection or force failure
		if retries < PublishMaxRetries {
			time.Sleep(time.Duration(math.Pow(2, float64(retries))) * time.Second)
			return p.publishWithRetry(ctx, task, retries+1)
		}
		return errors.New("rabbitmq connection not available")
	}
	ch := p.ch // thread-safe capture? amqp channels are thread safe for publishing usually, but we need to ensure not nil
	p.mu.RUnlock()

	msgBody, err := json.Marshal(TaskMessage{
		TaskID:     task.ID.String(),
		Name:       task.Name,
		Payload:    task.Payload,
		RetryCount: task.RetryCount,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal task: %w", err)
	}

	routingKey := getRoutingKey(task.Priority)

	// Create Confirmation Channel for this single publish
	// Note: For high throughput, we shouldn't do this per message.
	// But requirement asks for "Publisher confirms".
	// The robust way is enabling it globally and listening to NotifyPublish,
	// OR using PublishWithDeferredConfirm (if available) or wait for confirmation.
	// Simple way:
	confirmation, err := ch.PublishWithDeferredConfirmWithContext(
		ctx,
		ExchangeName,
		routingKey,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent, // Persistent
			Body:         msgBody,
			Timestamp:    time.Now(),
			MessageId:    task.ID.String(),
		},
	)

	if err != nil {
		if retries < PublishMaxRetries {
			log.Warn().Err(err).Int("retry", retries).Msg("failed to publish, retrying...")
			time.Sleep(time.Duration(math.Pow(2, float64(retries))) * time.Second)
			return p.publishWithRetry(ctx, task, retries+1)
		}
		return err
	}

	// Wait for ack
	ok := confirmation.Wait()
	if !ok {
		return errors.New("failed to key publisher confirmation from rabbitmq")
	}

	return nil
}

func (p *RabbitMQPublisher) PublishBatch(ctx context.Context, tasks []*domain.Task) error {
	for _, task := range tasks {
		if err := p.Publish(ctx, task); err != nil {
			// On batch failure, we can stop and return error, or try best effort.
			// Returning error on first failure is safer provided idempotency exists downstream.
			return fmt.Errorf("failed to publish task %s: %w", task.ID, err)
		}
	}
	return nil
}

func (p *RabbitMQPublisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.isClosed = true

	if p.ch != nil {
		if err := p.ch.Close(); err != nil {
			log.Error().Err(err).Msg("failed to close channel")
		}
	}
	if p.conn != nil {
		if err := p.conn.Close(); err != nil {
			return err
		}
	}
	return nil
}

func getRoutingKey(priority domain.TaskPriority) string {
	switch priority {
	case domain.TaskPriorityHigh:
		return RoutingKeyHigh
	case domain.TaskPriorityMedium:
		return RoutingKeyMedium
	case domain.TaskPriorityLow:
		return RoutingKeyLow
	default:
		return RoutingKeyLow
	}
}
