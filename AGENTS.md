# Repository Guidelines

## Project Structure & Module Organization
Odyssey is a Go modular monolith. Key paths:
- `cmd/odyssey` HTTP server entrypoint; `cmd/worker` Asynq worker.
- `internal/` domain modules (auth, sales, procurement, etc.), app wiring in `internal/app`, shared helpers in `internal/shared`.
- `jobs/` background job wiring; `report/` Gotenberg client + handlers.
- `web/templates` SSR pages and partials; `web/static` CSS assets.
- `migrations/` schema changes; `sql/queries` and `sql/schema` feed sqlc.
- `docs/` architecture/guides, `scripts/` automation, `testing/` test helpers.

## Build, Test, and Development Commands
- `docker compose up -d` or `make dev` – start app + dependencies.
- `~/go/bin/air` – hot reload (see `QUICK_REFERENCE.md`).
- `make lint` – run `golangci-lint`.
- `make test` – run Go tests; `make build` – compile binaries.
- `make migrate-up`/`make migrate-down` – apply/rollback migrations.
- `make seed` – load default data; `make sqlc-gen` – regenerate SQL bindings.
- `./tools/scripts/run.sh` – run services without Docker (foreground).

## Coding Style & Naming Conventions
- Follow Go conventions: `gofmt` formatting, `go vet`, `golangci-lint`.
- Package names are lowercase; exported identifiers use CamelCase.
- Tests use `*_test.go` and table-driven patterns (see `docs/guides/handlers.md`).
- SQL lives in `sql/queries` with `-- name: QueryName` blocks for sqlc.

## Testing Guidelines
- Set `ODYSSEY_TEST_MODE=1` and `GOTENBERG_URL=http://127.0.0.1:0` for fast, isolated runs.
- Unit tests: `go test ./...` or `make test`.
- Integration suites live beside modules (e.g., `internal/.../integration_test.go`) and use `testify/suite`; run with `go test -tags=integration ./...` or target package.

## Commit & Pull Request Guidelines
- Commit style follows conventional commits: `type(scope): summary` (e.g., `feat(ui): ...`, `refactor(backend): ...`).
- PRs should include a clear description, test results, and linked issues; add screenshots for `web/` UI changes and update docs when behavior changes.

## Security & Configuration Tips
- Keep secrets out of git; configure via env vars like `PG_DSN`, `REDIS_ADDR`, and `GOTENBERG_URL`.
- Default credentials in `README.md` are for local development only.

## AI & Token Limit Guidelines
To prevent "The model's generation exceeded the maximum output token limit" errors, the AI must strictly adhere to the following rules:
- **Use Artifacts for Long Outputs**: Always use `write_to_file` with `IsArtifact: true` for long documents, walkthroughs, or plans. Never output full code files or large blocks directly in the chat.
- **Concise Code References**: Do not use "Print full file" commands. Use line numbers and specific ranges (e.g., `file.go:10-50`) instead of dumping entire files in the chat.
- **Incremental Steps**: Break down massive tasks (e.g., full feature creation) into sequential steps with check-ins. Request user approval when a natural pausing point is reached to clear the token buffer.
- **Efficient Logging**: Never dump huge terminal logs (like `npm install` or full test outputs) into the chat. Pipe outputs to a file or limit characters read. Use `view_file` instead of `cat` via bash.
- **Concise Reporting**: Keep responses short and focus only on actionable updates. Avoid narrating previously written code.
