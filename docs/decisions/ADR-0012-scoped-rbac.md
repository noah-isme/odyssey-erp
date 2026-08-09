# ADR-0012: Scoped Role-Based Access Control

## Status

Accepted

## Context

ADR-0002 established a foundational relational RBAC model with global user-role assignments. As Odyssey scales to support multi-company deployments, organizational hierarchies, and more sensitive modules (HR, payroll, manufacturing), a purely global assignment model is insufficient. Users often act as administrators in one company branch but have restricted or no access in another.

We need a permission model that scopes authorization to specific boundaries (company, branch) and supports versioned policies, separation of duties (SoD), and auditable access reviews.

## Decision

We will supersede global user-role assignments with scoped assignments while retaining the global permission catalog:

1. **Scoped Assignments**: User-role links must explicitly define their scope (company, and optionally branch). Middleware and business services will require the active company scope for authorization lookups.
2. **Role Templates**: Introduce versioned system role templates (e.g., Finance Controller, Warehouse Operator) that companies can adopt or customize.
3. **Effective-Dated Assignments**: Role assignments will support start/end timestamps for temporary access.
4. **Separation of Duties (SoD)**: The system will enforce conflict rules (e.g., preventing a user from having both payment proposal and execution roles simultaneously).
5. **Migration**: Existing global `user_roles` links will be migrated to explicit company-scoped assignments without removing current access.

## Consequences

- **Flexibility**: Multi-company support is native to the authorization model.
- **Security**: Granular control limits blast radius and enforces high-risk workflow boundaries.
- **Complexity**: Middleware and services must pass and evaluate scope context alongside permissions. Legacy callers relying on global checks must be refactored.

## Follow-up Work

- Implement the scoped assignment tables and versioned role templates.
- Update RBAC middleware to mandate company scope resolution.
- Develop migration fixtures to safely transform legacy global assignments.
- Build SSR management views for role matrices, conflicts, and access reviews.
