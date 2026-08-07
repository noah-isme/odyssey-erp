-- =============================================================================
-- CMMS WORK ORDERS
-- =============================================================================

-- name: InsertWorkOrder :one
INSERT INTO work_orders (company_id, number, title, description, asset_id, location_id, priority, status, category, requester_id, assignee_id, planned_start, planned_end, estimated_hours, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING id;

-- name: GetWorkOrder :one
SELECT wo.id, wo.company_id, wo.number, wo.title, wo.description, wo.asset_id, wo.location_id,
       wo.priority, wo.status, wo.category, wo.requester_id, wo.assignee_id,
       wo.planned_start, wo.planned_end, wo.actual_start, wo.actual_end,
       wo.estimated_hours, wo.actual_hours, wo.created_by, wo.created_at, wo.updated_at,
       a.name AS asset_name, l.name AS location_name
FROM work_orders wo
LEFT JOIN assets a ON wo.asset_id = a.id
LEFT JOIN locations l ON wo.location_id = l.id
WHERE wo.id = $1;

-- name: ListWorkOrders :many
SELECT wo.id, wo.company_id, wo.number, wo.title, wo.description, wo.asset_id, wo.location_id,
       wo.priority, wo.status, wo.category, wo.requester_id, wo.assignee_id,
       wo.planned_start, wo.planned_end, wo.actual_start, wo.actual_end,
       wo.estimated_hours, wo.actual_hours, wo.created_by, wo.created_at, wo.updated_at,
       a.name AS asset_name, l.name AS location_name
FROM work_orders wo
LEFT JOIN assets a ON wo.asset_id = a.id
LEFT JOIN locations l ON wo.location_id = l.id
WHERE wo.company_id = $1
  AND ($2::int8 IS NULL OR wo.asset_id = $2)
  AND ($3::int8 IS NULL OR wo.location_id = $3)
  AND ($4::int8 IS NULL OR wo.assignee_id = $4)
  AND ($5::text IS NULL OR wo.status = $5)
  AND ($6::text IS NULL OR wo.priority = $6)
  AND ($7::text IS NULL OR wo.category = $7)
  AND ($8::timestamptz IS NULL OR wo.planned_start >= $8)
  AND ($9::timestamptz IS NULL OR wo.planned_end <= $9)
ORDER BY wo.planned_start ASC NULLS LAST
LIMIT $10 OFFSET $11;

-- name: UpdateWorkOrder :exec
UPDATE work_orders
SET title = $2, description = $3, asset_id = $4, location_id = $5, priority = $6,
    category = $7, assignee_id = $8, planned_start = $9, planned_end = $10,
    estimated_hours = $11, updated_at = NOW()
WHERE id = $1;

-- name: UpdateWorkOrderStatus :exec
UPDATE work_orders
SET status = $2, updated_at = NOW()
WHERE id = $1;

-- name: CountWorkOrdersWithPrefix :one
SELECT COUNT(*) FROM work_orders WHERE company_id = $1 AND number LIKE $2 || '%';

-- =============================================================================
-- WORK ORDER TASKS
-- =============================================================================

-- name: InsertWorkOrderTask :one
INSERT INTO work_order_tasks (work_order_id, sequence, title, description, status, assignee_id, estimated_hours)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id;

-- name: GetWorkOrderTask :one
SELECT id, work_order_id, sequence, title, description, status, assignee_id,
       estimated_hours, actual_hours, completed_at, created_at, updated_at
FROM work_order_tasks
WHERE id = $1;

-- name: ListWorkOrderTasks :many
SELECT id, work_order_id, sequence, title, description, status, assignee_id,
       estimated_hours, actual_hours, completed_at, created_at, updated_at
FROM work_order_tasks
WHERE work_order_id = $1
ORDER BY sequence;

-- name: UpdateWorkOrderTask :exec
UPDATE work_order_tasks
SET title = $2, description = $3, assignee_id = $4, estimated_hours = $5, updated_at = NOW()
WHERE id = $1;

-- name: CompleteWorkOrderTask :exec
UPDATE work_order_tasks
SET status = 'COMPLETED', actual_hours = $2, completed_at = NOW(), updated_at = NOW()
WHERE id = $1;

-- =============================================================================
-- ASSETS
-- =============================================================================

-- name: InsertAsset :one
INSERT INTO assets (company_id, code, name, description, asset_type, parent_id, location_id,
                    manufacturer, model, serial_number, install_date, warranty_expiry, status, criticality)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING id;

-- name: GetAsset :one
SELECT id, company_id, code, name, description, asset_type, parent_id, location_id,
       manufacturer, model, serial_number, install_date, warranty_expiry,
       status, criticality, created_at, updated_at
FROM assets
WHERE id = $1;

-- name: ListAssets :many
SELECT id, company_id, code, name, description, asset_type, parent_id, location_id,
       manufacturer, model, serial_number, install_date, warranty_expiry,
       status, criticality, created_at, updated_at
FROM assets
WHERE company_id = $1
  AND ($2::int8 IS NULL OR location_id = $2)
  AND ($3::text IS NULL OR asset_type = $3)
  AND ($4::text IS NULL OR status = $4)
  AND ($5::text IS NULL OR code ILIKE '%' || $5 || '%' OR name ILIKE '%' || $5 || '%')
ORDER BY code
LIMIT $6 OFFSET $7;

-- name: UpdateAsset :exec
UPDATE assets
SET name = $2, description = $3, location_id = $4, manufacturer = $5, model = $6,
    serial_number = $7, warranty_expiry = $8, status = $9, criticality = $10, updated_at = NOW()
WHERE id = $1;

-- =============================================================================
-- LOCATIONS
-- =============================================================================

-- name: InsertLocation :one
INSERT INTO locations (company_id, code, name, description, parent_id, address, gps_lat, gps_lng, active)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id;

-- name: GetLocation :one
SELECT id, company_id, code, name, description, parent_id, address, gps_lat, gps_lng, active, created_at, updated_at
FROM locations
WHERE id = $1;

-- name: ListLocations :many
SELECT id, company_id, code, name, description, parent_id, address, gps_lat, gps_lng, active, created_at, updated_at
FROM locations
WHERE company_id = $1
ORDER BY code;

-- =============================================================================
-- PREVENTIVE MAINTENANCE SCHEDULES
-- =============================================================================

-- name: InsertPMSchedule :one
INSERT INTO pm_schedules (company_id, asset_id, name, description, frequency_type, frequency_value,
                          meter_reading_type, task_template_id, next_due_date, next_due_meter, active)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id;

-- name: GetPMSchedule :one
SELECT id, company_id, asset_id, name, description, frequency_type, frequency_value,
       meter_reading_type, task_template_id, next_due_date, next_due_meter, active, created_at, updated_at
FROM pm_schedules
WHERE id = $1;

-- name: ListPMSchedules :many
SELECT id, company_id, asset_id, name, description, frequency_type, frequency_value,
       meter_reading_type, task_template_id, next_due_date, next_due_meter, active, created_at, updated_at
FROM pm_schedules
WHERE asset_id = $1
ORDER BY created_at;

-- name: ListDuePMSchedules :many
SELECT id, company_id, asset_id, name, description, frequency_type, frequency_value,
       meter_reading_type, task_template_id, next_due_date, next_due_meter, active, created_at, updated_at
FROM pm_schedules
WHERE company_id = $1
  AND active = true
  AND (next_due_date IS NOT NULL AND next_due_date <= NOW()
       OR next_due_meter IS NOT NULL AND next_due_meter <= (
           SELECT value FROM meter_readings WHERE asset_id = pm_schedules.asset_id
           AND reading_type = pm_schedules.meter_reading_type
           ORDER BY reading_date DESC LIMIT 1
       ))
ORDER BY next_due_date ASC NULLS LAST;

-- name: ListAllDuePMSchedules :many
SELECT id, company_id, asset_id, name, description, frequency_type, frequency_value,
       meter_reading_type, task_template_id, next_due_date, next_due_meter, active, created_at, updated_at
FROM pm_schedules
WHERE active = true
  AND (next_due_date IS NOT NULL AND next_due_date <= NOW()
       OR next_due_meter IS NOT NULL AND next_due_meter <= (
           SELECT value FROM meter_readings WHERE asset_id = pm_schedules.asset_id
           AND reading_type = pm_schedules.meter_reading_type
           ORDER BY reading_date DESC LIMIT 1
       ))
ORDER BY company_id ASC, next_due_date ASC NULLS LAST;

-- name: UpdatePMScheduleNextDue :exec
UPDATE pm_schedules
SET next_due_date = $2, next_due_meter = $3, updated_at = NOW()
WHERE id = $1;

-- =============================================================================
-- TASK TEMPLATES
-- =============================================================================

-- name: InsertTaskTemplate :one
INSERT INTO task_templates (company_id, name, description, category, estimated_hours, instructions, safety_notes)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id;

-- name: GetTaskTemplate :one
SELECT id, company_id, name, description, category, estimated_hours, instructions, safety_notes, created_at, updated_at
FROM task_templates
WHERE id = $1;

-- name: ListTaskTemplates :many
SELECT id, company_id, name, description, category, estimated_hours, instructions, safety_notes, created_at, updated_at
FROM task_templates
WHERE company_id = $1
ORDER BY name;

-- =============================================================================
-- TASK TEMPLATE STEPS
-- =============================================================================

-- name: InsertTaskTemplateStep :exec
INSERT INTO task_template_steps (task_template_id, sequence, title, description, estimated_hours, instructions)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListTaskTemplateSteps :many
SELECT id, task_template_id, sequence, title, description, estimated_hours, instructions, created_at
FROM task_template_steps
WHERE task_template_id = $1
ORDER BY sequence;

-- =============================================================================
-- SPARE PARTS
-- =============================================================================

-- name: InsertSparePart :one
INSERT INTO spare_parts (company_id, code, name, description, category, unit_of_measure,
                         min_quantity, max_quantity, reorder_point, lead_time_days, unit_cost, critical_spare)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING id;

-- name: GetSparePart :one
SELECT id, company_id, code, name, description, category, unit_of_measure,
       min_quantity, max_quantity, reorder_point, lead_time_days, unit_cost, critical_spare, created_at, updated_at
FROM spare_parts
WHERE id = $1;

-- name: ListSpareParts :many
SELECT id, company_id, code, name, description, category, unit_of_measure,
       min_quantity, max_quantity, reorder_point, lead_time_days, unit_cost, critical_spare, created_at, updated_at
FROM spare_parts
WHERE company_id = $1
ORDER BY code;

-- =============================================================================
-- WORK ORDER SPARE PARTS
-- =============================================================================

-- name: InsertWorkOrderSparePart :one
INSERT INTO work_order_spare_parts (work_order_id, spare_part_id, quantity, unit_cost, total_cost)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: GetWorkOrderSparePart :one
SELECT id, work_order_id, spare_part_id, quantity, unit_cost, total_cost, issued_at, issued_by, created_at
FROM work_order_spare_parts
WHERE id = $1;

-- name: ListWorkOrderSpareParts :many
SELECT id, work_order_id, spare_part_id, quantity, unit_cost, total_cost, issued_at, issued_by, created_at
FROM work_order_spare_parts
WHERE work_order_id = $1;

-- name: IssueWorkOrderSparePart :exec
UPDATE work_order_spare_parts
SET issued_at = NOW(), issued_by = $2
WHERE id = $1;

-- =============================================================================
-- METER READINGS
-- =============================================================================

-- name: InsertMeterReading :one
INSERT INTO meter_readings (asset_id, reading_type, value, reading_date, entered_by, notes)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id;

-- name: GetMeterReading :one
SELECT id, asset_id, reading_type, value, reading_date, entered_by, notes, created_at
FROM meter_readings
WHERE id = $1;

-- name: ListMeterReadings :many
SELECT id, asset_id, reading_type, value, reading_date, entered_by, notes, created_at
FROM meter_readings
WHERE asset_id = $1
  AND ($2::text IS NULL OR reading_type = $2)
ORDER BY reading_date DESC
LIMIT $3;


-- name: CreateIoTSensor :one
INSERT INTO cmms_iot_sensors (
    company_id, asset_id, sensor_code, sensor_type, status
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING id;

-- name: UpdateIoTSensorReading :exec
UPDATE cmms_iot_sensors
SET last_reading_at = $2, last_reading_value = $3
WHERE id = $1;

-- name: InsertIoTReading :one
INSERT INTO cmms_iot_readings (
    sensor_id, value, timestamp
) VALUES (
    $1, $2, NOW()
) RETURNING id;

-- name: CreatePredictiveModel :one
INSERT INTO cmms_predictive_models (
    company_id, asset_type, model_name, version, accuracy, is_active
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING id;

-- name: CreatePredictiveAlert :one
INSERT INTO cmms_predictive_alerts (
    company_id, asset_id, sensor_id, model_id, severity, description
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING id;

-- name: ResolvePredictiveAlert :exec
UPDATE cmms_predictive_alerts
SET resolved_at = NOW()
WHERE id = $1;