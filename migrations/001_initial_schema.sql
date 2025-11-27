-- migrations/001_initial_schema.sql

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_partman";

-- Servers table
CREATE TABLE servers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    hostname VARCHAR(255) NOT NULL UNIQUE,
    ip_address INET NOT NULL,
    os VARCHAR(100),
    os_version VARCHAR(100),
    kernel_version VARCHAR(100),
    architecture VARCHAR(50),
    cpu_model VARCHAR(255),
    cpu_cores INTEGER,
    total_memory BIGINT,
    labels JSONB DEFAULT '{}',
    status VARCHAR(20) DEFAULT 'online',
    last_seen TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Metrics table (partitioned by time)
CREATE TABLE metrics (
    id BIGSERIAL,
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
    
    -- CPU metrics
    cpu_usage_percent DECIMAL(5,2),
    cpu_user_percent DECIMAL(5,2),
    cpu_system_percent DECIMAL(5,2),
    cpu_idle_percent DECIMAL(5,2),
    cpu_iowait_percent DECIMAL(5,2),
    load_avg_1 DECIMAL(10,2),
    load_avg_5 DECIMAL(10,2),
    load_avg_15 DECIMAL(10,2),
    
    -- CPU distribution per core
    cpu_per_core JSONB,
    
    -- Memory metrics
    memory_total BIGINT,
    memory_used BIGINT,
    memory_free BIGINT,
    memory_available BIGINT,
    memory_cached BIGINT,
    memory_buffers BIGINT,
    memory_usage_percent DECIMAL(5,2),
    
    -- Swap metrics
    swap_total BIGINT,
    swap_used BIGINT,
    swap_free BIGINT,
    swap_usage_percent DECIMAL(5,2),
    
    -- Disk metrics
    disk_read_bytes BIGINT,
    disk_write_bytes BIGINT,
    disk_read_ops BIGINT,
    disk_write_ops BIGINT,
    disk_io_time_ms BIGINT,
    
    -- Partition metrics
    partitions JSONB,
    
    -- Disk temperature
    disk_temps JSONB,
    
    -- Network metrics
    network_rx_bytes BIGINT,
    network_tx_bytes BIGINT,
    network_rx_packets BIGINT,
    network_tx_packets BIGINT,
    network_rx_errors BIGINT,
    network_tx_errors BIGINT,
    
    -- Network latency
    network_latency_ms DECIMAL(10,2),
    
    -- Process metrics
    top_cpu_processes JSONB,
    top_memory_processes JSONB,
    process_count INTEGER,
    
    -- Docker metrics
    docker_containers JSONB,
    
    -- RAID status
    raid_status JSONB,
    
    PRIMARY KEY (server_id, timestamp, id)
) PARTITION BY RANGE (timestamp);

-- Create initial partitions for metrics (last 7 days + next 30 days)
CREATE TABLE metrics_default PARTITION OF metrics DEFAULT;

DO $$
DECLARE
    partition_date DATE;
    partition_name TEXT;
    start_date DATE;
    end_date DATE;
BEGIN
    FOR i IN -7..30 LOOP
        partition_date := CURRENT_DATE + (i || ' days')::INTERVAL;
        partition_name := 'metrics_' || TO_CHAR(partition_date, 'YYYY_MM_DD');
        start_date := partition_date;
        end_date := partition_date + INTERVAL '1 day';
        
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS %I PARTITION OF metrics FOR VALUES FROM (%L) TO (%L)',
            partition_name, start_date, end_date
        );
        
        EXECUTE format('CREATE INDEX IF NOT EXISTS %I ON %I (server_id, timestamp DESC)', 
            partition_name || '_server_time_idx', partition_name);
        EXECUTE format('CREATE INDEX IF NOT EXISTS %I ON %I (timestamp DESC)', 
            partition_name || '_time_idx', partition_name);
    END LOOP;
END $$;

-- Cron jobs table
CREATE TABLE cron_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    schedule VARCHAR(100) NOT NULL,
    command TEXT NOT NULL,
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(server_id, name)
);

-- Cron results table
CREATE TABLE cron_results (
    id BIGSERIAL PRIMARY KEY,
    cron_job_id UUID NOT NULL REFERENCES cron_jobs(id) ON DELETE CASCADE,
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    executed_at TIMESTAMP WITH TIME ZONE NOT NULL,
    duration_ms INTEGER,
    exit_code INTEGER,
    status VARCHAR(20),
    output TEXT,
    error TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
) PARTITION BY RANGE (executed_at);

-- Create cron_results partitions
CREATE TABLE cron_results_default PARTITION OF cron_results DEFAULT;

DO $$
DECLARE
    partition_date DATE;
    partition_name TEXT;
    start_date DATE;
    end_date DATE;
BEGIN
    FOR i IN -7..30 LOOP
        partition_date := CURRENT_DATE + (i || ' days')::INTERVAL;
        partition_name := 'cron_results_' || TO_CHAR(partition_date, 'YYYY_MM_DD');
        start_date := partition_date;
        end_date := partition_date + INTERVAL '1 day';
        
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS %I PARTITION OF cron_results FOR VALUES FROM (%L) TO (%L)',
            partition_name, start_date, end_date
        );
        
        EXECUTE format('CREATE INDEX IF NOT EXISTS %I ON %I (cron_job_id, executed_at DESC)', 
            partition_name || '_job_time_idx', partition_name);
    END LOOP;
END $$;

-- Port checks table
CREATE TABLE port_checks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    port INTEGER NOT NULL,
    protocol VARCHAR(10) DEFAULT 'tcp',
    enabled BOOLEAN DEFAULT true,
    expected_response TEXT,
    timeout_ms INTEGER DEFAULT 5000,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(server_id, name)
);

-- Port check results table
CREATE TABLE port_results (
    id BIGSERIAL PRIMARY KEY,
    port_check_id UUID NOT NULL REFERENCES port_checks(id) ON DELETE CASCADE,
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    checked_at TIMESTAMP WITH TIME ZONE NOT NULL,
    status VARCHAR(20),
    response_time_ms INTEGER,
    error TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
) PARTITION BY RANGE (checked_at);

-- Create port_results partitions
CREATE TABLE port_results_default PARTITION OF port_results DEFAULT;

DO $$
DECLARE
    partition_date DATE;
    partition_name TEXT;
    start_date DATE;
    end_date DATE;
BEGIN
    FOR i IN -7..30 LOOP
        partition_date := CURRENT_DATE + (i || ' days')::INTERVAL;
        partition_name := 'port_results_' || TO_CHAR(partition_date, 'YYYY_MM_DD');
        start_date := partition_date;
        end_date := partition_date + INTERVAL '1 day';
        
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS %I PARTITION OF port_results FOR VALUES FROM (%L) TO (%L)',
            partition_name, start_date, end_date
        );
        
        EXECUTE format('CREATE INDEX IF NOT EXISTS %I ON %I (port_check_id, checked_at DESC)', 
            partition_name || '_check_time_idx', partition_name);
    END LOOP;
END $$;

-- Indexes
CREATE INDEX idx_servers_hostname ON servers(hostname);
CREATE INDEX idx_servers_status ON servers(status);
CREATE INDEX idx_servers_labels ON servers USING GIN(labels);
CREATE INDEX idx_servers_last_seen ON servers(last_seen DESC);

CREATE INDEX idx_cron_jobs_server ON cron_jobs(server_id);
CREATE INDEX idx_cron_jobs_enabled ON cron_jobs(enabled);

CREATE INDEX idx_port_checks_server ON port_checks(server_id);
CREATE INDEX idx_port_checks_enabled ON port_checks(enabled);

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Triggers for updated_at
CREATE TRIGGER update_servers_updated_at BEFORE UPDATE ON servers
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_cron_jobs_updated_at BEFORE UPDATE ON cron_jobs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_port_checks_updated_at BEFORE UPDATE ON port_checks
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Function for automatic partition creation
CREATE OR REPLACE FUNCTION create_future_partitions()
RETURNS void AS $$
DECLARE
    partition_date DATE;
    partition_name TEXT;
    start_date DATE;
    end_date DATE;
    table_name TEXT;
BEGIN
    FOR table_name IN SELECT unnest(ARRAY['metrics', 'cron_results', 'port_results']) LOOP
        FOR i IN 1..7 LOOP
            partition_date := CURRENT_DATE + (i || ' days')::INTERVAL;
            partition_name := table_name || '_' || TO_CHAR(partition_date, 'YYYY_MM_DD');
            start_date := partition_date;
            end_date := partition_date + INTERVAL '1 day';
            
            IF NOT EXISTS (
                SELECT 1 FROM pg_class WHERE relname = partition_name
            ) THEN
                EXECUTE format(
                    'CREATE TABLE %I PARTITION OF %I FOR VALUES FROM (%L) TO (%L)',
                    partition_name, table_name, start_date, end_date
                );
                
                EXECUTE format('CREATE INDEX %I ON %I (server_id, %s DESC)', 
                    partition_name || '_server_time_idx', 
                    partition_name,
                    CASE table_name 
                        WHEN 'metrics' THEN 'timestamp'
                        WHEN 'cron_results' THEN 'executed_at'
                        WHEN 'port_results' THEN 'checked_at'
                    END
                );
            END IF;
        END LOOP;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- Function to delete old metrics (retention policy)
CREATE OR REPLACE FUNCTION cleanup_old_data(retention_days INTEGER DEFAULT 90)
RETURNS void AS $$
DECLARE
    cutoff_date TIMESTAMP WITH TIME ZONE;
    partition_name TEXT;
    table_name TEXT;
BEGIN
    cutoff_date := NOW() - (retention_days || ' days')::INTERVAL;
    
    FOR table_name IN SELECT unnest(ARRAY['metrics', 'cron_results', 'port_results']) LOOP
        FOR partition_name IN 
            SELECT tablename 
            FROM pg_tables 
            WHERE schemaname = 'public' 
            AND tablename LIKE table_name || '_%'
            AND tablename != table_name || '_default'
        LOOP
            DECLARE
                partition_date DATE;
            BEGIN
                partition_date := TO_DATE(
                    SUBSTRING(partition_name FROM LENGTH(table_name) + 2), 
                    'YYYY_MM_DD'
                );
                
                IF partition_date < DATE(cutoff_date) THEN
                    EXECUTE format('DROP TABLE IF EXISTS %I', partition_name);
                    RAISE NOTICE 'Dropped partition: %', partition_name;
                END IF;
            EXCEPTION WHEN OTHERS THEN
                CONTINUE;
            END;
        END LOOP;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- Create views for easier querying
CREATE OR REPLACE VIEW server_latest_metrics AS
SELECT DISTINCT ON (m.server_id)
    s.id as server_id,
    s.hostname,
    s.ip_address,
    s.status,
    m.timestamp,
    m.cpu_usage_percent,
    m.memory_usage_percent,
    m.swap_usage_percent,
    m.load_avg_1,
    m.network_rx_bytes,
    m.network_tx_bytes
FROM servers s
LEFT JOIN metrics m ON s.id = m.server_id
ORDER BY m.server_id, m.timestamp DESC;

CREATE OR REPLACE VIEW server_health_summary AS
SELECT 
    s.id,
    s.hostname,
    s.status,
    s.last_seen,
    COUNT(DISTINCT cj.id) as cron_job_count,
    COUNT(DISTINCT pc.id) as port_check_count,
    CASE 
        WHEN s.last_seen > NOW() - INTERVAL '5 minutes' THEN 'online'
        WHEN s.last_seen > NOW() - INTERVAL '15 minutes' THEN 'warning'
        ELSE 'offline'
    END as health_status
FROM servers s
LEFT JOIN cron_jobs cj ON s.id = cj.server_id AND cj.enabled = true
LEFT JOIN port_checks pc ON s.id = pc.server_id AND pc.enabled = true
GROUP BY s.id, s.hostname, s.status, s.last_seen;