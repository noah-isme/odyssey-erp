-- Phase 6: Freight Finance rollback
-- Drop dependent tables before the rate-card and freight-charge parents.
-- Deliberately omit CASCADE so an unexpected later schema dependency blocks
-- rollback instead of being removed implicitly.

DROP TABLE IF EXISTS freight_audit_log;
DROP TABLE IF EXISTS landed_costs;
DROP TABLE IF EXISTS freight_charges;
DROP TABLE IF EXISTS rate_surcharges;
DROP TABLE IF EXISTS rate_cards;
