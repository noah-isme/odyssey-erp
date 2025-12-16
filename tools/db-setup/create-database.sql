-- Odyssey ERP Database Setup
-- Run this with: sudo -u postgres psql -f create-database.sql

-- Set password for postgres user
ALTER USER postgres WITH PASSWORD 'postgres';

-- Create odyssey user
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_user WHERE usename = 'odyssey') THEN
        CREATE USER odyssey WITH PASSWORD 'odyssey';
        RAISE NOTICE 'User odyssey created';
    ELSE
        RAISE NOTICE 'User odyssey already exists';
    END IF;
END
$$;

-- Create database
SELECT 'CREATE DATABASE odyssey OWNER odyssey'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'odyssey')
\gexec

-- Grant privileges
GRANT ALL PRIVILEGES ON DATABASE odyssey TO odyssey;

-- Show results
\echo ''
\echo '=== Created User ==='
\du odyssey

\echo ''
\echo '=== Created Database ==='
\l odyssey

\echo ''
\echo 'Database setup complete!'
\echo 'Test connection: PGPASSWORD=odyssey psql -h localhost -U odyssey -d odyssey'
