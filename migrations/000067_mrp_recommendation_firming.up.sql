ALTER TABLE mrp_work_orders
    ADD COLUMN IF NOT EXISTS planned_start_date DATE,
    ADD COLUMN IF NOT EXISTS planned_due_date DATE,
    ADD COLUMN IF NOT EXISTS planning_recommendation_id BIGINT REFERENCES mrp_planning_recommendations(id) ON DELETE RESTRICT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_mrp_work_orders_planning_recommendation
    ON mrp_work_orders(planning_recommendation_id)
    WHERE planning_recommendation_id IS NOT NULL;

ALTER TABLE mrp_planning_recommendations
    ADD COLUMN IF NOT EXISTS firmed_work_order_id BIGINT REFERENCES mrp_work_orders(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS firmed_pr_id BIGINT REFERENCES prs(id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS idx_mrp_planning_recommendations_firmed
    ON mrp_planning_recommendations(status) WHERE status='PLANNED';
