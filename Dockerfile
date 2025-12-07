# Stage 1: Builder
FROM golang:1.25-alpine AS builder

# Install SSL ca-certificates (required if calling HTTPS endpoints)
RUN apk update && apk add --no-cache ca-certificates git

WORKDIR /app

# Download dependencies first (cached if go.mod/go.sum don't change)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
# CGO_ENABLED=0 creates a statically linked binary suitable for scratch containers
RUN CGO_ENABLED=0 GOOS=linux go build -o /api cmd/api/main.go

# Stage 2: Runner
FROM scratch

WORKDIR /app

# Copy CA certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary
COPY --from=builder /api /app/api

# Copy .env.example as .env or expect env vars to be injected
# Note: In production k8s/docker, env vars are usually injected directly, 
# but copying .env is possible if strictly needed. 
# We'll rely on env var injection for pure Docker.

# Expose port
EXPOSE 8080

# Run
CMD ["/app/api"]
