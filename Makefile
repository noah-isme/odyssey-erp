PROJECT_NAME=odyssey-erp
GO_BIN?=go
VET_PKGS=$(shell $(GO_BIN) list ./... | grep -v "/cmd/odyssey$$")
VET_CONSOL_PKGS=$(shell $(GO_BIN) list ./internal/consol/...)
SQLC_BIN?=$(HOME)/go/bin/sqlc
MIGRATE_BIN?=$(HOME)/go/bin/migrate
AIR_BIN?=$(HOME)/go/bin/air
PERIOD?=$(shell date +%Y-%m)
COMPANY_ID?=1
CERT_GO_TMP?=/tmp/odyssey-cert-go-tmp
CERT_GO_CACHE?=/tmp/odyssey-cert-go-cache
GROUP_ID?=1
ENTITIES?=all
FX_MODE?=on
FX_PAIR?=IDRUSD
FX_FROM?=2024-01
FX_TO?=2025-12
FX_SOURCE?=./rates.csv
BRANCH_ID?=
BRANCH_QUERY=$(if $(BRANCH_ID),&branch_id=$(BRANCH_ID),)

export APP_ENV?=development
export PG_DSN?=postgres://odyssey:odyssey@localhost:5434/odyssey?sslmode=disable

.PHONY: dev air lint vet vet-consol test build docs-check release-check pdf-release-check production-build-check production-release-check midtrans-sandbox-certify migrate-guard migrate-up migrate-down migrate-up-remote migrate-down-remote test-migrate sqlc-gen seed seed-phase3 seed-phase4 seed-realistic refresh-mv reports-demo pdf-sample export-demo fx-tools analytics-dashboard analytics-dashboard-pdf analytics-dashboard-csv prom-up grafana-load alert-test monitor-demo release-phase6

dev:
	docker compose up --build

air:
	@if [ -f .env ]; then set -a && . ./.env && set +a; fi; \
	$(AIR_BIN)

lint:
	golangci-lint run --timeout=5m ./...

vet:
	@echo "==> go vet"
	@timeout 180s env \
	GOPROXY=$(if $(GOPROXY),$(GOPROXY),https://proxy.golang.org) \
	GOSUMDB=$(if $(GOSUMDB),$(GOSUMDB),sum.golang.org) \
	GOFLAGS='-mod=readonly' \
	ODYSSEY_TEST_MODE=$(if $(ODYSSEY_TEST_MODE),$(ODYSSEY_TEST_MODE),1) \
	$(GO_BIN) vet $(VET_PKGS)

vet-consol:
	@echo "==> go vet (consol)"
	@timeout 120s env \
	GOPROXY=$(if $(GOPROXY),$(GOPROXY),https://proxy.golang.org) \
	GOSUMDB=$(if $(GOSUMDB),$(GOSUMDB),sum.golang.org) \
	GOFLAGS='-mod=readonly' \
	ODYSSEY_TEST_MODE=$(if $(ODYSSEY_TEST_MODE),$(ODYSSEY_TEST_MODE),1) \
	$(GO_BIN) vet $(VET_CONSOL_PKGS)

test:
	$(GO_BIN) test ./...

build:
	$(GO_BIN) build ./...

docs-check:
	bash scripts/check-docs.sh

release-check: docs-check
	bash scripts/check-release-hygiene.sh

pdf-release-check:
	$(GO_BIN) test -tags "production pdf" ./internal/consol/http
	$(GO_BIN) build -tags "production pdf" ./...

production-build-check: release-check
	$(GO_BIN) test ./...
	$(GO_BIN) vet ./...
	CGO_ENABLED=0 GOOS=linux $(GO_BIN) build -tags production ./...
	$(GO_BIN) test -tags "production pdf" ./internal/consol/http
	CGO_ENABLED=0 GOOS=linux $(GO_BIN) build -tags "production pdf" ./...

production-release-check: production-build-check
	bash scripts/check-production-release.sh

midtrans-sandbox-certify:
	@mkdir -p "$(CERT_GO_TMP)" "$(CERT_GO_CACHE)"
	GOTMPDIR="$(CERT_GO_TMP)" GOCACHE="$(CERT_GO_CACHE)" ODYSSEY_TEST_MODE=1 GOTENBERG_URL='http://127.0.0.1:0' $(GO_BIN) test ./internal/connectors/providers/midtrans -run '^TestMidtransSandboxCertification$$' -count=1 -v

# migrate-guard refuses to run unless the effective PG_DSN (command line,
# environment, or the default above) points at localhost, 127.0.0.1, or ::1.
# The host is parsed from the authority component, so a lookalike in the
# password or path cannot satisfy the check. libpq/pq let query parameters
# override the URL host (postgres://localhost/db?host=prod, also hostaddr= and
# service=) and keyword/value DSNs carry host= directly, so any DSN that
# mentions those keys is refused outright (it fails closed, even for a
# password that happens to contain "host=").
migrate-guard:
	@dsn="$$PG_DSN"; lc=$$(printf '%s' "$$dsn" | tr 'A-Z' 'a-z'); \
	case "$$lc" in \
		*host=*|*hostaddr=*|*service=*) echo "refusing to migrate: PG_DSN must not set host=, hostaddr= or service= (they override the URL host); use migrate-up-remote/migrate-down-remote for deliberate deploys" >&2; exit 1;; \
	esac; \
	rest="$${dsn#*://}"; auth="$${rest%%[/?]*}"; hostport="$${auth##*@}"; \
	case "$$hostport" in \
		localhost|localhost:*|127.0.0.1|127.0.0.1:*|\[::1\]|\[::1\]:*) ;; \
		*) echo "refusing to migrate non-local host; set PG_DSN explicitly (use migrate-up-remote/migrate-down-remote for deliberate deploys)" >&2; exit 1;; \
	esac

migrate-up: migrate-guard
	$(MIGRATE_BIN) -path migrations -database "$(PG_DSN)" up

migrate-down: migrate-guard
	$(MIGRATE_BIN) -path migrations -database "$(PG_DSN)" down 1

# Unguarded variants for deliberate deploys against non-local databases.
migrate-up-remote:
	$(MIGRATE_BIN) -path migrations -database "$(PG_DSN)" up

migrate-down-remote:
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

seed-realistic:
	$(GO_BIN) run ./scripts/seed/realistic/...

refresh-mv:
	$(GO_BIN) run ./scripts/finance/refreshmv/main.go

reports-demo:
	$(GO_BIN) run ./scripts/finance/reportsdemo/main.go

export-demo:
	curl -fsS -o /tmp/consol-pl.csv "http://localhost:8080/finance/consol/pl/export.csv?group=$(GROUP_ID)&period=$(PERIOD)&entities=$(ENTITIES)&fx=$(FX_MODE)"
	test -s /tmp/consol-pl.csv
	curl -fsS -o /tmp/consol-pl.pdf "http://localhost:8080/finance/consol/pl/pdf?group=$(GROUP_ID)&period=$(PERIOD)&entities=$(ENTITIES)&fx=$(FX_MODE)"
	test -s /tmp/consol-pl.pdf
	curl -fsS -o /tmp/consol-bs.csv "http://localhost:8080/finance/consol/bs/export.csv?group=$(GROUP_ID)&period=$(PERIOD)&entities=$(ENTITIES)&fx=$(FX_MODE)"
	test -s /tmp/consol-bs.csv
	curl -fsS -o /tmp/consol-bs.pdf "http://localhost:8080/finance/consol/bs/pdf?group=$(GROUP_ID)&period=$(PERIOD)&entities=$(ENTITIES)&fx=$(FX_MODE)"
	test -s /tmp/consol-bs.pdf
	@echo "Exports saved to /tmp/consol-pl.{csv,pdf} and /tmp/consol-bs.{csv,pdf}"

fx-tools:
	@echo "Validate FX coverage:";
	@echo "  odyssey fx validate --group $(GROUP_ID) --period $(PERIOD) --pair $(FX_PAIR) --json";
	@echo ""
	@echo "Preview FX backfill candidates:";
	@echo "  odyssey fx backfill --pair $(FX_PAIR) --from $(FX_FROM) --to $(FX_TO) --source $(FX_SOURCE) --mode dry";

fx-fetch:
	$(GO_BIN) run ./cmd/odyssey fx fetch --date "$(FX_DATE)"

fx-status:
	$(GO_BIN) run ./cmd/odyssey fx status --date "$(FX_DATE)"

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

test-migrate: migrate-guard
	@echo "test-migrate dummy"
migrate-status:
	@echo "migrate-status dummy"
seed-production:
	@echo "seed-production dummy"
