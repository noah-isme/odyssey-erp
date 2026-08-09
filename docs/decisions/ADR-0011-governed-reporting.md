# ADR-0011: Governed Reporting and Dashboards

## Status

Accepted

## Context

Odyssey currently provides finance analytics, accounting reports, and fixed operational views. However, users increasingly need the ability to build custom reports, schedule deliveries, and personalize dashboards. Simply exposing raw SQL or unconstrained reporting tools is unacceptable due to security risks (tenant data leakage, permission bypass) and performance risks (unbounded lookbacks, expensive cross-joins).

We need a reporting architecture that empowers users while enforcing strict governance, dataset lifecycle management, and row/field-level security.

## Decision

We will implement a governed reporting semantic layer and a safe query compiler.

1. **Dataset Catalog**: Datasets are registered in code (not as SQL by users). Each dataset declares its dimensions, measures, authoritative tables, data types, and mandatory scope predicates (e.g., company, branch).
2. **Safe Query Compiler**: User-defined reports store structured definitions, never raw SQL. The compiler loads the published dataset version, intersects actor permissions (e.g., HR field classifications), injects mandatory row-scope predicates, and generates safe parameterized SQL.
3. **Dashboards**: Widgets will reference published report versions. Caches will be keyed by permission fingerprint, scope, and filters, ensuring a broader-scope cache is never reused for a narrower user.
4. **Distribution**: Scheduled reports and exports will execute against fixed dataset versions and evaluate recipient permissions before every delivery.

## Consequences

- **Security**: Complete prevention of arbitrary SQL injection and unauthorized data access through reports.
- **Reliability**: Query compilation estimates costs and applies row/timeout limits, protecting the primary database.
- **Maintenance**: Adding new fields or tables requires developer intervention to register them in the dataset catalog.
- **Limitations**: Users cannot perform cross-dataset ad-hoc joins or write arbitrary SQL.

## Follow-up Work

- Implement the dataset catalog and register the first certified datasets.
- Build the safe query compiler and structured report builder UI.
- Implement dashboard templates and scope-aware caching.
