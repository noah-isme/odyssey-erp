# Behavioral fixtures for mutating routes — Framework + Tier 1 (master data)

## Context

The e2e regression suite (`tests/e2e/regression_test.go`) derives its work list from the
binary's own chi route dump, so it stays honest as routes come and go. Today it proves two
things about the ~111 mutating routes:

1. **Negative path (all routes):** `assertMutationsRequireCSRF` sends a tokenless request to
   every mutating route and asserts `403`. This proves a route *cannot be driven without a
   token* — it says nothing about whether the route actually works.
2. **Positive path (one route):** `assertGuardedMutationSucceedsWithToken` is the *only*
   behavioral write — it creates a unit with a valid token and re-fetches the listing to
   confirm the row appears.

So 110 of 111 mutations have zero behavioral coverage: a handler could 500 on every valid
submit and the suite would stay green. This plan builds a **reusable fixture framework** and
populates it with **Tier 1: the 8 master-data resources** (create / update / delete — ~24
routes), which are flat form-encoded POSTs with a fully-mapped field contract. Tiers 2
(document lifecycle chains) and 3 (external side effects) reuse the same framework and are
described as a follow-on at the end.

## The framework

Add to `tests/e2e/regression_test.go` (package `e2e`). One shared state object threads
self-created IDs between fixtures so children can reference real parents:

```go
type fixtureCtx struct {
    t       *testing.T
    client  *http.Client
    baseURL string
    created map[string]string // fixture name -> id it produced (e.g. "company" -> "42")
}

type mutationFixture struct {
    name    string                                        // "create company"
    route   string                                        // "POST /masterdata/companies" — matched against the dump
    needs   []string                                      // fixture names that must run first (FK ordering)
    arrange func(c *fixtureCtx) (path string, form url.Values) // concrete path + fields, using c.created for FKs
    wantMax int                                           // highest acceptable status (303 for redirect-on-success)
    assert  func(c *fixtureCtx)                           // re-fetch and prove the effect; may stash a new id in c.created
}
```

**Runner** `assertMutationFixturesSucceed(t, client, baseURL, routes []routeEntry, fixtures []mutationFixture)`:

- Topologically sort `fixtures` by `needs` (small Kahn's-algorithm loop; fatal on cycle).
- For each fixture: resolve `arrange` → `(path, form)`, call `fetchCSRF(t, client, baseURL+formPage)`
  to get a live token, inject it under the token field the templates emit, then `postForm`.
  Assert status `<= wantMax` and that the body carries no `serverErrorMarkers`. Run `assert`.
- **Coverage tracking:** build the set of `POST` patterns each fixture's `route` matches; diff
  against `mutatingRoutes(routes)`. `t.Logf` every mutating route with no fixture — mirroring the
  file's existing "fail rather than silently lose coverage" discipline. (Log, not fail, since
  Tiers 2/3 are intentionally deferred.)

**Reuse (do not reinvent):** `fetchCSRF` (:695), `postForm` (:773), `get` (:752, absorbs one
429), `fetchPage` (:406, follows redirects), `harvestLinks` (:358), `pace` (:723, 45/min),
`serverErrorMarkers` (:631), `routeEntry`/`mutatingRoutes` (:180/:522). The existing
`assertGuardedMutationSucceedsWithToken` unit fixture is folded into this framework as the first
entry and then deleted.

## Tier 1 fixtures — the 8 master-data resources

All share one route shape (confirmed): `POST <res>/` (create), `POST <res>/{id}/edit` (update),
`POST <res>/{id}/delete`. Create/update parse identical fields via `r.PostFormValue`. Success is
a `303` redirect. Every `code` uses a unique `E2E-<nanos>` value (as the existing units fixture
does) to stay idempotent across reruns. Deletes/updates only ever target self-created IDs from
`c.created` — never `unreachableID`, which stays exclusive to the guard sweep.

Field contract (required fields → `arrange` form values):

| Resource (`<res>`) | Create fields | FK needs |
|---|---|---|
| `/masterdata/units` | code, name | — |
| `/masterdata/categories` | code, name | — |
| `/masterdata/taxes` | code, name, rate | — |
| `/masterdata/companies` | code, name (+opt address, tax_id) | — |
| `/masterdata/suppliers` | code, name (+opt email, phone) | — |
| `/masterdata/branches` | code, name, **company_id** | company |
| `/masterdata/warehouses` | code, name, **branch_id** | branch (→ company) |
| `/masterdata/products` | code, name, **category_id**, **unit_id**, price, cost_method (+opt tax_id, supplier) | category + unit |

Seed order falls out of `needs`: `units, categories, taxes, companies, suppliers` (any order) →
`branches` → `warehouses`, `products`.

**Capturing the new ID for FK reuse:** create handlers redirect to the listing, not the detail
page, so the `assert` step GETs the listing, `harvestLinks` it, and picks the row whose edit/detail
link is new (or matches the unique `E2E-<nanos>` code), stashing that id in `c.created`. This reuses
the same link-harvesting the detail sweep already relies on.

**Per resource, three fixtures:**
- *create* → POST fields above; assert the unique code appears in the listing; stash id.
- *update* → POST to `/{id}/edit` for the just-created id with a changed `name`; assert the new
  name renders on the detail/listing.
- *delete* → POST to `/{id}/delete` for the just-created id; assert the row is gone from the listing.

Ordering the delete *last per resource* (after update) keeps each resource self-cleaning, except
where a child depends on it (e.g. don't delete the company a branch still references — gate via
`needs` so the parent delete runs after the child delete, or skip parent deletes whose children
persist).

Handler references for the field contract (read before writing each `arrange`):
`internal/masterdata/{units,categories,taxes,companies,suppliers,branches,warehouses,products}/handler.go`
(Create/Update near the top of each) and the matching `validation.go`. **Products caveat:**
`category_id`/`unit_id` are enforced only by the DB FK, not `validate()` — omitting them yields a
500-class error, so the create fixture must always supply real ids from `c.created`.

## Wiring into the flow

`TestRegressionFlow` already logs in, sweeps GETs, harvests links, then runs the CSRF guard sweep.
Add the fixture run **after** the guard sweep (so we know tokens are enforced before we start
driving writes), passing the same authenticated `client`, `baseURL`, and the parsed route dump.
No CI change needed — `.github/workflows/ci.yml` already builds the binary, dumps routes to
`routes.json`, seeds (`make seed-phase4`), starts the server, and runs `TestRegressionFlow`; the
new assertions ride the existing invocation.

## Files

- **Edit** `tests/e2e/regression_test.go` — add `fixtureCtx`, `mutationFixture`, the runner, the
  Tier 1 fixture slice, and the call site in `TestRegressionFlow`; remove the now-subsumed
  `assertGuardedMutationSucceedsWithToken`.
- **Edit** `tests/e2e/routes_test.go` — add table-driven unit tests for the new pure logic (the
  topological sort and the fixture↔route coverage diff), matching the file's existing
  `funcName(%q) = ..., want ...` convention. These run without a live server.

## Verification

1. **Unit (no server):** `go test ./tests/e2e/ -run 'TestMutationFixture|TestFixtureTopo' -v` —
   proves the sort and coverage-diff logic in isolation.
2. **End-to-end (real server), mirroring CI locally:**
   - `docker compose up -d postgres redis` (host ports 5432 / 6380 per `docker-compose.yml`).
   - `migrate -path migrations -database "$PG_DSN" up` then `make seed-phase4` (creates
     `admin@odyssey.local` / `admin123`).
   - `go build -o bin/odyssey ./cmd/odyssey`
   - `ODYSSEY_TEST_MODE=0 ODYSSEY_DUMP_ROUTES=1 SESSION_SECRET=… CSRF_SECRET=… ./bin/odyssey > routes.json`
   - Start server: `ODYSSEY_TEST_MODE=0 APP_ADDR=127.0.0.1:8080 SESSION_SECRET=… CSRF_SECRET=… ./bin/odyssey &`; poll `GET /healthz`.
   - `ODYSSEY_E2E_URL=http://127.0.0.1:8080 ODYSSEY_E2E_ROUTES=$PWD/routes.json go test ./tests/e2e/ -run TestRegressionFlow -v -timeout 20m`
3. **Confirm the coverage log** lists exactly the Tier 2/3 routes as unfixtured (no master-data
   route should appear) — this is the proof Tier 1 is complete and the framework sees the rest.

## Follow-on (not in this plan)

- **Tier 2 — document lifecycle chains:** sales/purchasing/inventory/accounting `submit`/`approve`/
  `convert`/`post`/`void` transitions. Each becomes a fixture whose `needs` chains it after the
  create that produces a document in the right state; `arrange` reads the doc id from `c.created`.
- **Tier 3 — external side effects:** email/import/PDF/settings. Fixtures assert the call succeeds
  (and, where a sink exists like Mailpit at `:8025`, that the effect landed) rather than internal
  state.

Both reuse `fixtureCtx`/`mutationFixture`/the runner unchanged — only new fixture entries and,
for Tier 2, prior exploration of those handlers' form contracts.
