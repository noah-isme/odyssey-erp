-- Complete the persistence foundations used by the document and CMMS services.

CREATE TABLE doc_collaboration_changes (
    id BIGSERIAL PRIMARY KEY,
    session_id BIGINT NOT NULL REFERENCES doc_collaboration_sessions(id) ON DELETE CASCADE,
    actor_id BIGINT NOT NULL REFERENCES users(id),
    operation VARCHAR(20) NOT NULL,
    payload TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (operation IN ('INSERT', 'DELETE', 'REPLACE'))
);

CREATE INDEX idx_doc_collaboration_changes_session
    ON doc_collaboration_changes(session_id, occurred_at, id);

-- Search indexes are version-scoped. Keep one row per version so OCR retries
-- replace the previous index entry instead of multiplying search results.
DELETE FROM doc_search_indices older
USING doc_search_indices newer
WHERE older.document_version_id = newer.document_version_id
  AND older.id < newer.id;

CREATE UNIQUE INDEX uq_doc_search_indices_version
    ON doc_search_indices(document_version_id);

-- The original content-only index remains valid for existing deployments. This
-- expression index also covers title and keywords used by the completed query.
CREATE INDEX idx_doc_search_fts
    ON doc_search_indices USING gin(
        to_tsvector('english', coalesce(title, '') || ' ' || coalesce(content, '') || ' ' || coalesce(keywords, ''))
    );
