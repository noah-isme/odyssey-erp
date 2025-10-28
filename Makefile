PROJECT_NAME=odyssey-erp
GO_BIN?=go
SQLC_BIN?=sqlc
MIGRATE_BIN?=migrate

export APP_ENV?=development
export PG_DSN?=postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable

.PHONY: dev lint test build migrate-up migrate-down sqlc-gen seed seed-phase3 seed-phase4 refresh-mv reports-demo pdf-sample

dev:
	docker compose up --build

lint:
	golangci-lint run ./...

test:
	$(GO_BIN) test ./...

build:
	$(GO_BIN) build ./...

migrate-up:
	$(MIGRATE_BIN) -path migrations -database "$(PG_DSN)" up

migrate-down:
	$(MIGRATE_BIN) -path migrations -database "$(PG_DSN)" down 1

sqlc-gen:
	$(SQLC_BIN) generate

seed:
	$(GO_BIN) run ./scripts/seed/main.go

seed-phase3:
	$(GO_BIN) run ./scripts/seed/main.go
	@echo "Seed data including Phase 3 permissions loaded"

pdf-sample:
	curl -s -o sample.pdf -X POST http://localhost:8080/report/sample -H "Content-Type: application/x-www-form-urlencoded" -d "csrf_token=dummy"

seed-phase4:
	$(GO_BIN) run ./scripts/seed/main.go
	$(GO_BIN) run ./scripts/seed/phase4/main.go
	@echo "Seed data including Phase 4.2 finance mappings loaded"

refresh-mv:
	$(GO_BIN) run ./scripts/finance/refreshmv/main.go

reports-demo:
	$(GO_BIN) run ./scripts/finance/reportsdemo/main.go
