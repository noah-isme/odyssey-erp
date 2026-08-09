-- name: CreateRouteOptimizationJob :one
INSERT INTO logistics_route_optimization_jobs (
    company_id, trip_id, status, engine, started_at
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING id;

-- name: GetRouteOptimizationJob :one
SELECT * FROM logistics_route_optimization_jobs WHERE id = $1;

-- name: UpdateRouteOptimizationJobStatus :exec
UPDATE logistics_route_optimization_jobs
SET status = $2, error_message = $3, completed_at = $4
WHERE id = $1;

-- name: CreateRouteSequence :one
INSERT INTO logistics_route_sequences (
    optimization_job_id, trip_stop_id, optimized_sequence, estimated_arrival_at, estimated_distance_km
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING id;

-- name: GetRouteSequences :many
SELECT * FROM logistics_route_sequences WHERE optimization_job_id = $1 ORDER BY optimized_sequence ASC;

-- ═══════════════════════════════════════════════════════════════════════════
-- CARRIERS & RATE CARDS
-- ═══════════════════════════════════════════════════════════════════════════

-- name: CreateCarrier :one
INSERT INTO carriers (
    company_id, carrier_name, carrier_code, contact_name, contact_email, contact_phone,
    insurance_provider, insurance_policy_number, insurance_expires_at, created_by, updated_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
) RETURNING id;

-- name: GetCarrier :one
SELECT * FROM carriers WHERE id = $1;

-- name: ListCarriers :many
SELECT * FROM carriers 
WHERE company_id = $1 
  AND ($2::text = '' OR status = $2)
ORDER BY carrier_name ASC;

-- name: UpdateCarrierStatus :exec
UPDATE carriers 
SET status = $2, updated_at = NOW() 
WHERE id = $1;

-- name: CreateCarrierRateCard :one
INSERT INTO carrier_rate_cards (
    company_id, carrier_id, route_from_city, route_to_city, weight_from, weight_to,
    volume_from, volume_to, rate_per_unit, rate_unit, currency, effective_from, effective_to,
    minimum_charge, fuel_surcharge_pct
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
) RETURNING id;

-- name: GetCarrierApplicableRateCard :one
SELECT * FROM carrier_rate_cards
WHERE carrier_id = $1
  AND route_from_city = $2
  AND route_to_city = $3
  AND weight_from <= $4 AND weight_to >= $4
  AND volume_from <= $5 AND volume_to >= $5
  AND effective_from <= CURRENT_DATE
  AND (effective_to IS NULL OR effective_to >= CURRENT_DATE)
ORDER BY effective_from DESC
LIMIT 1;

-- name: ListCarrierRateCards :many
SELECT * FROM carrier_rate_cards
WHERE carrier_id = $1
  AND effective_from <= CURRENT_DATE
ORDER BY effective_from DESC;

-- ═══════════════════════════════════════════════════════════════════════════
-- FLEETS & VEHICLES
-- ═══════════════════════════════════════════════════════════════════════════

-- name: CreateFleet :one
INSERT INTO fleets (
    company_id, fleet_name, fleet_code, fleet_type, warehouse_id, home_city, created_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING id;

-- name: GetFleet :one
SELECT * FROM fleets WHERE id = $1;

-- name: ListFleets :many
SELECT * FROM fleets 
WHERE company_id = $1 
  AND status = 'ACTIVE'
ORDER BY fleet_name ASC;

-- name: UpdateFleetStatus :exec
UPDATE fleets 
SET status = $2, updated_at = NOW() 
WHERE id = $1;

-- name: CreateVehicle :one
INSERT INTO vehicles (
    company_id, fleet_id, vehicle_registration, vehicle_type, license_plate, vin, 
    make, model, year_manufactured, max_weight_kg, max_volume_cbm, insurance_expires_at, 
    gps_device_id, created_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
) RETURNING id;

-- name: GetVehicle :one
SELECT * FROM vehicles WHERE id = $1;

-- name: ListVehiclesByFleet :many
SELECT * FROM vehicles 
WHERE fleet_id = $1 
  AND status IN ('AVAILABLE', 'IN_USE', 'MAINTENANCE')
ORDER BY vehicle_registration ASC;

-- name: ListAvailableVehicles :many
SELECT * FROM vehicles 
WHERE company_id = $1 
  AND status = 'AVAILABLE'
ORDER BY vehicle_registration ASC;

-- name: UpdateVehicleStatus :exec
UPDATE vehicles 
SET status = $2, updated_at = NOW() 
WHERE id = $1;

-- ═══════════════════════════════════════════════════════════════════════════
-- DRIVERS
-- ═══════════════════════════════════════════════════════════════════════════

-- name: CreateDriver :one
INSERT INTO drivers (
    company_id, driver_name, driver_code, email, phone, license_number,
    license_class, license_expires_at, emergency_contact_name, emergency_contact_phone,
    notes, created_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
) RETURNING id;

-- name: GetDriver :one
SELECT * FROM drivers WHERE id = $1;

-- name: ListDrivers :many
SELECT * FROM drivers 
WHERE company_id = $1 
  AND ($2::text = '' OR status = $2)
ORDER BY driver_name ASC;

-- name: UpdateDriverStatus :exec
UPDATE drivers 
SET status = $2, updated_at = NOW() 
WHERE id = $1;

-- ═══════════════════════════════════════════════════════════════════════════
-- SHIPMENTS
-- ═══════════════════════════════════════════════════════════════════════════

-- name: CreateShipment :one
INSERT INTO shipments (
    company_id, shipment_number, shipment_type, origin_warehouse_id, 
    destination_warehouse_id, destination_address, destination_city, destination_country,
    destination_contact_name, destination_contact_phone,
    planned_dispatch_at, planned_delivery_at, created_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
) RETURNING id;

-- name: GetShipment :one
SELECT * FROM shipments WHERE id = $1;

-- name: ListShipments :many
SELECT * FROM shipments 
WHERE company_id = $1 
  AND ($2::text = '' OR status = $2)
ORDER BY created_at DESC;

-- name: UpdateShipmentStatus :exec
UPDATE shipments 
SET status = $2,
    actual_dispatch_at = CASE
        WHEN $2 IN ('DISPATCHED', 'IN_TRANSIT') THEN COALESCE(actual_dispatch_at, NOW())
        ELSE actual_dispatch_at
    END,
    actual_delivery_at = CASE
        WHEN $2 = 'DELIVERED' THEN COALESCE(actual_delivery_at, NOW())
        ELSE actual_delivery_at
    END,
    updated_at = NOW()
WHERE id = $1;

-- name: AssignShipmentTransportCarrier :exec
UPDATE shipments
SET carrier_id = $2, carrier_service_type = $3, vehicle_id = NULL, driver_id = NULL, updated_at = NOW()
WHERE id = $1;

-- name: AssignShipmentTransportFleet :exec
UPDATE shipments
SET vehicle_id = $2, driver_id = $3, carrier_id = NULL, carrier_service_type = NULL, updated_at = NOW()
WHERE id = $1;

-- name: AddShipmentLine :one
INSERT INTO shipment_lines (
    company_id, shipment_id, product_id, quantity, weight_kg, volume_cbm
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING id;

-- name: ListShipmentLines :many
SELECT * FROM shipment_lines WHERE shipment_id = $1;

-- ═══════════════════════════════════════════════════════════════════════════
-- TRIPS & STOPS
-- ═══════════════════════════════════════════════════════════════════════════

-- name: CreateTrip :one
INSERT INTO trips (
    company_id, trip_number, vehicle_id, driver_id, fleet_id,
    origin_warehouse_id, planned_start_at, planned_end_at, created_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING id;

-- name: GetTrip :one
SELECT * FROM trips WHERE id = $1;

-- name: ListTrips :many
SELECT * FROM trips 
WHERE company_id = $1 
  AND ($2::text = '' OR status = $2)
ORDER BY created_at DESC;

-- name: UpdateTripStatus :exec
UPDATE trips 
SET status = $2, updated_at = NOW() 
WHERE id = $1;

-- name: AddTripStop :one
INSERT INTO trip_stops (
    company_id, trip_id, shipment_id, stop_sequence, stop_type,
    warehouse_id, location_address, location_city,
    location_lat, location_lon, contact_name, contact_phone,
    planned_arrival_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
) RETURNING id;

-- name: ListTripStops :many
SELECT * FROM trip_stops WHERE trip_id = $1 ORDER BY stop_sequence ASC;
