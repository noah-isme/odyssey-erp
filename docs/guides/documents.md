# Document Management

## Current status

**Implemented foundation; advanced processing is partial.** Odyssey ERP offers the
document control, managed-storage, versioning, approval, signature, retention, and
permission foundations in `internal/documents/`. OCR and collaboration now have
durable worker/repository paths, but binary OCR and a realtime transport still need
deployment-specific providers.

## Supported scope

- **Documents & Versions:** Hierarchical storage with category organization and version control.
- **Classifications & Security:** Document classification levels (Public, Internal, Confidential, Restricted) backed by granular Access Control Lists (ACLs) defining Read, Write, Admin, Approve, and Sign permissions.
- **Numbering Rules:** Auto-generation of document numbers based on configurable patterns (prefixes, suffixes, and sequences).
- **Review Workflows:** Multi-step review and approval workflows with role or user assignments.
- **E-Signatures:** Secure electronic signatures with cryptographic challenges, hashing, and meaning tracking (compliant with standard e-signature regulations).
- **Retention & Legal Holds:** Automated retention policies with disposition actions (Archive, Delete) and explicit Legal Holds that prevent document destruction.
- **Audit Trails:** Comprehensive logging of document access events (views, downloads, shares, prints).
- **Cross-Module Linking:** Capabilities to link documents as references or attachments to records in other modules (e.g., Sales, Procurement).
- **OCR and Search:** OCR jobs are persisted and dispatched through Asynq. Text-based
  blobs are extracted by the built-in worker and indexed across title, keywords, and
  content with tenant-scoped PostgreSQL full-text search. A configured OCR extractor
  is required for scanned PDFs and images.
- **Collaboration Persistence:** Sessions use cryptographically random tokens and
  immutable, company-scoped change records. A websocket/realtime delivery layer is
  intentionally outside the document service.
- **Retention Worker:** Expired schedules become pending disposition requests. The
  scheduled disposition worker checks version-specific legal holds before deleting
  managed storage and archiving the version; approval remains explicit.

## Gaps

Scanned-PDF/image OCR still requires an injected OCR provider; the default extractor
rejects binary input rather than returning fabricated text. Realtime collaborative
editing and websocket fan-out are not included in the persistence layer. The current
disposition executor archives versions and removes their managed blob; richer policy
actions and blob-reference garbage collection remain follow-up work.
