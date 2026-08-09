---
type: "query"
date: "2026-08-09T07:33:00.698380+00:00"
question: "Are the 180 inferred relationships involving SessionFromContext() (e.g. with .auditRecord() and .companyID()) actually correct?"
contributor: "graphify"
outcome: "useful"
source_nodes: ["SessionFromContext()", "Context", "Session", "Middleware", "CSRFTokenFromContext()"]
---

# Q: Are the 180 inferred relationships involving SessionFromContext() (e.g. with .auditRecord() and .companyID()) actually correct?

## Answer

Expanded from original query via vocab: [session, relationships, context, handler, service, repository, domain, job]. Graph node SessionFromContext() is at internal/shared/context.go:L13 and has degree 183; 180 call edges are INFERRED, while only 3 are EXTRACTED. Source audit finds 191 direct production call sites: 173 handler/UI/API and 18 app/RBAC/shared infrastructure, with none in services, repositories, domains, or jobs. Conclusion: relationships are mostly real, but provenance is wrong; Graphify is under-resolving Go selector calls. Odyssey has handler-boundary coupling, not cross-layer session leakage.

## Outcome

- Signal: useful

## Source Nodes

- SessionFromContext()
- Context
- Session
- Middleware
- CSRFTokenFromContext()