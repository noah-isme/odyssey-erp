# Build stage
FROM golang:1.25.0-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git make

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build application
RUN CGO_ENABLED=0 GOOS=linux go build -tags production -a -installsuffix cgo -o /app/odyssey ./cmd/odyssey
RUN CGO_ENABLED=0 GOOS=linux go build -tags production -a -installsuffix cgo -o /app/worker ./cmd/worker
RUN CGO_ENABLED=0 GOOS=linux go build -tags production -a -installsuffix cgo -o /app/bootstrap-admin ./cmd/bootstrap-admin
RUN CGO_ENABLED=0 GOOS=linux go build -tags production -a -installsuffix cgo -o /app/seed ./scripts/seed
RUN GOBIN=/app go install -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.3

# Runtime stage
FROM alpine:3.22

WORKDIR /app

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata
RUN addgroup -S odyssey && adduser -S -G odyssey odyssey

# Copy binaries from builder
COPY --from=builder /app/odyssey /app/odyssey
COPY --from=builder /app/worker /app/worker
COPY --from=builder /app/bootstrap-admin /app/bootstrap-admin
COPY --from=builder /app/seed /app/seed
COPY --from=builder /app/migrate /app/migrate

# Copy migrations and other necessary files
COPY --from=builder /app/migrations ./migrations
COPY --from=builder /app/web ./web

RUN chown -R odyssey:odyssey /app

USER odyssey

# Expose port
EXPOSE 8080 9091

# Run application
CMD ["/app/odyssey"]
