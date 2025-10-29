PROJECT_NAME=odyssey-erp
GO_BIN?=go
SQLC_BIN?=sqlc
MIGRATE_BIN?=migrate
PERIOD?=$(shell date +%Y-%m)
COMPANY_ID?=1
BRANCH_ID?=
BRANCH_QUERY=$(if $(BRANCH_ID),&branch_id=$(BRANCH_ID),)

export APP_ENV?=development
export PG_DSN?=postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable

.PHONY: dev lint test build migrate-up migrate-down sqlc-gen seed seed-phase3 seed-phase4 refresh-mv reports-demo pdf-sample analytics-dashboard analytics-dashboard-pdf analytics-dashboard-csv prom-up grafana-load alert-test monitor-demo release-phase6

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

analytics-dashboard:
	curl -fsS "http://localhost:8080/finance/analytics?period=$(PERIOD)&company_id=$(COMPANY_ID)$(BRANCH_QUERY)"

analytics-dashboard-pdf:
	curl -fsS -o /tmp/analytics-dashboard.pdf "http://localhost:8080/finance/analytics/pdf?period=$(PERIOD)&company_id=$(COMPANY_ID)$(BRANCH_QUERY)"
	test -s /tmp/analytics-dashboard.pdf

analytics-dashboard-csv:
        curl -fsS -o /tmp/analytics-dashboard.csv "http://localhost:8080/finance/analytics/export.csv?period=$(PERIOD)&company_id=$(COMPANY_ID)$(BRANCH_QUERY)"
        test -s /tmp/analytics-dashboard.csv

prom-up:
        @echo "Starting observability stack (Prometheus + Grafana)"
        @echo "Run: docker compose -f deploy/observability/docker-compose.yml up -d"

grafana-load:
        @echo "Provisioning dashboards from deploy/grafana/dashboards"
        @echo "Use grafana-dashboard-tooling or API to upload JSON definitions."

alert-test:
        $(GO_BIN) test ./internal/observability -run TestFinanceAlertRules -count=1
        $(GO_BIN) test ./internal/e2e -run TestAlertSimulationProducesFiringAndResolvedLogs -count=1

monitor-demo:
        $(GO_BIN) test ./internal/perf -run TestFinanceLatencyTargets -count=1
        $(GO_BIN) test ./internal/perf -run TestAnalyticsJobThroughputAndReliability -count=1

release-phase6: lint test build
        @echo "Phase 6 release checklist complete. Tag with v0.6.0-final."
