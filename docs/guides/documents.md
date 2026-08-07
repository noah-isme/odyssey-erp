# Document Management

## Current status

**Full Implementation.** Odyssey ERP offers an enterprise-grade document control and management system. Core implementation is in `internal/documents/`.

## Supported scope

- **Documents & Versions:** Hierarchical storage with category organization and version control.
- **Classifications & Security:** Document classification levels (Public, Internal, Confidential, Restricted) backed by granular Access Control Lists (ACLs) defining Read, Write, Admin, Approve, and Sign permissions.
- **Numbering Rules:** Auto-generation of document numbers based on configurable patterns (prefixes, suffixes, and sequences).
- **Review Workflows:** Multi-step review and approval workflows with role or user assignments.
- **E-Signatures:** Secure electronic signatures with cryptographic challenges, hashing, and meaning tracking (compliant with standard e-signature regulations).
- **Retention & Legal Holds:** Automated retention policies with disposition actions (Archive, Delete) and explicit Legal Holds that prevent document destruction.
- **Audit Trails:** Comprehensive logging of document access events (views, downloads, shares, prints).
- **Cross-Module Linking:** Capabilities to link documents as references or attachments to records in other modules (e.g., Sales, Procurement).

## Gaps

Real-time collaborative editing (like Google Docs), Optical Character Recognition (OCR) for scanned PDFs, and deep content indexing/search inside file attachments are not currently native features.
