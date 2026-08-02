-- Migration 000079: Phase 4 - Transport Execution (Carriers, Fleet, Vehicles, Drivers, Shipments, Trips)
-- Creates tables and permissions for carrier management, fleet operations, and shipment execution

BEGIN;

-- ═══════════════════════════════════════════════════════════════════════════
-- CARRIERS & RATE CARDS
-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS carriers (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    carrier_name TEXT NOT NULL,
    carrier_code TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE', 'SUSPENDED')),
    contact_name TEXT,
    contact_email TEXT,
    contact_phone TEXT,
    insurance_provider TEXT,
    insurance_policy_number TEXT,
    insurance_expires_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id),
    updated_by BIGINT NOT NULL REFERENCES users(id),
    UNIQUE(company_id, carrier_code)
);

CREATE TABLE IF NOT EXISTS carrier_rate_cards (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    carrier_id BIGINT NOT NULL REFERENCES carriers(id),
    route_from_city TEXT NOT NULL,
    route_to_city TEXT NOT NULL,
    weight_from NUMERIC(12,4) NOT NULL, -- kg, exact accounting
    weight_to NUMERIC(12,4) NOT NULL,
    volume_from NUMERIC(12,4) NOT NULL, -- cubic meters
    volume_to NUMERIC(12,4) NOT NULL,
    rate_per_unit NUMERIC(14,4) NOT NULL, -- exact accounting
    rate_unit TEXT NOT NULL CHECK (rate_unit IN ('KG', 'CBM', 'SHIPMENT')),
    currency TEXT NOT NULL DEFAULT 'USD',
    effective_from DATE NOT NULL,
    effective_to DATE,
    minimum_charge NUMERIC(14,4),
    fuel_surcharge_pct NUMERIC(6,2),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, carrier_id, route_from_city, route_to_city, weight_from, weight_to, effective_from)
);

-- ═══════════════════════════════════════════════════════════════════════════
-- FLEET MANAGEMENT
-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS fleets (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    fleet_name TEXT NOT NULL,
    fleet_code TEXT NOT NULL,
    fleet_type TEXT NOT NULL CHECK (fleet_type IN ('OWN', 'CONTRACTED', 'MIXED')),
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE', 'RETIRED')),
    warehouse_id BIGINT REFERENCES warehouses(id),
    home_city TEXT,
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id),
    UNIQUE(company_id, fleet_code)
);

CREATE TABLE IF NOT EXISTS vehicles (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    fleet_id BIGINT NOT NULL REFERENCES fleets(id),
    vehicle_registration TEXT NOT NULL,
    vehicle_type TEXT NOT NULL CHECK (vehicle_type IN ('VAN', 'TRUCK', 'LORRY', 'BIKE', 'CAR')),
    status TEXT NOT NULL DEFAULT 'AVAILABLE' CHECK (status IN ('AVAILABLE', 'IN_USE', 'MAINTENANCE', 'RETIRED')),
    max_weight_kg NUMERIC(10,2),
    max_volume_cbm NUMERIC(10,4),
    license_plate TEXT NOT NULL UNIQUE,
    vin TEXT UNIQUE,
    make TEXT,
    model TEXT,
    year_manufactured INT,
    last_maintenance_at TIMESTAMP WITH TIME ZONE,
    next_maintenance_due DATE,
    insurance_expires_at TIMESTAMP WITH TIME ZONE,
    gps_device_id TEXT,
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id),
    UNIQUE(company_id, vehicle_registration)
);

CREATE TABLE IF NOT EXISTS drivers (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    driver_name TEXT NOT NULL,
    driver_code TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE', 'ON_LEAVE', 'TERMINATED')),
    email TEXT,
    phone TEXT,
    license_number TEXT NOT NULL,
    license_class TEXT CHECK (license_class IN ('A', 'B', 'C', 'D', 'E')),
    license_expires_at TIMESTAMP WITH TIME ZONE,
    emergency_contact_name TEXT,
    emergency_contact_phone TEXT,
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id),
    UNIQUE(company_id, driver_code),
    UNIQUE(license_number)
);

-- ═══════════════════════════════════════════════════════════════════════════
-- SHIPMENTS & TRIPS
-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS shipments (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    shipment_number TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'CONFIRMED', 'DISPATCHED', 'IN_TRANSIT', 'DELIVERED', 'CANCELLED')),
    shipment_type TEXT NOT NULL CHECK (shipment_type IN ('DELIVERY', 'RETURN', 'TRANSFER')),
    origin_warehouse_id BIGINT REFERENCES warehouses(id),
    destination_warehouse_id BIGINT REFERENCES warehouses(id),
    destination_address TEXT,
    destination_city TEXT,
    destination_country TEXT,
    destination_contact_name TEXT,
    destination_contact_phone TEXT,
    -- Transport assignment: either vehicle+driver OR carrier+service
    vehicle_id BIGINT REFERENCES vehicles(id),
    driver_id BIGINT REFERENCES drivers(id),
    carrier_id BIGINT REFERENCES carriers(id),
    carrier_service_type TEXT CHECK (carrier_service_type IN ('STANDARD', 'EXPRESS', 'OVERNIGHT', 'ECONOMY')),
    -- Scheduling
    planned_dispatch_at TIMESTAMP WITH TIME ZONE,
    planned_delivery_at TIMESTAMP WITH TIME ZONE,
    actual_dispatch_at TIMESTAMP WITH TIME ZONE,
    actual_delivery_at TIMESTAMP WITH TIME ZONE,
    -- Tracking
    total_weight_kg NUMERIC(14,4),
    total_volume_cbm NUMERIC(14,4),
    freight_charge NUMERIC(14,4),
    freight_currency TEXT DEFAULT 'USD',
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id),
    UNIQUE(company_id, shipment_number),
    -- Constraint: must have vehicle+driver OR carrier+service, not both
    CONSTRAINT shipment_transport_assignment CHECK (
        (vehicle_id IS NOT NULL AND driver_id IS NOT NULL AND carrier_id IS NULL) OR
        (vehicle_id IS NULL AND driver_id IS NULL AND carrier_id IS NOT NULL)
    )
);

CREATE TABLE IF NOT EXISTS shipment_lines (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    shipment_id BIGINT NOT NULL REFERENCES shipments(id),
    product_id BIGINT NOT NULL REFERENCES products(id),
    quantity NUMERIC(14,4) NOT NULL,
    weight_kg NUMERIC(14,4),
    volume_cbm NUMERIC(14,4),
    lot_number TEXT,
    serial_numbers TEXT[] DEFAULT ARRAY[]::TEXT[],
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS trips (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    trip_number TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'PLANNED' CHECK (status IN ('PLANNED', 'DISPATCHED', 'IN_PROGRESS', 'COMPLETED', 'CANCELLED')),
    vehicle_id BIGINT NOT NULL REFERENCES vehicles(id),
    driver_id BIGINT NOT NULL REFERENCES drivers(id),
    fleet_id BIGINT REFERENCES fleets(id),
    origin_warehouse_id BIGINT REFERENCES warehouses(id),
    planned_start_at TIMESTAMP WITH TIME ZONE,
    planned_end_at TIMESTAMP WITH TIME ZONE,
    actual_start_at TIMESTAMP WITH TIME ZONE,
    actual_end_at TIMESTAMP WITH TIME ZONE,
    total_distance_km NUMERIC(10,2),
    fuel_used_liters NUMERIC(10,4),
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id),
    UNIQUE(company_id, trip_number)
);

CREATE TABLE IF NOT EXISTS trip_stops (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    trip_id BIGINT NOT NULL REFERENCES trips(id),
    shipment_id BIGINT REFERENCES shipments(id),
    stop_sequence INT NOT NULL,
    stop_type TEXT NOT NULL CHECK (stop_type IN ('PICKUP', 'DELIVERY', 'TRANSFER')),
    warehouse_id BIGINT REFERENCES warehouses(id),
    location_address TEXT,
    location_city TEXT,
    location_lat NUMERIC(10,8),
    location_lon NUMERIC(11,8),
    contact_name TEXT,
    contact_phone TEXT,
    planned_arrival_at TIMESTAMP WITH TIME ZONE,
    actual_arrival_at TIMESTAMP WITH TIME ZONE,
    actual_departure_at TIMESTAMP WITH TIME ZONE,
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, trip_id, stop_sequence)
);

-- ═══════════════════════════════════════════════════════════════════════════
-- INDEXES FOR PERFORMANCE
-- ═══════════════════════════════════════════════════════════════════════════

CREATE INDEX idx_carriers_company_status ON carriers(company_id, status);
CREATE INDEX idx_carrier_rate_cards_company_carrier ON carrier_rate_cards(company_id, carrier_id, effective_from);
CREATE INDEX idx_fleets_company_status ON fleets(company_id, status);
CREATE INDEX idx_vehicles_company_fleet_status ON vehicles(company_id, fleet_id, status);
CREATE INDEX idx_drivers_company_status ON drivers(company_id, status);
CREATE INDEX idx_shipments_company_status ON shipments(company_id, status);
CREATE INDEX idx_shipments_vehicle_driver ON shipments(vehicle_id, driver_id);
CREATE INDEX idx_shipments_carrier ON shipments(carrier_id);
CREATE INDEX idx_shipments_dispatch_delivery ON shipments(planned_dispatch_at, planned_delivery_at);
CREATE INDEX idx_shipment_lines_shipment ON shipment_lines(shipment_id);
CREATE INDEX idx_trips_company_vehicle_status ON trips(company_id, vehicle_id, status);
CREATE INDEX idx_trips_driver_status ON trips(driver_id, status);
CREATE INDEX idx_trip_stops_trip ON trip_stops(trip_id, stop_sequence);
CREATE INDEX idx_trip_stops_shipment ON trip_stops(shipment_id);

-- ═══════════════════════════════════════════════════════════════════════════
-- AUDIT LOGGING
-- ═══════════════════════════════════════════════════════════════════════════

-- Audit trail entries created by application code

-- ═══════════════════════════════════════════════════════════════════════════
-- RBAC PERMISSIONS (Phase 4)
-- ═══════════════════════════════════════════════════════════════════════════

INSERT INTO rbac_permissions (permission_code, permission_name, description, module, created_at)
VALUES
    ('logistics.carriers.view', 'View Carriers', 'View carrier list and details', 'logistics', NOW()),
    ('logistics.carriers.create', 'Create Carrier', 'Create new carrier', 'logistics', NOW()),
    ('logistics.carriers.edit', 'Edit Carrier', 'Edit carrier details', 'logistics', NOW()),
    ('logistics.carriers.delete', 'Delete Carrier', 'Delete carrier', 'logistics', NOW()),
    ('logistics.fleet.view', 'View Fleet', 'View fleet and vehicle details', 'logistics', NOW()),
    ('logistics.fleet.create', 'Create Fleet/Vehicle', 'Create new fleet or vehicle', 'logistics', NOW()),
    ('logistics.fleet.edit', 'Edit Fleet/Vehicle', 'Edit fleet or vehicle', 'logistics', NOW()),
    ('logistics.drivers.view', 'View Drivers', 'View driver list and details', 'logistics', NOW()),
    ('logistics.drivers.create', 'Create Driver', 'Create new driver', 'logistics', NOW()),
    ('logistics.drivers.edit', 'Edit Driver', 'Edit driver details', 'logistics', NOW()),
    ('logistics.shipments.view', 'View Shipments', 'View shipment list and tracking', 'logistics', NOW()),
    ('logistics.shipments.create', 'Create Shipment', 'Create new shipment', 'logistics', NOW()),
    ('logistics.shipments.dispatch', 'Dispatch Shipment', 'Dispatch shipment to carrier/vehicle', 'logistics', NOW()),
    ('logistics.shipments.track', 'Track Shipment', 'Real-time tracking and updates', 'logistics', NOW()),
    ('logistics.trips.view', 'View Trips', 'View trip list and planning', 'logistics', NOW()),
    ('logistics.trips.create', 'Create Trip', 'Create new trip', 'logistics', NOW()),
    ('logistics.trips.dispatch', 'Dispatch Trip', 'Dispatch trip to driver', 'logistics', NOW()),
    ('logistics.trips.complete', 'Complete Trip', 'Mark trip as completed', 'logistics', NOW());

COMMIT;
