CREATE TABLE IF NOT EXISTS document_sequences (
    company_id BIGINT NOT NULL,
    doc_type TEXT NOT NULL,
    period TEXT NOT NULL,
    seq BIGINT NOT NULL,
    PRIMARY KEY (company_id, doc_type, period)
);
