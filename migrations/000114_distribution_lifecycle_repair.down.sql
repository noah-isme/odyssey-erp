ALTER TABLE transfer_orders
    DROP CONSTRAINT IF EXISTS transfer_transport_assignment;

ALTER TABLE transfer_orders
    ADD CONSTRAINT transfer_transport_assignment CHECK (
        (vehicle_id IS NOT NULL AND driver_id IS NOT NULL AND carrier_id IS NULL) OR
        (vehicle_id IS NULL AND driver_id IS NULL AND carrier_id IS NOT NULL)
    );
