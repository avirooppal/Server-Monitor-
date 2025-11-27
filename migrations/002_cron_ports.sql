CREATE TABLE IF NOT EXISTS cron_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    schedule VARCHAR(100) NOT NULL,
    command TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS cron_results (
    id BIGSERIAL PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES cron_jobs(id) ON DELETE CASCADE,
    success BOOLEAN NOT NULL,
    duration_ms BIGINT NOT NULL,
    output TEXT,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_cron_results_job_time ON cron_results(job_id, timestamp DESC);

CREATE TABLE IF NOT EXISTS port_checks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    host VARCHAR(255) NOT NULL,
    port INTEGER NOT NULL,
    protocol VARCHAR(10) DEFAULT 'tcp',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS port_results (
    id BIGSERIAL PRIMARY KEY,
    check_id UUID NOT NULL REFERENCES port_checks(id) ON DELETE CASCADE,
    success BOOLEAN NOT NULL,
    latency_ms BIGINT NOT NULL,
    error TEXT,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_port_results_check_time ON port_results(check_id, timestamp DESC);
