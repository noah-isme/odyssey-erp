# Odyssey ERP Handler Guidelines

These guidelines standardize server-rendered HTTP handlers in Odyssey.

## 1. Structure and Flow

1. **Authentication & Authorization**
   - Mount handlers behind the shared authentication middleware.
   - Use `rbac.RequireAny` / `rbac.RequireAll` helpers to guard permissions early.
2. **Parse Input**
   - Prefer dedicated request structs with explicit fields.
   - Decode form data with `r.ParseForm()` before accessing values.
3. **Validate**
   - Reuse `shared.Validate` utilities backed by go-playground/validator.
   - Always whitelist sort columns and filter keys.
4. **Execute Use Case**
   - Delegate to a service layer that depends on a repository-owned interface.
   - Keep generated `internal/sqlc` types and `pgtype` null/database wrappers inside PostgreSQL repository adapters.
   - Record audit logs for write operations.
5. **Respond**
   - Use Post/Redirect/Get (PRG) to avoid duplicate submissions.
   - Populate flash messages for user feedback.
   - Render templates with `view.Engine` and the shared `TemplateData` struct.

## 2. Pagination, Sorting, and Filtering

- Accept `page`, `per_page`, `sort`, and `direction` query params.
- Sanitize `sort` and `direction` against package-level allow lists.
- Store pagination metadata in `shared.Pagination` and pass it to templates.
- For filters, declare explicit allow lists (e.g. map[string]FilterHandler) and ignore unknown keys.

## 3. Error Handling

- Convert validation issues into user-friendly messages rendered in the template.
- Log unexpected errors with context (user ID, route).
- Never send `err.Error()` or a database/driver error directly to a client.
- For plain-text endpoints, use `shared.WriteError` when the status is inferred or `shared.WriteErrorStatus` when the endpoint has an explicit status contract.
- For JSON endpoints, use `shared.RespondJSONError` or `shared.JSONErrorFrom`; preserve a module-specific response envelope only when the API contract requires it.
- For redirects and form errors, use `shared.UserSafeMessage(err)` after logging the underlying error where useful.

The shared boundary classifies wrapped domain sentinels into `401`, `403`, `400`,
`404`, and `409`; unknown errors return a generic `500` message. Handlers should
wrap or return shared sentinels such as `shared.ErrNotFound`,
`shared.ErrValidation`, and `shared.ErrConflict` so that classification remains
consistent across SSR, JSON, and RFC 7807 responses.

Example:

```go
if err := service.Update(ctx, input); err != nil {
	logger.Error("update record", slog.Any("error", err))
	shared.WriteError(w, err)
	return
}
```

Do not pass `*sqlc.Queries`, `sqlc.*Params`, or `pgtype.*` values through service,
domain, or handler interfaces. Define a local input/row contract and map it in
the repository implementation instead.

## 4. Testing

- Write table-driven tests covering:
  - Authorization matrix.
  - Happy path with valid data.
  - Invalid input (validation + business rules).
  - Pagination/sort/filter combinations.
- Use the existing test helpers to create sessions and CSRF tokens.

## 5. Templates

- Keep templates free of business logic; pass computed values from handlers.
- Share partials for flash messages, pagination controls, and table headers.
- Ensure forms include hidden CSRF token inputs.

Adhering to these patterns keeps our handlers predictable, testable, and easy to maintain.
