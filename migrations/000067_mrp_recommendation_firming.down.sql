DROP INDEX IF EXISTS idx_mrp_planning_recommendations_firmed;
ALTER TABLE mrp_planning_recommendations
    DROP COLUMN IF EXISTS firmed_pr_id,
    DROP COLUMN IF EXISTS firmed_work_order_id;
DROP INDEX IF EXISTS idx_mrp_work_orders_planning_recommendation;
ALTER TABLE mrp_work_orders
    DROP COLUMN IF EXISTS planning_recommendation_id,
    DROP COLUMN IF EXISTS planned_due_date,
    DROP COLUMN IF EXISTS planned_start_date;
