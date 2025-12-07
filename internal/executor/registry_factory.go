package executor

import "github.com/rs/zerolog"

func NewExecutorRegistry(logger zerolog.Logger) *Registry {
	r := NewRegistry(logger)

	r.Register(NewEmailExecutor())
	r.Register(NewPaymentExecutor())

	return r
}
