# Documentation Policy

## Scope and authority

Current documentation lives in `README.md`, `AGENTS.md`, and the non-archive
content under `docs/`. The authoritative sources are:

| Topic | Source |
| --- | --- |
| Supported capabilities | `docs/reference/module-catalog.md` |
| Future work | `docs/ROADMAP.md` |
| Released changes | `docs/CHANGELOG.md` and `docs/releases/` |
| Setup and commands | `README.md`, `QUICK_REFERENCE.md`, and `docs/getting-started/` |
| Architecture and development practices | `docs/architecture/`, `docs/decisions/`, and `AGENTS.md` |

## Keeping documents current

Every active document must describe the current codebase, use working links and
commands, and distinguish implemented behavior from planned work. A feature
claim should identify its handler, route, migration, test, or other concrete
implementation evidence.

Update the relevant source when a change affects a route, environment variable,
Make target, migration, supported capability, or release status. Run
`make docs-check` after documentation changes.

## Historical material

Completed phase notes, audits, one-off plans, and superseded runbooks belong in
`docs/archive/`. Archive material is historical only and must not be presented
as current setup, feature, or contributor guidance.
