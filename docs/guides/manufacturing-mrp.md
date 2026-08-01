# Manufacturing / MRP

## Current status

**Partial.** The Horizon foundation exposes company-scoped MRP BOMs and work orders. Routes are mounted under `/mrp`; local acceptance evidence is linked from [`horizon-mvp.md`](horizon-mvp.md).

## Supported scope

- Bill of Materials (BOM) records.
- Work orders and lifecycle/idempotency controls.
- Company isolation and links to products and inventory records.

## Not currently documented as supported

Production schedules, machine schedules, capacity planning, demand-driven material calculation, WIP tracking, finished-goods completion, production time, and yield analytics. These require a separate production-planning design before being described as implemented.
