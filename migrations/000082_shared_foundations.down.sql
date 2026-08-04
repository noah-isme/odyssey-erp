-- Phase 0: Shared Foundations for Documents, CMMS, and QMS
-- Migration 000082: Shared object storage and module foundations (DOWN)

-- Drop in reverse order of creation

-- Seed data removal
DELETE FROM document_categories WHERE code = 'GENERAL';
DELETE FROM document_numbering_rules WHERE code = 'DEFAULT';
DELETE FROM document_classifications WHERE code IN ('PUBLIC', 'INTERNAL', 'CONFIDENTIAL', 'RESTRICTED');

-- Company feature flags
DROP TABLE IF EXISTS company_module_features;

-- Outbox events
DROP TABLE IF EXISTS outbox_events;

-- Document access events
DROP TABLE IF EXISTS document_access_events;

-- Disposition requests
DROP TABLE IF EXISTS disposition_requests;

-- Legal hold references
DROP TABLE IF EXISTS legal_hold_references;

-- Legal holds
DROP TABLE IF EXISTS legal_holds;

-- Document retention
DROP TABLE IF EXISTS document_retention;

-- Retention policies
DROP TABLE IF EXISTS retention_policies;

-- Document signatures
DROP TABLE IF EXISTS document_signatures;

-- Document signature challenges
DROP TABLE IF EXISTS document_signature_challenges;

-- Document review decisions
DROP TABLE IF EXISTS document_review_decisions;

-- Document review steps
DROP TABLE IF EXISTS document_review_steps;

-- Document links
DROP TABLE IF EXISTS document_links;

-- Document ACLs
DROP TABLE IF EXISTS document_acls;

-- Document versions
DROP TABLE IF EXISTS document_versions;

-- Documents (drop FK first)
ALTER TABLE documents DROP CONSTRAINT IF EXISTS fk_documents_current_version;
DROP TABLE IF EXISTS documents;

-- Document numbering rules
DROP TABLE IF EXISTS document_numbering_rules;

-- Document categories
DROP TABLE IF EXISTS document_categories;

-- Document classifications
DROP TABLE IF EXISTS document_classifications;

-- Storage blobs
DROP TABLE IF EXISTS storage_blobs;