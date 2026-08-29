DROP TRIGGER IF EXISTS trg_prevent_ap_exception_resolution_event_mutation
    ON ap_exception_resolution_events;
DROP TRIGGER IF EXISTS trg_validate_ap_exception_resolution_event
    ON ap_exception_resolution_events;
DROP FUNCTION IF EXISTS prevent_ap_exception_resolution_event_mutation();
DROP FUNCTION IF EXISTS validate_ap_exception_resolution_event();
DROP TABLE IF EXISTS ap_exception_resolution_events;
