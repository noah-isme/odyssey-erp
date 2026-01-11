# Odyssey ERP Handler Guidelines

These guidelines standardize how we build server-rendered HTTP handlers during Phase 2.

## 1. Structure and Flow

1. **Authentication & Authorization**
   - Mount handlers behind the shared authentication middleware.
   - Use `rbac.RequireAny` / `rbac.RequireAll` helpers to guard permissions early.
2. **Parse Input**
   - Prefer dedicated request structs with explicit fields.
   - Decode form data with `r.ParseForm()` before accessing values.
3. **Validate**
   - Reuse `shared.Validate` utilities (coming in Phase 2) backed by go-playground/validator.
   - Always whitelist sort columns and filter keys.
4. **Execute Use Case**
   - Delegate to a service layer that wraps SQLC repositories.
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
- Avoid leaking database errors directly to the client.

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
