CREATE TABLE IF NOT EXISTS mrp_work_center_shifts (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    work_center_id BIGINT NOT NULL REFERENCES mrp_work_centers(id) ON DELETE CASCADE,
    weekday SMALLINT NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    start_time TIME NOT NULL,
    end_time TIME NOT NULL,
    capacity_hours NUMERIC(8,2) NOT NULL CHECK (capacity_hours > 0),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    UNIQUE(work_center_id, weekday, start_time),
    CHECK(end_time > start_time)
);
CREATE TABLE IF NOT EXISTS mrp_work_center_calendar_exceptions (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    work_center_id BIGINT NOT NULL REFERENCES mrp_work_centers(id) ON DELETE CASCADE,
    exception_date DATE NOT NULL,
    exception_type TEXT NOT NULL CHECK(exception_type IN ('HOLIDAY','MAINTENANCE','CAPACITY_OVERRIDE')),
    capacity_hours NUMERIC(8,2) NOT NULL DEFAULT 0 CHECK(capacity_hours >= 0),
    note TEXT NOT NULL DEFAULT '',
    UNIQUE(work_center_id, exception_date)
);
ALTER TABLE mrp_work_order_operations
    ADD COLUMN IF NOT EXISTS scheduled_start_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS scheduled_end_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS schedule_manual BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS schedule_sequence INT,
    ADD COLUMN IF NOT EXISTS scheduled_by BIGINT REFERENCES users(id) ON DELETE RESTRICT;
CREATE TABLE IF NOT EXISTS mrp_operation_dependencies (
    operation_id BIGINT NOT NULL REFERENCES mrp_work_order_operations(id) ON DELETE CASCADE,
    predecessor_operation_id BIGINT NOT NULL REFERENCES mrp_work_order_operations(id) ON DELETE CASCADE,
    PRIMARY KEY(operation_id, predecessor_operation_id),
    CHECK(operation_id <> predecessor_operation_id)
);
CREATE TABLE IF NOT EXISTS mrp_schedule_exceptions (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    operation_id BIGINT NOT NULL REFERENCES mrp_work_order_operations(id) ON DELETE CASCADE,
    exception_type TEXT NOT NULL CHECK(exception_type IN ('OVERLOAD','LATE','MISSING_CAPACITY','DEPENDENCY')),
    detail TEXT NOT NULL,
    resolved_at TIMESTAMPTZ,
    resolved_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_mrp_work_order_operations_schedule ON mrp_work_order_operations(work_center_id,scheduled_start_at);
