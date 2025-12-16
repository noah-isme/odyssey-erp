# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git make

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/odyssey ./cmd/odyssey
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/worker ./cmd/worker

# Runtime stage
FROM alpine:latest

WORKDIR /app

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Copy binaries from builder
COPY --from=builder /app/odyssey /app/odyssey
COPY --from=builder /app/worker /app/worker

# Copy migrations and other necessary files
COPY --from=builder /app/migrations ./migrations
COPY --from=builder /app/web ./web

# Expose port
EXPOSE 8080

# Run application
CMD ["/app/odyssey"]
