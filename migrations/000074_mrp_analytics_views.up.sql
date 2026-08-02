CREATE OR REPLACE VIEW mrp_analytics_operation_daily AS
SELECT op.company_id,DATE(COALESCE(op.completed_at,op.updated_at)) AS day,wo.product_id,wo.routing_id,op.work_center_id,op.code AS operation_code,
 SUM(op.good_quantity) AS good_quantity,SUM(op.scrap_quantity) AS scrap_quantity,
 SUM(op.actual_setup_minutes) AS actual_setup_minutes,SUM(op.actual_run_minutes) AS actual_run_minutes,
 SUM(op.planned_setup_minutes) AS planned_setup_minutes,SUM(op.planned_run_minutes) AS planned_run_minutes
FROM mrp_work_order_operations op JOIN mrp_work_orders wo ON wo.id=op.work_order_id
WHERE op.status='COMPLETED' GROUP BY op.company_id,DATE(COALESCE(op.completed_at,op.updated_at)),wo.product_id,wo.routing_id,op.work_center_id,op.code;
CREATE OR REPLACE VIEW mrp_analytics_wip_value AS
SELECT location.company_id,location.warehouse_id AS source_warehouse_id,location.wip_warehouse_id,
 balance.product_id,balance.qty,balance.avg_cost,(balance.qty*balance.avg_cost) AS value,
 MIN(movement.created_at) AS first_issued_at
FROM mrp_wip_locations location JOIN inventory_balances balance ON balance.warehouse_id=location.wip_warehouse_id
LEFT JOIN mrp_material_movements movement ON movement.company_id=location.company_id AND movement.destination_warehouse_id=location.wip_warehouse_id AND movement.product_id=balance.product_id AND movement.movement_type='ISSUE'
WHERE location.active GROUP BY location.company_id,location.warehouse_id,location.wip_warehouse_id,balance.product_id,balance.qty,balance.avg_cost;
CREATE OR REPLACE VIEW mrp_analytics_schedule_adherence AS
SELECT op.company_id,op.work_center_id,wo.product_id,op.id AS operation_id,op.scheduled_end_at,op.completed_at,
 CASE WHEN op.completed_at IS NULL THEN NULL WHEN op.scheduled_end_at IS NULL THEN NULL WHEN op.completed_at<=op.scheduled_end_at THEN TRUE ELSE FALSE END AS on_time
FROM mrp_work_order_operations op JOIN mrp_work_orders wo ON wo.id=op.work_order_id;
CREATE OR REPLACE VIEW mrp_analytics_work_center_utilization AS
SELECT shift.company_id,shift.work_center_id,DATE(op.scheduled_start_at) AS day,
 COALESCE(SUM(op.planned_setup_minutes+op.planned_run_minutes),0)/60.0 AS scheduled_hours,
 COALESCE(SUM(shift.capacity_hours),0) AS available_hours
FROM mrp_work_center_shifts shift LEFT JOIN mrp_work_order_operations op ON op.work_center_id=shift.work_center_id AND DATE(op.scheduled_start_at)=CURRENT_DATE AND op.status NOT IN ('COMPLETED')
WHERE shift.active GROUP BY shift.company_id,shift.work_center_id,DATE(op.scheduled_start_at);
