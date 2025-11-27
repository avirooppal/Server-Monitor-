package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"server-monitor/backend/internal/db"
	"server-monitor/backend/internal/model"
	ws "server-monitor/backend/internal/websocket"
)

// API groups the database and websocket hub used by handlers.
type API struct {
	DB  *db.Database
	Hub *ws.WSHub
}

// NewAPI creates a new API instance.
func NewAPI(database *db.Database, hub *ws.WSHub) *API {
	return &API{DB: database, Hub: hub}
}

// RegisterServer handles agent registration.
func (api *API) RegisterServer(c *gin.Context) {
	var reg struct {
		Hostname      string                 `json:"hostname" binding:"required"`
		IPAddress     string                 `json:"ip_address" binding:"required"`
		OS            string                 `json:"os"`
		CPUModel      string                 `json:"cpu_model"`
		TotalRAM      string                 `json:"total_ram"`
		KernelVersion string                 `json:"kernel_version"`
		Uptime        string                 `json:"uptime"`
		Labels        map[string]interface{} `json:"labels"`
		AgentVersion  string                 `json:"agent_version"`
	}
	if err := c.ShouldBindJSON(&reg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var server model.Server
	labelsJSON, _ := json.Marshal(reg.Labels)
	
	// Use UPSERT to handle re-registration of existing servers
	err := api.DB.GetPool().QueryRow(c.Request.Context(), `
		INSERT INTO servers (hostname, ip_address, os, cpu_model, total_ram, kernel_version, uptime, labels, agent_version, last_seen, updated_at)
		VALUES ($1, $2::INET, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
		ON CONFLICT (hostname) DO UPDATE SET
			ip_address = EXCLUDED.ip_address,
			os = EXCLUDED.os,
			cpu_model = EXCLUDED.cpu_model,
			total_ram = EXCLUDED.total_ram,
			kernel_version = EXCLUDED.kernel_version,
			uptime = EXCLUDED.uptime,
			labels = EXCLUDED.labels,
			agent_version = EXCLUDED.agent_version,
			last_seen = NOW(),
			updated_at = NOW()
		RETURNING id, created_at, updated_at`,
		reg.Hostname, reg.IPAddress, reg.OS, reg.CPUModel, reg.TotalRAM, reg.KernelVersion, reg.Uptime, labelsJSON, reg.AgentVersion).
		Scan(&server.ID, &server.CreatedAt, &server.UpdatedAt)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	// Fill in other fields for response
	server.Hostname = reg.Hostname
	server.IPAddress = reg.IPAddress
	server.OS = reg.OS
	server.Labels = reg.Labels
	server.AgentVersion = reg.AgentVersion
	
	c.JSON(http.StatusCreated, server)
}

// ListServers returns all registered servers.
func (api *API) ListServers(c *gin.Context) {
	rows, err := api.DB.GetPool().Query(c.Request.Context(), `
		SELECT id, hostname, ip_address::TEXT, os, cpu_model, total_ram, kernel_version, uptime, labels, agent_version, last_seen, created_at, updated_at
		FROM servers
		ORDER BY hostname`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	servers := []model.Server{}
	for rows.Next() {
		var s model.Server
		var labelsJSON []byte
		if err := rows.Scan(&s.ID, &s.Hostname, &s.IPAddress, &s.OS, &s.CPUModel, &s.TotalRAM, &s.KernelVersion, &s.Uptime, &labelsJSON, &s.AgentVersion, &s.LastSeen, &s.CreatedAt, &s.UpdatedAt); err != nil {
			continue
		}
		json.Unmarshal(labelsJSON, &s.Labels)
		servers = append(servers, s)
	}
	c.JSON(http.StatusOK, servers)
}

// IngestMetrics receives metric batches from the agent.
func (api *API) IngestMetrics(c *gin.Context) {
	start := time.Now()
	var req model.IngestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update server's last_seen timestamp.
	if _, err := api.DB.GetPool().Exec(c.Request.Context(), "UPDATE servers SET last_seen = NOW() WHERE id = $1", req.ServerID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server not found"})
		return
	}

	// Bulk insert metrics.
	if err := api.DB.BulkInsertMetrics(c.Request.Context(), req.Metrics); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Broadcast to any WebSocket listeners.
	for _, metric := range req.Metrics {
		metric.ServerID = req.ServerID
		select {
		case api.Hub.Broadcast <- &ws.WSMessage{Type: "metric", Data: metric, ServerID: req.ServerID}:
		default:
		}
	}

	duration := time.Since(start).Milliseconds()
	c.JSON(http.StatusOK, model.IngestResponse{Success: true, Inserted: len(req.Metrics), Failed: 0, DurationMS: duration})
}

// GetMetrics returns metric data based on query parameters.
func (api *API) GetMetrics(c *gin.Context) {
	serverID := c.Query("server_id")
	metricType := c.Query("metric_type")
	from := c.Query("from")
	to := c.Query("to")

	query := `
		SELECT id, server_id, metric_type, metric_name, value, labels, timestamp
		FROM metrics WHERE 1=1`
	args := []interface{}{}
	argPos := 1
	if serverID != "" {
		query += fmt.Sprintf(" AND server_id = $%d", argPos)
		args = append(args, serverID)
		argPos++
	}
	if metricType != "" {
		query += fmt.Sprintf(" AND metric_type = $%d", argPos)
		args = append(args, metricType)
		argPos++
	}
	if metricName := c.Query("metric_name"); metricName != "" {
		query += fmt.Sprintf(" AND metric_name = $%d", argPos)
		args = append(args, metricName)
		argPos++
	}
	if from != "" {
		query += fmt.Sprintf(" AND timestamp >= $%d", argPos)
		args = append(args, from)
		argPos++
	}
	if to != "" {
		query += fmt.Sprintf(" AND timestamp <= $%d", argPos)
		args = append(args, to)
		argPos++
	}
	query += " ORDER BY timestamp DESC LIMIT 100"

	rows, err := api.DB.GetPool().Query(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	metrics := []model.Metric{}
	for rows.Next() {
		var m model.Metric
		var labelsJSON []byte
		if err := rows.Scan(&m.ID, &m.ServerID, &m.MetricType, &m.MetricName, &m.Value, &labelsJSON, &m.Timestamp); err != nil {
			continue
		}
		json.Unmarshal(labelsJSON, &m.Labels)
		metrics = append(metrics, m)
	}
	c.JSON(http.StatusOK, gin.H{"metrics": metrics, "count": len(metrics)})
}

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

// HandleWebSocket upgrades HTTP to a WebSocket for real‑time metric streaming.
func (api *API) HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	var serverID *uuid.UUID
	if sid := c.Query("server_id"); sid != "" {
		if id, err := uuid.Parse(sid); err == nil {
			serverID = &id
		}
	}
	filters := make(map[string]string)
	if mt := c.Query("metric_type"); mt != "" {
		filters["metric_type"] = mt
	}
	client := &ws.WSClient{Conn: conn, Send: make(chan *ws.WSMessage, 256), ServerID: serverID, Filters: filters}
	api.Hub.Register <- client
	go client.WritePump()
	go client.ReadPump(api.Hub)
}

// Cron Handlers -----------------------------------------------------------
func (api *API) CreateCronJob(c *gin.Context) {
	var job model.CronJob
	if err := c.ShouldBindJSON(&job); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := api.DB.CreateCronJob(c.Request.Context(), &job); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, job)
}

func (api *API) IngestCronResult(c *gin.Context) {
	var result model.CronResult
	if err := c.ShouldBindJSON(&result); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := api.DB.CreateCronResult(c.Request.Context(), &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (api *API) ListCronJobs(c *gin.Context) {
	serverID := c.Query("server_id")
	if serverID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "server_id is required"})
		return
	}
	jobs, err := api.DB.GetCronJobs(c.Request.Context(), serverID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, jobs)
}

// Port Handlers -----------------------------------------------------------
func (api *API) CreatePortCheck(c *gin.Context) {
	var check model.PortCheck
	if err := c.ShouldBindJSON(&check); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := api.DB.CreatePortCheck(c.Request.Context(), &check); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, check)
}

func (api *API) IngestPortResult(c *gin.Context) {
	var result model.PortResult
	if err := c.ShouldBindJSON(&result); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := api.DB.CreatePortResult(c.Request.Context(), &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (api *API) ListPortChecks(c *gin.Context) {
	serverID := c.Query("server_id")
	if serverID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "server_id is required"})
		return
	}
	checks, err := api.DB.GetPortChecks(c.Request.Context(), serverID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, checks)
}

// Process Handlers
func (api *API) IngestProcessSnapshot(c *gin.Context) {
	var snapshot model.ProcessSnapshot
	if err := c.ShouldBindJSON(&snapshot); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("Received process snapshot with %d processes for server %s", len(snapshot.Processes), snapshot.ServerID)

	if err := api.DB.CreateProcessSnapshot(c.Request.Context(), &snapshot); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Broadcast to WebSocket listeners
	msg := &ws.WSMessage{Type: "process_snapshot", Data: snapshot, ServerID: snapshot.ServerID}
	select {
	case api.Hub.Broadcast <- msg:
		log.Printf("Broadcasted process snapshot to WebSocket hub")
	default:
		log.Printf("WARNING: WebSocket broadcast channel full, process snapshot not sent")
	}

	c.JSON(http.StatusCreated, snapshot)
}

func (api *API) IngestContainerSnapshot(c *gin.Context) {
	var snapshot model.ContainerSnapshot
	if err := c.ShouldBindJSON(&snapshot); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := api.DB.CreateContainerSnapshot(c.Request.Context(), &snapshot); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Broadcast to WebSocket listeners
	msg := &ws.WSMessage{Type: "container_snapshot", Data: snapshot, ServerID: snapshot.ServerID}
	select {
	case api.Hub.Broadcast <- msg:
	default:
	}

	c.JSON(http.StatusCreated, snapshot)
}

func (api *API) GetContainerSnapshot(c *gin.Context) {
	serverID := c.Query("server_id")
	if serverID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "server_id is required"})
		return
	}

	snapshot, err := api.DB.GetLatestContainerSnapshot(c.Request.Context(), serverID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, snapshot)
}
