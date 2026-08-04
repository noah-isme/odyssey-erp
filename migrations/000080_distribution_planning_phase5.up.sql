-- Migration 000080: Phase 5 - Distribution Planning (Load Planning, Route Optimization, Transfer Orders)
-- Creates tables for planning horizon, load consolidation, route optimization, and inter-warehouse transfers

BEGIN;

-- ═══════════════════════════════════════════════════════════════════════════
-- PLANNING CONFIGURATION
-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS planning_horizons (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    warehouse_id BIGINT NOT NULL REFERENCES warehouses(id),
    planning_start_date DATE NOT NULL,
    planning_end_date DATE NOT NULL,
    frozen_until_date DATE,
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'ARCHIVED', 'CANCELLED')),
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id),
    UNIQUE(company_id, warehouse_id, planning_start_date, planning_end_date)
);

CREATE TABLE IF NOT EXISTS planning_rules (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    warehouse_id BIGINT NOT NULL REFERENCES warehouses(id),
    rule_name TEXT NOT NULL,
    rule_type TEXT NOT NULL CHECK (rule_type IN ('CAPACITY', 'WEIGHT', 'TIME_WINDOW', 'VEHICLE_TYPE', 'CUSTOM')),
    max_load_weight_kg NUMERIC(14,4),
    max_load_volume_cbm NUMERIC(14,4),
    max_items_per_load INT,
    time_window_start TIME,
    time_window_end TIME,
    vehicle_type_required TEXT,
    custom_rule_expression TEXT,
    priority INT DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id)
);

-- ═══════════════════════════════════════════════════════════════════════════
-- LOAD PLANNING & CONSOLIDATION
-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS loads (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    load_number TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'PLANNED', 'READY', 'DISPATCHED', 'IN_TRANSIT', 'DELIVERED', 'CANCELLED')),
    origin_warehouse_id BIGINT NOT NULL REFERENCES warehouses(id),
    destination_warehouse_id BIGINT REFERENCES warehouses(id),
    destination_address TEXT,
    destination_city TEXT,
    destination_country TEXT,
    vehicle_id BIGINT REFERENCES vehicles(id),
    driver_id BIGINT REFERENCES drivers(id),
    carrier_id BIGINT REFERENCES carriers(id),
    carrier_service_type TEXT,
    -- Load metrics
    total_weight_kg NUMERIC(14,4),
    total_volume_cbm NUMERIC(14,4),
    total_items INT,
    -- Planning dates
    planned_pickup_date DATE,
    planned_delivery_date DATE,
    actual_dispatch_at TIMESTAMP WITH TIME ZONE,
    actual_delivery_at TIMESTAMP WITH TIME ZONE,
    -- Costs
    freight_charge NUMERIC(14,4),
    freight_currency TEXT DEFAULT 'USD',
    -- Notes
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id),
    UNIQUE(company_id, load_number)
);

CREATE TABLE IF NOT EXISTS load_items (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    load_id BIGINT NOT NULL REFERENCES loads(id),
    shipment_id BIGINT REFERENCES shipments(id),
    product_id BIGINT NOT NULL REFERENCES products(id),
    quantity NUMERIC(14,4) NOT NULL,
    weight_kg NUMERIC(14,4),
    volume_cbm NUMERIC(14,4),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- ═══════════════════════════════════════════════════════════════════════════
-- ROUTE OPTIMIZATION & SEQUENCING
-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS delivery_routes (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    route_number TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'OPTIMIZED', 'APPROVED', 'ACTIVE', 'COMPLETED', 'CANCELLED')),
    load_id BIGINT NOT NULL REFERENCES loads(id),
    total_distance_km NUMERIC(10,2),
    estimated_duration_minutes INT,
    optimization_score NUMERIC(5,2),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id),
    UNIQUE(company_id, route_number)
);

CREATE TABLE IF NOT EXISTS route_stops (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    route_id BIGINT NOT NULL REFERENCES delivery_routes(id),
    stop_sequence INT NOT NULL,
    stop_type TEXT NOT NULL CHECK (stop_type IN ('WAREHOUSE', 'CUSTOMER', 'DELIVERY_POINT')),
    warehouse_id BIGINT REFERENCES warehouses(id),
    customer_id BIGINT,
    customer_address TEXT,
    customer_city TEXT,
    location_lat NUMERIC(10,8),
    location_lon NUMERIC(11,8),
    contact_name TEXT,
    contact_phone TEXT,
    planned_arrival_time TIME,
    planned_departure_time TIME,
    actual_arrival_at TIMESTAMP WITH TIME ZONE,
    actual_departure_at TIMESTAMP WITH TIME ZONE,
    items_delivered INT,
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, route_id, stop_sequence)
);

-- ═══════════════════════════════════════════════════════════════════════════
-- TRANSFER ORDERS (Inter-Warehouse Transfers)
-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS transfer_orders (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    transfer_number TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'APPROVED', 'DISPATCHED', 'IN_TRANSIT', 'RECEIVED', 'CANCELLED')),
    from_warehouse_id BIGINT NOT NULL REFERENCES warehouses(id),
    to_warehouse_id BIGINT NOT NULL REFERENCES warehouses(id),
    -- Transport assignment
    load_id BIGINT REFERENCES loads(id),
    vehicle_id BIGINT REFERENCES vehicles(id),
    driver_id BIGINT REFERENCES drivers(id),
    carrier_id BIGINT REFERENCES carriers(id),
    carrier_service_type TEXT,
    -- Scheduling
    planned_dispatch_date DATE,
    planned_arrival_date DATE,
    actual_dispatch_at TIMESTAMP WITH TIME ZONE,
    actual_arrival_at TIMESTAMP WITH TIME ZONE,
    -- Tracking
    total_weight_kg NUMERIC(14,4),
    total_volume_cbm NUMERIC(14,4),
    total_items INT,
    -- In-transit inventory
    in_transit_quantity NUMERIC(14,4),
    -- Costs
    transfer_cost NUMERIC(14,4),
    transfer_cost_currency TEXT DEFAULT 'USD',
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id),
    UNIQUE(company_id, transfer_number),
    CONSTRAINT transfer_transport_assignment CHECK (
        (vehicle_id IS NOT NULL AND driver_id IS NOT NULL AND carrier_id IS NULL) OR
        (vehicle_id IS NULL AND driver_id IS NULL AND carrier_id IS NOT NULL)
    )
);

CREATE TABLE IF NOT EXISTS transfer_order_lines (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    transfer_order_id BIGINT NOT NULL REFERENCES transfer_orders(id),
    product_id BIGINT NOT NULL REFERENCES products(id),
    quantity_requested NUMERIC(14,4) NOT NULL,
    quantity_shipped NUMERIC(14,4),
    quantity_received NUMERIC(14,4),
    lot_number TEXT,
    serial_numbers TEXT[] DEFAULT ARRAY[]::TEXT[],
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- ═══════════════════════════════════════════════════════════════════════════
-- INDEXES FOR PERFORMANCE
-- ═══════════════════════════════════════════════════════════════════════════

CREATE INDEX idx_planning_horizons_company_warehouse ON planning_horizons(company_id, warehouse_id, planning_start_date);
CREATE INDEX idx_planning_rules_company_warehouse ON planning_rules(company_id, warehouse_id, is_active);
CREATE INDEX idx_loads_company_status ON loads(company_id, status);
CREATE INDEX idx_loads_origin_destination ON loads(origin_warehouse_id, destination_warehouse_id);
CREATE INDEX idx_loads_vehicle_driver ON loads(vehicle_id, driver_id);
CREATE INDEX idx_loads_carrier ON loads(carrier_id);
CREATE INDEX idx_load_items_load ON load_items(load_id);
CREATE INDEX idx_delivery_routes_company_status ON delivery_routes(company_id, status);
CREATE INDEX idx_delivery_routes_load ON delivery_routes(load_id);
CREATE INDEX idx_route_stops_route ON route_stops(route_id, stop_sequence);
CREATE INDEX idx_transfer_orders_company_status ON transfer_orders(company_id, status);
CREATE INDEX idx_transfer_orders_from_to_warehouse ON transfer_orders(from_warehouse_id, to_warehouse_id);
CREATE INDEX idx_transfer_orders_load ON transfer_orders(load_id);
CREATE INDEX idx_transfer_order_lines_transfer ON transfer_order_lines(transfer_order_id);

-- ═══════════════════════════════════════════════════════════════════════════
-- RBAC PERMISSIONS (Phase 5)
-- ═══════════════════════════════════════════════════════════════════════════

INSERT INTO permissions (name, description) VALUES
    ('distribution.planning.view', 'View planning horizons and rules'),
    ('distribution.planning.manage', 'Create and manage planning configuration'),
    ('distribution.loads.view', 'View loads and consolidations'),
    ('distribution.loads.create', 'Create new load consolidation'),
    ('distribution.loads.dispatch', 'Dispatch load to vehicle/carrier'),
    ('distribution.routes.view', 'View delivery routes and optimization'),
    ('distribution.routes.create', 'Create and plan delivery routes'),
    ('distribution.routes.optimize', 'Run route optimization algorithm'),
    ('distribution.transfers.view', 'View transfer orders'),
    ('distribution.transfers.create', 'Create inter-warehouse transfer'),
    ('distribution.transfers.dispatch', 'Dispatch transfer order'),
    ('distribution.transfers.receive', 'Receive transferred inventory')
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.name IN (
    'distribution.planning.view', 'distribution.planning.manage',
    'distribution.loads.view', 'distribution.loads.create', 'distribution.loads.dispatch',
    'distribution.routes.view', 'distribution.routes.create', 'distribution.routes.optimize',
    'distribution.transfers.view', 'distribution.transfers.create', 'distribution.transfers.dispatch', 'distribution.transfers.receive'
)
WHERE LOWER(TRIM(r.name)) IN ('admin', 'administrator')
ON CONFLICT DO NOTHING;

COMMIT;
