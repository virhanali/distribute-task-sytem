.PHONY: run build test tidy docker-up docker-down

run:
	go run cmd/api/main.go

build:
	go build -o bin/api cmd/api/main.go

test:
	go test -v ./...

tidy:
	go mod tidy

# Simple docker compose helpers if user adds docker-compose.yml later
docker-up:
	docker-compose up -d

docker-down:
	docker-compose down
