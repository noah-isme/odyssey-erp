DROP INDEX IF EXISTS uq_quotations_crm_opportunity;
ALTER TABLE quotations DROP COLUMN IF EXISTS crm_opportunity_id;
