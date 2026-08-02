DROP INDEX IF EXISTS idx_mrp_work_order_operations_schedule;
DROP TABLE IF EXISTS mrp_schedule_exceptions;
DROP TABLE IF EXISTS mrp_operation_dependencies;
ALTER TABLE mrp_work_order_operations DROP COLUMN IF EXISTS scheduled_by,DROP COLUMN IF EXISTS schedule_sequence,DROP COLUMN IF EXISTS schedule_manual,DROP COLUMN IF EXISTS scheduled_end_at,DROP COLUMN IF EXISTS scheduled_start_at;
DROP TABLE IF EXISTS mrp_work_center_calendar_exceptions;
DROP TABLE IF EXISTS mrp_work_center_shifts;
