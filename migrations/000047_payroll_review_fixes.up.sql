CREATE EXTENSION IF NOT EXISTS btree_gist;

ALTER TABLE payroll_rule_versions
    ADD CONSTRAINT payroll_rule_versions_no_reviewed_overlap
    EXCLUDE USING gist (
        rule_type WITH =,
        daterange(effective_from, COALESCE(effective_to, 'infinity'::DATE), '[]') WITH &&
    ) WHERE (reviewed_at IS NOT NULL);

ALTER TABLE payroll_company_policies
    ADD CONSTRAINT payroll_company_policies_no_overlap
    EXCLUDE USING gist (
        company_id WITH =,
        daterange(effective_from, COALESCE(effective_to, 'infinity'::DATE), '[]') WITH &&
    );

CREATE TABLE payroll_run_events (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES payroll_runs(id) ON DELETE RESTRICT,
    event_type TEXT NOT NULL CHECK (event_type IN ('REJECTED')),
    actor_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX payroll_run_events_run_idx ON payroll_run_events(run_id, created_at DESC);
