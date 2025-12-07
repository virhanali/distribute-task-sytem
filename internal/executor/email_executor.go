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

type EmailExecutor struct {
}

func NewEmailExecutor() *EmailExecutor {
	return &EmailExecutor{}
}

type EmailPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (e *EmailExecutor) Name() string {
	return "email_delivery"
}

func (e *EmailExecutor) Execute(ctx context.Context, task *domain.Task) error {
	var payload EmailPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return fmt.Errorf("invalid payload for email executor: %w", err)
	}

	log.Info().
		Str("task_id", task.ID.String()).
		Str("to", payload.To).
		Str("subject", payload.Subject).
		Msg("sending email...")

	processTime := time.Duration(1000+rand.Intn(2000)) * time.Millisecond

	select {
	case <-time.After(processTime):
		log.Info().Str("task_id", task.ID.String()).Msg("email sent successfully")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
