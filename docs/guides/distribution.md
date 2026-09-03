# Distribution planning and load execution

## Current status

The distribution module exposes a usable core outbound lifecycle. It is mounted
under `/distribution` and coordinates planning-owned records with the existing
logistics and inventory services.

The supported core flow is:

```text
planning horizon → load → shipment line → ready → dispatch → in transit
→ delivery → inventory adjustment → existing inventory/GL integration
```

`DispatchLoad` advances linked logistics shipments to `IN_TRANSIT`, and
`DeliverLoad` posts one deterministic negative inventory adjustment per load
item before marking the shipments and load delivered. The application adapter
keeps SQLC and logistics repository types out of the distribution package.

## HTTP endpoints

All endpoints require an authenticated session with a `company_id` scope.

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/distribution/planning/horizons` | List active horizons |
| `POST` | `/distribution/planning/horizons` | Create a warehouse horizon |
| `POST` | `/distribution/planning/rules` | Add a capacity, weight, time-window, vehicle, or custom rule |
| `GET` | `/distribution/loads` | List company loads; accepts `status` |
| `POST` | `/distribution/loads` | Create a draft load |
| `GET` | `/distribution/loads/{id}` | Read a load and its items |
| `POST` | `/distribution/loads/{id}/shipments` | Create a logistics shipment and link its lines to the load |
| `POST` | `/distribution/loads/{id}/ready` | Validate rules and mark the load ready |
| `POST` | `/distribution/loads/{id}/dispatch` | Assign carrier/service or vehicle/driver and start transit |
| `POST` | `/distribution/loads/{id}/deliver` | Post outbound inventory and complete delivery |
| `GET` | `/distribution/loads/{id}/utilization` | Return weight, volume, and item utilization |
| `POST` | `/distribution/routes` | Create a route for a ready/planned load |
| `POST` | `/distribution/routes/{id}/stops` | Add a manually sequenced stop |
| `POST` | `/distribution/routes/{id}/optimize` | Validate stops and mark a route optimized |
| `POST` | `/distribution/routes/{id}/approve` | Approve an optimized route |
| `GET` | `/distribution/routes/{id}/metrics` | Return stop completion metrics |
| `GET` | `/distribution/transfers` | List transfer orders; accepts `status` |
| `POST` | `/distribution/transfers` | Create a draft transfer order |
| `POST` | `/distribution/transfers/{id}/lines` | Add a transfer line |
| `POST` | `/distribution/transfers/{id}/approve` | Validate and approve a transfer |
| `POST` | `/distribution/transfers/{id}/dispatch` | Assign transport and start transit |
| `POST` | `/distribution/transfers/{id}/receive` | Receive all requested quantities |

JSON error responses use the shared HTTP error boundary and never expose raw
database errors.

## Persistence notes

Migration `000080_distribution_planning_phase5` creates the planning, load,
route, and transfer tables. Migration
`000114_distribution_lifecycle_repair` permits draft and approved transfer
orders to exist without transport assignment; dispatched and later states still
require either a vehicle plus driver or a carrier.

The repository uses typed dates, times, and exact `NUMERIC` values. Load, route,
and transfer numbers are assigned from the inserted row ID rather than relying
on undeclared database sequences. Queries also match the actual migration
columns: the child tables do not have `updated_at`, and routes do not have an
`actual_duration_minutes` column.

## Inventory and GL behavior

Distribution does not write journals directly. The application adapter calls
`inventory.Service.PostAdjustment` with `RefModule: "DISTRIBUTION"` and a
deterministic reference code. The inventory service invokes its configured
integration hooks, which resolve the open accounting period and inventory
adjustment mappings before posting GL. A production environment must therefore
have the normal inventory accounts and mappings configured before delivery is
enabled.

## Tests

The normal package suite contains a complete lifecycle test with fake module
ports. The database-backed lifecycle is opt-in and uses the real distribution,
logistics, and inventory services:

```bash
make migrate-up
DISTRIBUTION_TEST_DSN="$PG_DSN" \
  go test -tags=integration ./internal/distribution \
  -run TestDatabaseLoadLifecycle -v
```

Remaining work includes transfer-order inventory movements and GL treatment,
route distance/resequencing optimization, proof of delivery, carrier API
execution, freight costing, and planner/dispatcher SSR workbenches.
