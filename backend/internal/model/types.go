package model

import (
	"time"

	"github.com/google/uuid"
)

type Server struct {
	ID           uuid.UUID              `json:"id"`
	Hostname     string                 `json:"hostname"`
	IPAddress    string                 `json:"ip_address"`
	OS            string                 `json:"os"`
	CPUModel      string                 `json:"cpu_model"`
	TotalRAM      string                 `json:"total_ram"`
	KernelVersion string                 `json:"kernel_version"`
	Uptime        string                 `json:"uptime"`
	Labels        map[string]interface{} `json:"labels"`
	AgentVersion  string                 `json:"agent_version"`
	LastSeen      time.Time              `json:"last_seen"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

type Metric struct {
	ID         int64                  `json:"id"`
	ServerID   uuid.UUID              `json:"server_id"`
	MetricType string                 `json:"metric_type"`
	MetricName string                 `json:"metric_name"`
	Value      float64                `json:"value"`
	Labels     map[string]interface{} `json:"labels"`
	Timestamp  time.Time              `json:"timestamp"`
}

type IngestRequest struct {
	ServerID uuid.UUID `json:"server_id"`
	Metrics  []Metric  `json:"metrics"`
}

type IngestResponse struct {
	Success    bool  `json:"success"`
	Inserted   int   `json:"inserted"`
	Failed     int   `json:"failed"`
	DurationMS int64 `json:"duration_ms"`
}

type CronJob struct {
	ID        uuid.UUID `json:"id"`
	ServerID  uuid.UUID `json:"server_id"`
	Name      string    `json:"name"`
	Schedule  string    `json:"schedule"`
	Command   string    `json:"command"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CronResult struct {
	ID         int64     `json:"id"`
	JobID      uuid.UUID `json:"job_id"`
	Success    bool      `json:"success"`
	DurationMS int64     `json:"duration_ms"`
	Output     string    `json:"output"`
	Timestamp  time.Time `json:"timestamp"`
}

type PortCheck struct {
	ID        uuid.UUID `json:"id"`
	ServerID  uuid.UUID `json:"server_id"`
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Protocol  string    `json:"protocol"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PortResult struct {
	ID        int64     `json:"id"`
	CheckID   uuid.UUID `json:"check_id"`
	Success   bool      `json:"success"`
	LatencyMS int64     `json:"latency_ms"`
	Error     string    `json:"error"`
	Timestamp time.Time `json:"timestamp"`
}
type Process struct {
	PID           int32   `json:"pid"`
	Name          string  `json:"name"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryPercent float32 `json:"memory_percent"`
	Username      string  `json:"username"`
	Status        string  `json:"status"`
	Threads       int32   `json:"threads"`
	IOReadBytes   uint64  `json:"io_read_bytes"`
	IOWriteBytes  uint64  `json:"io_write_bytes"`
}

type ProcessSnapshot struct {
	ID         int64     `json:"id"`
	ServerID   uuid.UUID `json:"server_id"`
	Processes  []Process `json:"processes"`
	Timestamp  time.Time `json:"timestamp"`
}

type Container struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	State   string `json:"state"`
	Status  string `json:"status"`
	Created int64  `json:"created"`
	Ports          string `json:"ports"`
	NetRxBytes     uint64  `json:"net_rx_bytes"`
	NetTxBytes     uint64  `json:"net_tx_bytes"`
	DiskReadBytes  uint64  `json:"disk_read_bytes"`
	DiskWriteBytes uint64  `json:"disk_write_bytes"`
	CPUPercent     float64 `json:"cpu_percent"`
	MemoryUsage    uint64  `json:"memory_usage"`
}

type ContainerSnapshot struct {
	ID         int64       `json:"id"`
	ServerID   uuid.UUID   `json:"server_id"`
	Containers []Container `json:"containers"`
	Timestamp  time.Time   `json:"timestamp"`
}
