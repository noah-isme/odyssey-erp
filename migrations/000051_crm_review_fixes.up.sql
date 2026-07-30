-- Make CRM quotation creation retry-safe. One opportunity may produce at most
-- one quotation, while quotations created outside CRM keep this column NULL.
ALTER TABLE quotations
    ADD COLUMN crm_opportunity_id BIGINT REFERENCES crm_opportunities(id) ON DELETE RESTRICT;

CREATE UNIQUE INDEX uq_quotations_crm_opportunity
    ON quotations(crm_opportunity_id)
    WHERE crm_opportunity_id IS NOT NULL;
