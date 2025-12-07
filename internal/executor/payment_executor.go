package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"distribute-task-sytem/internal/domain"

	"github.com/rs/zerolog/log"
)

type PaymentExecutor struct {
}

func NewPaymentExecutor() *PaymentExecutor {
	return &PaymentExecutor{}
}

type PaymentPayload struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
	OrderID  string  `json:"order_id"`
}

func (e *PaymentExecutor) Name() string {
	return "process_payment"
}

func (e *PaymentExecutor) Execute(ctx context.Context, task *domain.Task) error {
	var payload PaymentPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return fmt.Errorf("invalid payload for payment executor: %w", err)
	}

	log.Info().
		Str("task_id", task.ID.String()).
		Float64("amount", payload.Amount).
		Str("order_id", payload.OrderID).
		Msg("processing payment...")

	processTime := time.Duration(2000+rand.Intn(3000)) * time.Millisecond

	select {
	case <-time.After(processTime):
		log.Info().Str("task_id", task.ID.String()).Msg("payment processed successfully")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
