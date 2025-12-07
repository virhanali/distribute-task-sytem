.PHONY: run build test tidy docker-up docker-down docker-logs migrate-up migrate-down

# Load .env file if it exists
-include .env

# Default DSN for local migration commands (running on host, connection to localhost:5432)
# This overrides the one in .env if it uses the service name 'postgres'
DB_DSN ?= postgres://taskuser:taskpass@localhost:5432/taskdb?sslmode=disable

run:
	go run cmd/api/main.go

build:
	go build -o bin/api cmd/api/main.go

test:
	go test -v ./...

tidy:
	go mod tidy

# Docker Compose Commands
docker-up:
	docker-compose up -d --remove-orphans

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f

# Database Migration Commands
# Requires https://github.com/golang-migrate/migrate
migrate-up:
	migrate -path migrations -database "$(DB_DSN)" up

migrate-down:
	migrate -path migrations -database "$(DB_DSN)" down

migrate-create:
	@read -p "Enter migration name: " name; \
	migrate create -ext sql -dir migrations -seq $$name
