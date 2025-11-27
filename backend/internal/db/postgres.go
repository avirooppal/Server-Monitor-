package db

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"server-monitor/backend/internal/model"
)

type Database struct {
	pool *pgxpool.Pool
}

func NewDatabase(connString string) (*Database, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, err
	}
	return &Database{pool: pool}, nil
}

func (db *Database) Init(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS servers (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		hostname VARCHAR(255) UNIQUE NOT NULL,
		ip_address INET NOT NULL,
		os VARCHAR(100),
		cpu_model TEXT,
		total_ram TEXT,
		labels JSONB DEFAULT '{}',
		agent_version VARCHAR(50),
		last_seen TIMESTAMP WITH TIME ZONE,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	);
	ALTER TABLE servers ADD COLUMN IF NOT EXISTS cpu_model TEXT;
	ALTER TABLE servers ADD COLUMN IF NOT EXISTS total_ram TEXT;

	CREATE INDEX IF NOT EXISTS idx_servers_hostname ON servers(hostname);
	CREATE INDEX IF NOT EXISTS idx_servers_last_seen ON servers(last_seen);

	CREATE TABLE IF NOT EXISTS metrics (
		id BIGSERIAL,
		server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
		metric_type VARCHAR(50) NOT NULL,
		metric_name VARCHAR(100) NOT NULL,
		value DOUBLE PRECISION NOT NULL,
		labels JSONB DEFAULT '{}',
		timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
		PRIMARY KEY (id, timestamp)
	) PARTITION BY RANGE (timestamp);

	CREATE INDEX IF NOT EXISTS idx_metrics_server_time ON metrics(server_id, timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_metrics_type_time ON metrics(metric_type, timestamp DESC);

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

	CREATE TABLE IF NOT EXISTS process_snapshots (
		id BIGSERIAL PRIMARY KEY,
		server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
		processes JSONB NOT NULL,
		timestamp TIMESTAMP WITH TIME ZONE NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_process_snapshots_server_time ON process_snapshots(server_id, timestamp DESC);

	CREATE TABLE IF NOT EXISTS container_snapshots (
		id BIGSERIAL PRIMARY KEY,
		server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
		containers JSONB NOT NULL,
		timestamp TIMESTAMP WITH TIME ZONE NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_container_snapshots_server_time ON container_snapshots(server_id, timestamp DESC);
	`

	_, err := db.pool.Exec(ctx, schema)
	if err != nil {
		return err
	}

	// Create partitions for current and next 2 months
	return db.EnsurePartitions(ctx)
}

func (db *Database) EnsurePartitions(ctx context.Context) error {
	for i := -1; i < 3; i++ {
		month := time.Now().AddDate(0, i, 0)
		start := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 1, 0)

		tableName := fmt.Sprintf("metrics_%s", start.Format("2006_01"))

		query := fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s PARTITION OF metrics
			FOR VALUES FROM ('%s') TO ('%s')
		`, tableName, start.Format(time.RFC3339), end.Format(time.RFC3339))

		_, err := db.pool.Exec(ctx, query)
		if err != nil {
			log.Printf("Warning: Failed to create partition %s: %v", tableName, err)
		}
	}
	return nil
}

func (db *Database) BulkInsertMetrics(ctx context.Context, metrics []model.Metric) error {
	rows := make([][]interface{}, len(metrics))
	for i, m := range metrics {
		labelsJSON, _ := json.Marshal(m.Labels)
		rows[i] = []interface{}{
			m.ServerID, m.MetricType, m.MetricName, m.Value, labelsJSON, m.Timestamp,
		}
	}

	_, err := db.pool.CopyFrom(
		ctx,
		pgx.Identifier{"metrics"},
		[]string{"server_id", "metric_type", "metric_name", "value", "labels", "timestamp"},
		pgx.CopyFromRows(rows),
	)
	return err
}

func (db *Database) GetPool() *pgxpool.Pool {
	return db.pool
}

// Cron Jobs
func (db *Database) CreateCronJob(ctx context.Context, job *model.CronJob) error {
	return db.pool.QueryRow(ctx, `
		INSERT INTO cron_jobs (server_id, name, schedule, command)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at
	`, job.ServerID, job.Name, job.Schedule, job.Command).
		Scan(&job.ID, &job.CreatedAt, &job.UpdatedAt)
}

func (db *Database) CreateCronResult(ctx context.Context, result *model.CronResult) error {
	return db.pool.QueryRow(ctx, `
		INSERT INTO cron_results (job_id, success, duration_ms, output, timestamp)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, result.JobID, result.Success, result.DurationMS, result.Output, result.Timestamp).
		Scan(&result.ID)
}

func (db *Database) GetCronJobs(ctx context.Context, serverID string) ([]model.CronJob, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT id, server_id, name, schedule, command, created_at, updated_at
		FROM cron_jobs WHERE server_id = $1 ORDER BY name
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []model.CronJob
	for rows.Next() {
		var j model.CronJob
		if err := rows.Scan(&j.ID, &j.ServerID, &j.Name, &j.Schedule, &j.Command, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

// Port Checks
func (db *Database) CreatePortCheck(ctx context.Context, check *model.PortCheck) error {
	return db.pool.QueryRow(ctx, `
		INSERT INTO port_checks (server_id, name, host, port, protocol)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`, check.ServerID, check.Name, check.Host, check.Port, check.Protocol).
		Scan(&check.ID, &check.CreatedAt, &check.UpdatedAt)
}

func (db *Database) CreatePortResult(ctx context.Context, result *model.PortResult) error {
	return db.pool.QueryRow(ctx, `
		INSERT INTO port_results (check_id, success, latency_ms, error, timestamp)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, result.CheckID, result.Success, result.LatencyMS, result.Error, result.Timestamp).
		Scan(&result.ID)
}

func (db *Database) GetPortChecks(ctx context.Context, serverID string) ([]model.PortCheck, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT id, server_id, name, host, port, protocol, created_at, updated_at
		FROM port_checks WHERE server_id = $1 ORDER BY name
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checks []model.PortCheck
	for rows.Next() {
		var c model.PortCheck
		if err := rows.Scan(&c.ID, &c.ServerID, &c.Name, &c.Host, &c.Port, &c.Protocol, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		checks = append(checks, c)
	}
	return checks, nil
}

// Process Snapshots
func (db *Database) CreateProcessSnapshot(ctx context.Context, snapshot *model.ProcessSnapshot) error {
	processesJSON, _ := json.Marshal(snapshot.Processes)
	return db.pool.QueryRow(ctx, `
		INSERT INTO process_snapshots (server_id, processes, timestamp)
		VALUES ($1, $2, $3)
		RETURNING id
	`, snapshot.ServerID, processesJSON, snapshot.Timestamp).
		Scan(&snapshot.ID)
}

// Container Snapshots
func (db *Database) CreateContainerSnapshot(ctx context.Context, snapshot *model.ContainerSnapshot) error {
	containersJSON, _ := json.Marshal(snapshot.Containers)
	return db.pool.QueryRow(ctx, `
		INSERT INTO container_snapshots (server_id, containers, timestamp)
		VALUES ($1, $2, $3)
		RETURNING id
	`, snapshot.ServerID, containersJSON, snapshot.Timestamp).
		Scan(&snapshot.ID)
}

func (db *Database) GetLatestContainerSnapshot(ctx context.Context, serverID string) (*model.ContainerSnapshot, error) {
	var snapshot model.ContainerSnapshot
	var containersJSON []byte
	err := db.pool.QueryRow(ctx, `
		SELECT id, server_id, containers, timestamp
		FROM container_snapshots
		WHERE server_id = $1
		ORDER BY timestamp DESC
		LIMIT 1
	`, serverID).Scan(&snapshot.ID, &snapshot.ServerID, &containersJSON, &snapshot.Timestamp)

	if err != nil {
		return nil, err
	}

	json.Unmarshal(containersJSON, &snapshot.Containers)
	return &snapshot, nil
}
