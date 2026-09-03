# ADR-0013: Fiscal Calendars and Timezone Policy

## Status

Accepted

## Context

Odyssey currently handles accounting periods via a mix of global `periods` and partially company-aware `accounting_periods`. Timezones and locales are loosely defined, often relying on server-local time or implicit UTC assumptions.

For enterprise compliance and multi-company operations, we require strict, versioned fiscal calendars (e.g., 4-4-5, shifted years, custom adjustment periods) and unambiguous rules for timezone handling across UI display, operational cutoffs, and accounting dates.

## Decision

We will implement governed fiscal calendars and a formal timezone/locale policy:

1. **Timezone Policy**:
   - Store all event/audit timestamps as UTC `TIMESTAMPTZ`. Display them in the user's timezone, falling back to the company timezone.
   - Accounting and document dates are stored as `DATE` and never shifted by viewer timezone.
   - Company operational deadlines are evaluated and scheduled against the company's timezone.
2. **Fiscal Calendars**:
   - Implement company-scoped fiscal calendar versions, years, and periods.
   - Support standard calendar months, shifted months, 4-4-5/4-5-4 patterns, and custom adjustment periods.
   - A period containing posted journals cannot be deleted, resized, or reordered.
3. **Period Resolver**:
   - Consolidate legacy global period lookups behind a new company-scoped Period Resolver service.
   - Comparative reports will resolve prior periods by fiscal sequence rather than calendar-month string arithmetic.

## Consequences

- **Data Integrity**: Complete isolation of fiscal policies between companies. Timezone ambiguity is eliminated.
- **Migration Effort**: Safely migrating legacy global `periods` to company-scoped periods requires meticulous reconciliation and a dedicated rollout phase to avoid breaking existing journals or subledgers.
- **Complexity**: Reporting and background jobs must proactively resolve company context and timezones rather than relying on system time.

## Follow-up Work

- Define the IANA timezone and BCP 47 locale data structures.
- Implement the fiscal calendar and period schema.
- Audit and map all legacy period callers to the new Period Resolver.
- Build migration fixtures and dual-verification paths for posting and reporting.
