# ADR-0002: Role-Based Access Control

## Status

Accepted

## Context

Phase 2 introduces multi-team collaboration features that require granular control over who can access master and organization data. Phase 1 shipped basic authentication but lacked authorization beyond simple "is logged in" checks. We now need a scalable RBAC approach that supports:

- Explicit roles with human-readable names.
- Fine-grained permissions that can be reused across roles.
- User-role assignments that may evolve over time.
- Auditable changes so security reviews can trace responsibility.
- Server-side enforcement that is testable and compatible with SSR handlers.

Existing infrastructure already provides PostgreSQL, SQLC for data access, and chi-based HTTP handlers. The design must integrate with these tools while keeping the mental model simple for implementers.

## Decision

We implement a relational RBAC model using four tables: `roles`, `permissions`, `role_permissions`, and `user_roles`. Services expose operations for managing roles, attaching permissions, and evaluating a user's effective permission set. Middleware inspects the authenticated user stored in the request context and verifies that the user possesses either *any* or *all* permissions required by a handler. Permission names are normalized as lowercase dot-separated strings (e.g. `master.view`).

Data access is generated with SQLC to ensure type safety and to centralize SQL definitions. The RBAC service wraps the generated queries with business rules such as preventing duplicate assignments and recording audit logs in follow-up work. Authoritative checks are provided in `internal/rbac/middleware.go` as reusable helpers `RequireAny` and `RequireAll`.

## Consequences

- The schema can be extended without data migrations by adding new permissions and roles.
- Authorization logic is centralized, allowing handlers to remain small and declarative.
- Tests can cover the authorization matrix by constructing fixtures in the RBAC tables.
- Role and permission management flows can be built incrementally since the data model already supports them.
- The middleware layer depends on the request context containing an authenticated user object with ID information; handlers must continue to enforce authentication before authorization.

## Alternatives Considered

### Hard-coded role checks

Rejected because it would quickly become unmaintainable as more features are added. Database-driven assignments provide the flexibility required by real deployments.

### Attribute-based access control (ABAC)

Deferred because it is more complex than needed for Phase 2. RBAC satisfies the current requirements, and we can layer ABAC rules later if necessary.

## Follow-up Work

- Build SSR management pages for roles and permissions.
- Extend the audit logging subsystem to capture RBAC mutations.
- Document onboarding steps for granting permissions to new hires.
