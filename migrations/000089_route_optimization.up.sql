CREATE TABLE logistics_route_optimization_jobs (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    trip_id BIGINT NOT NULL,
    status VARCHAR(50) NOT NULL,
    engine VARCHAR(50) NOT NULL,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE logistics_route_sequences (
    id BIGSERIAL PRIMARY KEY,
    optimization_job_id BIGINT NOT NULL REFERENCES logistics_route_optimization_jobs(id) ON DELETE CASCADE,
    trip_stop_id BIGINT NOT NULL,
    optimized_sequence INT NOT NULL,
    estimated_arrival_at TIMESTAMP WITH TIME ZONE,
    estimated_distance_km NUMERIC(10, 2)
);
