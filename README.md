# Mirage Boilerplate

A production-ready Go backend boilerplate implementing **Clean Architecture**.

## Architecture Overview

The project follows a unidirectional flow:

`Handler` -> `Service` -> `Repository` -> `Database`

- **cmd/api**: Entry point. Wires everything together (Dependency Injection).
- **internal/config**: Configuration loading (Env vars).
- **internal/handler**: HTTP handlers (Parse request, call service, write response).
- **internal/service**: Business logic. Defines interfaces for repositories.
- **internal/repository**: Data access. Implements interfaces defined by services.
- **internal/database**: Database connection setup.
- **pkg**: Shared public code.

## Tech Stack

- **Go 1.22+**: Using standard `net/http` with the new ServeMux.
- **PostgreSQL**: Using `pgx/v5` driver.
- **Configuration**: `godotenv`.
- **Logging**: `log/slog` (Structured Logging).

## Getting Started

1. Copy `.env.example` to `.env`:
   ```sh
   cp .env.example .env
   ```
2. Run the server:
   ```sh
   make run
   # or
   go run cmd/api/main.go
   ```

## Best Practices Implemented

- **Dependency Injection**: Dependencies are injected via constructors (not globals).
- **Graceful Shutdown**: Server handles SIGTERM/SIGINT to finish active requests.
- **Structured Logging**: JSON logs in production, text in development.
- **Timeout Management**: Database and Server timeouts configured.
