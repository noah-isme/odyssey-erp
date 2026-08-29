-- AP exception resolution evidence. Resolution comments are captured as
-- append-only events instead of being folded into the mutable workbench row.
-- The company is stored as an immutable tenant snapshot so history remains
-- scoped even if legacy invoice ownership is repaired later.
CREATE TABLE ap_exception_resolution_events (
    id BIGSERIAL PRIMARY KEY,
    ap_exception_id BIGINT NOT NULL REFERENCES ap_exceptions(id) ON DELETE RESTRICT,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    from_status TEXT NOT NULL CHECK (from_status IN ('OPEN', 'IN_REVIEW')),
    to_status TEXT NOT NULL CHECK (to_status IN ('RESOLVED', 'REJECTED')),
    comment TEXT NOT NULL DEFAULT '',
    actor_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ap_exception_resolution_events_one_per_exception
        UNIQUE (ap_exception_id)
);

CREATE INDEX idx_ap_exception_resolution_events_company_time
    ON ap_exception_resolution_events(company_id, created_at ASC, id ASC);
CREATE INDEX idx_ap_exception_resolution_events_exception_time
    ON ap_exception_resolution_events(ap_exception_id, created_at ASC, id ASC);

-- Resolution events must carry the tenant derived from the exception's
-- invoice (with the supplier fallback used by the workbench for legacy rows).
-- The atomic SQLC transition supplies and constrains the status values; the
-- table checks below preserve those values in the append-only record.
CREATE OR REPLACE FUNCTION validate_ap_exception_resolution_event()
RETURNS TRIGGER AS $$
DECLARE
    expected_company BIGINT;
BEGIN
    SELECT COALESCE(i.company_id, s.company_id)
      INTO expected_company
      FROM ap_exceptions e
      JOIN ap_invoices i ON i.id = e.ap_invoice_id
      JOIN suppliers s ON s.id = i.supplier_id
     WHERE e.id = NEW.ap_exception_id;

    IF expected_company IS NULL OR NEW.company_id <> expected_company THEN
        RAISE EXCEPTION 'AP exception resolution event company does not match exception owner';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION prevent_ap_exception_resolution_event_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'AP exception resolution events are immutable';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_validate_ap_exception_resolution_event
    ON ap_exception_resolution_events;
CREATE TRIGGER trg_validate_ap_exception_resolution_event
    BEFORE INSERT ON ap_exception_resolution_events
    FOR EACH ROW EXECUTE FUNCTION validate_ap_exception_resolution_event();

DROP TRIGGER IF EXISTS trg_prevent_ap_exception_resolution_event_mutation
    ON ap_exception_resolution_events;
CREATE TRIGGER trg_prevent_ap_exception_resolution_event_mutation
    BEFORE UPDATE OR DELETE ON ap_exception_resolution_events
    FOR EACH ROW EXECUTE FUNCTION prevent_ap_exception_resolution_event_mutation();
