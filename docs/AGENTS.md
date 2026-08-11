# Odyssey ERP: Agent Configuration & Guidelines

## Overview
This document provides comprehensive guidelines for AI agents working within the Odyssey ERP codebase. It outlines the architecture, rules, workflows, and best practices that agents must follow to maintain code quality, consistency, and stability. Note that this file (`docs/AGENTS.md`) is the detailed operational guide for agents, supplementing the high-level repository guidelines found in the root `AGENTS.md`.

## Agent Architecture & Tools

### Available Skills
When interacting with this repository, agents have access to the following skills:
- **`frontend-standards`**: Enforces strict separation of concerns (HTML, CSS, JS).
- **`graphify`**: Enables knowledge graph queries to understand codebase relationships. Use `graphify update .` after making structural code changes.
- **`ci-workflows`**: Simulates Continuous Integration checks to ensure changes meet repository standards before pushing.
- **`git-safeguards`**: Ensures safe git practices and consistent commit formatting.

### Key Rules & Patterns
1. **Modular Monolith**: Code is organized as a modular monolith under `internal/`.
2. **Module Structure**: Every module must contain `handler.go` and `routes.go`, and should utilize `sqlc`-generated queries for database interactions.
3. **Template Engine**: HTML templates are located in `web/templates/` and must leverage layout inheritance.
4. **Strict Asset Separation**:
   - CSS resides in `web/static/css/` — **never use inline styles**.
   - JS resides in `web/static/js/` — **never use inline scripts**.
5. **Database Access**: SQL queries live in `sql/queries/` with `sqlc` annotations.
6. **Migrations**: Database migrations are stored in `migrations/` and must use sequential numbering.
7. **Commit Conventions**: Always use Conventional Commits (e.g., `type(scope): summary`).

## Development Workflow for Agents
Agents should follow this standard development cycle:
1. **Start Dependencies**: `docker compose up -d`
2. **Hot Reloading**: Run `~/go/bin/air` to automatically reload the application on code changes.
3. **Database Changes**: Run `make migrate-up` to apply schema changes, followed by `make sqlc-gen` if queries were modified.
4. **Testing**: Run `make test` to execute the test suite.
5. **Linting**: Run `make lint` to verify code quality.

## Module Development Guide
When creating a new module:
1. Create a new directory under `internal/` (e.g., `internal/inventory/`).
2. Implement HTTP handlers in `handler.go`.
3. Define route registrations in `routes.go`.
4. Register the new module's routes in `app/routes.go`.
5. Create necessary SQL queries in `sql/queries/` and run `make sqlc-gen`.
6. Create views in `web/templates/` as needed.

## Database Change Guide
When modifying the database schema:
1. Create a new migration file in `migrations/` ensuring the sequential numbering is maintained.
2. Write the UP and DOWN SQL statements.
3. Run `make migrate-up`.
4. Update or add SQL queries in `sql/queries/` with proper `sqlc` annotations.
5. Run `make sqlc-gen` to regenerate the Go bindings.

## UI Change Guide
When modifying the user interface:
- **HTML**: Modify templates in `web/templates/`. Ensure changes respect the layout inheritance.
- **Styling**: Modify or add CSS in `web/static/css/`. **Do not add inline `style` attributes** or `<style>` blocks in templates.
- **Interactivity**: Modify or add JS in `web/static/js/`. **Do not add inline event handlers** (e.g., `onclick`) or `<script>` blocks in templates.

## Testing Guide
- Ensure the test environment is properly configured:
  - `ODYSSEY_TEST_MODE=1`
  - `GOTENBERG_URL=http://127.0.0.1:0` (if applicable for PDF generation tests)
- Write tests in `*_test.go` files alongside the code being tested.
- Follow the **table-driven testing** pattern for Go.
- Run `make test` to verify all tests pass before completing a task.

## Common Tasks & Recipes
- **Adding a new page**:
  1. Create the template in `web/templates/`.
  2. Add a handler method in the relevant module's `handler.go`.
  3. Register the route in the module's `routes.go`.
- **Adding a background job**:
  1. Define the job logic in the `jobs/` directory.
  2. Register the job in the worker configuration.
- **Documentation Maintenance**:
  1. Keep `docs/` and the root `AGENTS.md` up-to-date.
  2. Move outdated documentation to `docs/archive/`.
  3. Run `make docs-check` to validate documentation changes.

## Debugging Tips
- If UI styles are not updating, verify that you didn't accidentally use inline styles and that the browser cache is clear (or use incognito/cache-busting during dev).
- If `sqlc` bindings are missing or incorrect, ensure you ran `make sqlc-gen` after modifying the queries.
- If tests are failing unexpectedly, verify that `ODYSSEY_TEST_MODE=1` is correctly set in the environment.

## Anti-Patterns to Avoid
- **Inline CSS/JS**: Never embed CSS or JS within HTML templates.
- **Bypassing `sqlc`**: Do not write raw `database/sql` queries in handlers; always use `sqlc` generated code.
- **Skipping Migrations**: Never modify the database schema directly without a corresponding migration file.
- **Monolithic Handlers**: Do not put all logic in the handler; separate business logic where appropriate.
- **Ignoring Conventional Commits**: Refrain from arbitrary commit messages; strict adherence to conventional commits is required.
