.PHONY: run-api run-worker build test tidy docker-up docker-down docker-logs migrate-up migrate-down migrate-create

-include .env

DB_DSN ?= postgres://taskuser:taskpass@localhost:5432/taskdb?sslmode=disable

run: run-api

run-api:
	go run cmd/api/main.go

run-worker:
	go run cmd/worker/main.go

build:
	mkdir -p bin
	go build -o bin/api cmd/api/main.go
	go build -o bin/worker cmd/worker/main.go

test:
	go test -v ./...

tidy:
	go mod tidy

docker-up:
	docker-compose up -d --remove-orphans

infra-up:
	docker-compose up -d postgres redis rabbitmq minio

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f

migrate-up:
	migrate -path migrations -database "$(DB_DSN)" up

migrate-down:
	migrate -path migrations -database "$(DB_DSN)" down

migrate-create:
	@read -p "Enter migration name: " name; \
	migrate create -ext sql -dir migrations -seq $$name

clean:
	rm -rf bin
	go clean
	docker-compose down -v --remove-orphans
