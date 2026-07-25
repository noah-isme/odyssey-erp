ALTER TABLE report_schedules ADD COLUMN period_offset_months INTEGER NOT NULL DEFAULT 0;
CREATE TABLE report_schedule_deliveries (
    id BIGSERIAL PRIMARY KEY,
    schedule_id BIGINT NOT NULL REFERENCES report_schedules(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('QUEUED','FAILED')),
    detail TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_report_schedule_deliveries_schedule ON report_schedule_deliveries(schedule_id, created_at DESC);
