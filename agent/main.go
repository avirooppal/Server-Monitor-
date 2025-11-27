package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	netstat "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

type Agent struct {
	ID           uuid.UUID
	Hostname     string
	APIUrl       string
	Client       *http.Client
	DockerClient *client.Client
	ServerID     uuid.UUID
	AgentVersion string
}

type ServerRegistration struct {
	Hostname     string                 `json:"hostname"`
	IPAddress    string                 `json:"ip_address"`
	OS           string                 `json:"os"`
	Labels       map[string]interface{} `json:"labels"`
	AgentVersion string                 `json:"agent_version"`
}

type Metric struct {
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
	ServerID  uuid.UUID `json:"server_id"`
	Processes []Process `json:"processes"`
	Timestamp time.Time `json:"timestamp"`
}

type Container struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	State   string `json:"state"`
	Status  string `json:"status"`
	Created int64  `json:"created"`
	Ports          string `json:"ports"`
	NetRxBytes     uint64 `json:"net_rx_bytes"`
	NetTxBytes     uint64 `json:"net_tx_bytes"`
	DiskReadBytes  uint64 `json:"disk_read_bytes"`
	DiskWriteBytes uint64 `json:"disk_write_bytes"`
	CPUPercent     float64 `json:"cpu_percent"`
	MemoryUsage    uint64  `json:"memory_usage"`
}

type ContainerSnapshot struct {
	ServerID   uuid.UUID   `json:"server_id"`
	Containers []Container `json:"containers"`
	Timestamp  time.Time   `json:"timestamp"`
}

type PortCheck struct {
	ID       uuid.UUID `json:"id"`
	ServerID uuid.UUID `json:"server_id"`
	Name     string    `json:"name"`
	Host     string    `json:"host"`
	Port     int       `json:"port"`
	Protocol string    `json:"protocol"`
}

type PortResult struct {
	CheckID   uuid.UUID `json:"check_id"`
	Success   bool      `json:"success"`
	LatencyMS int64     `json:"latency_ms"`
	Error     string    `json:"error"`
	Timestamp time.Time `json:"timestamp"`
}

func NewAgent(apiUrl string) *Agent {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Printf("Warning: Failed to create Docker client: %v", err)
		cli = nil
	}

	return &Agent{
		ID:           uuid.New(),
		APIUrl:       apiUrl,
		Client:       &http.Client{Timeout: 10 * time.Second},
		DockerClient: cli,
		AgentVersion: "1.0.0",
	}
}

func (a *Agent) Register() error {
	hostname, _ := os.Hostname()
	
	hostInfo, _ := host.Info()
	v, _ := mem.VirtualMemory()
	c, _ := cpu.Info()
	cpuModel := ""
	if len(c) > 0 {
		cpuModel = c[0].ModelName
	}
	totalRAM := fmt.Sprintf("%.1f GB", float64(v.Total)/1024/1024/1024)
	ip := "127.0.0.1" // simplified for now

	// Get uptime
	uptime := hostInfo.Uptime
	uptimeStr := fmt.Sprintf("%dd %dh %dm", uptime/86400, (uptime%86400)/3600, (uptime%3600)/60)

	req := map[string]interface{}{
		"hostname":       hostname,
		"ip_address":     ip,
		"os":             fmt.Sprintf("%s %s", hostInfo.OS, hostInfo.PlatformVersion),
		"cpu_model":      cpuModel,
		"total_ram":      totalRAM,
		"kernel_version": hostInfo.KernelVersion,
		"uptime":         uptimeStr,
		"labels":         map[string]string{"env": "prod"},
		"agent_version":  "1.0.0",
	}

	// Debug log
	log.Printf("Registration payload: %+v", req)

	jsonBody, _ := json.Marshal(req)
	resp, err := a.Client.Post(a.APIUrl+"/api/v1/servers", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("registration failed: %d", resp.StatusCode)
	}

	var result struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	a.ServerID = result.ID
	log.Printf("Registered with Server ID: %s", a.ServerID)
	return nil
}

func (a *Agent) CollectMetrics() ([]Metric, error) {
	var metrics []Metric
	now := time.Now()

	// CPU Total
	cpuPercent, _ := cpu.Percent(0, false)
	if len(cpuPercent) > 0 {
		metrics = append(metrics, Metric{
			ServerID:   a.ServerID,
			MetricType: "cpu_usage",
			MetricName: "cpu_total",
			Value:      cpuPercent[0],
			Timestamp:  now,
		})
	}

	// CPU Times (Distribution)
	cpuTimes, _ := cpu.Times(false)
	if len(cpuTimes) > 0 {
		ct := cpuTimes[0]
		metrics = append(metrics, Metric{
			ServerID:   a.ServerID,
			MetricType: "cpu_times",
			MetricName: "user",
			Value:      ct.User,
			Timestamp:  now,
		})
		metrics = append(metrics, Metric{
			ServerID:   a.ServerID,
			MetricType: "cpu_times",
			MetricName: "system",
			Value:      ct.System,
			Timestamp:  now,
		})
		metrics = append(metrics, Metric{
			ServerID:   a.ServerID,
			MetricType: "cpu_times",
			MetricName: "idle",
			Value:      ct.Idle,
			Timestamp:  now,
		})
		metrics = append(metrics, Metric{
			ServerID:   a.ServerID,
			MetricType: "cpu_times",
			MetricName: "nice",
			Value:      ct.Nice,
			Timestamp:  now,
		})
		metrics = append(metrics, Metric{
			ServerID:   a.ServerID,
			MetricType: "cpu_times",
			MetricName: "iowait",
			Value:      ct.Iowait,
			Timestamp:  now,
		})
		metrics = append(metrics, Metric{
			ServerID:   a.ServerID,
			MetricType: "cpu_times",
			MetricName: "irq",
			Value:      ct.Irq,
			Timestamp:  now,
		})
	}

	// vCPU Usage
	perCpuPercent, _ := cpu.Percent(0, true)
	for i, p := range perCpuPercent {
		metrics = append(metrics, Metric{
			ServerID:   a.ServerID,
			MetricType: "vcpu_usage",
			MetricName: fmt.Sprintf("vcpu_%d", i),
			Value:      p,
			Timestamp:  now,
		})
	}

	// Load Avg
	l, _ := load.Avg()
	metrics = append(metrics, Metric{
		ServerID:   a.ServerID,
		MetricType: "load_average",
		MetricName: "load1",
		Value:      l.Load1,
		Timestamp:  now,
	})
	metrics = append(metrics, Metric{
		ServerID:   a.ServerID,
		MetricType: "load_average",
		MetricName: "load5",
		Value:      l.Load5,
		Timestamp:  now,
	})
	metrics = append(metrics, Metric{
		ServerID:   a.ServerID,
		MetricType: "load_average",
		MetricName: "load15",
		Value:      l.Load15,
		Timestamp:  now,
	})

	// Memory
	v, _ := mem.VirtualMemory()
	metrics = append(metrics, Metric{
		ServerID:   a.ServerID,
		MetricType: "memory_usage",
		MetricName: "memory_used_percent",
		Value:      v.UsedPercent,
		Timestamp:  now,
	})
	metrics = append(metrics, Metric{
		ServerID:   a.ServerID,
		MetricType: "memory_usage",
		MetricName: "memory_total",
		Value:      float64(v.Total),
		Timestamp:  now,
	})
	metrics = append(metrics, Metric{
		ServerID:   a.ServerID,
		MetricType: "memory_usage",
		MetricName: "memory_used",
		Value:      float64(v.Used),
		Timestamp:  now,
	})
	metrics = append(metrics, Metric{
		ServerID:   a.ServerID,
		MetricType: "memory_usage",
		MetricName: "memory_free",
		Value:      float64(v.Free),
		Timestamp:  now,
	})
	metrics = append(metrics, Metric{
		ServerID:   a.ServerID,
		MetricType: "memory_usage",
		MetricName: "memory_shared",
		Value:      float64(v.Shared),
		Timestamp:  now,
	})
	metrics = append(metrics, Metric{
		ServerID:   a.ServerID,
		MetricType: "memory_usage",
		MetricName: "memory_buffers",
		Value:      float64(v.Buffers),
		Timestamp:  now,
	})
	metrics = append(metrics, Metric{
		ServerID:   a.ServerID,
		MetricType: "memory_usage",
		MetricName: "memory_cached",
		Value:      float64(v.Cached),
		Timestamp:  now,
	})

	// Swap
	s, _ := mem.SwapMemory()
	metrics = append(metrics, Metric{
		ServerID:   a.ServerID,
		MetricType: "swap_usage",
		MetricName: "swap_used_percent",
		Value:      s.UsedPercent,
		Timestamp:  now,
	})
	metrics = append(metrics, Metric{
		ServerID:   a.ServerID,
		MetricType: "swap_usage",
		MetricName: "swap_total",
		Value:      float64(s.Total),
		Timestamp:  now,
	})
	metrics = append(metrics, Metric{
		ServerID:   a.ServerID,
		MetricType: "swap_usage",
		MetricName: "swap_used",
		Value:      float64(s.Used),
		Timestamp:  now,
	})
	metrics = append(metrics, Metric{
		ServerID:   a.ServerID,
		MetricType: "swap_usage",
		MetricName: "swap_free",
		Value:      float64(s.Free),
		Timestamp:  now,
	})

	// Disk Usage
	partitions, _ := disk.Partitions(false)
	for _, p := range partitions {
		d, err := disk.Usage(p.Mountpoint)
		if err == nil {
			metrics = append(metrics, Metric{
				ServerID:   a.ServerID,
				MetricType: "disk_usage",
				MetricName: "disk_used_percent",
				Value:      d.UsedPercent,
				Labels:     map[string]interface{}{"path": p.Mountpoint, "fstype": p.Fstype},
				Timestamp:  now,
			})
			metrics = append(metrics, Metric{
				ServerID:   a.ServerID,
				MetricType: "disk_usage",
				MetricName: "disk_total",
				Value:      float64(d.Total),
				Labels:     map[string]interface{}{"path": p.Mountpoint},
				Timestamp:  now,
			})
			metrics = append(metrics, Metric{
				ServerID:   a.ServerID,
				MetricType: "disk_usage",
				MetricName: "disk_used",
				Value:      float64(d.Used),
				Labels:     map[string]interface{}{"path": p.Mountpoint},
				Timestamp:  now,
			})
		}
	}

	// Disk I/O
	diskIO, _ := disk.IOCounters()
	for _, io := range diskIO {
		metrics = append(metrics, Metric{
			ServerID:   a.ServerID,
			MetricType: "disk_io",
			MetricName: "read_bytes",
			Value:      float64(io.ReadBytes),
			Labels:     map[string]interface{}{"device": io.Name},
			Timestamp:  now,
		})
		metrics = append(metrics, Metric{
			ServerID:   a.ServerID,
			MetricType: "disk_io",
			MetricName: "write_bytes",
			Value:      float64(io.WriteBytes),
			Labels:     map[string]interface{}{"device": io.Name},
			Timestamp:  now,
		})
		metrics = append(metrics, Metric{
			ServerID:   a.ServerID,
			MetricType: "disk_io",
			MetricName: "read_count",
			Value:      float64(io.ReadCount),
			Labels:     map[string]interface{}{"device": io.Name},
			Timestamp:  now,
		})
		metrics = append(metrics, Metric{
			ServerID:   a.ServerID,
			MetricType: "disk_io",
			MetricName: "write_count",
			Value:      float64(io.WriteCount),
			Labels:     map[string]interface{}{"device": io.Name},
			Timestamp:  now,
		})
		metrics = append(metrics, Metric{
			ServerID:   a.ServerID,
			MetricType: "disk_io",
			MetricName: "read_time",
			Value:      float64(io.ReadTime),
			Labels:     map[string]interface{}{"device": io.Name},
			Timestamp:  now,
		})
		metrics = append(metrics, Metric{
			ServerID:   a.ServerID,
			MetricType: "disk_io",
			MetricName: "write_time",
			Value:      float64(io.WriteTime),
			Labels:     map[string]interface{}{"device": io.Name},
			Timestamp:  now,
		})
	}

	// Network (Total)
	netIO, _ := netstat.IOCounters(false)
	if len(netIO) > 0 {
		metrics = append(metrics, Metric{
			ServerID:   a.ServerID,
			MetricType: "network_usage",
			MetricName: "bytes_sent",
			Value:      float64(netIO[0].BytesSent),
			Timestamp:  now,
		})
		metrics = append(metrics, Metric{
			ServerID:   a.ServerID,
			MetricType: "network_usage",
			MetricName: "bytes_recv",
			Value:      float64(netIO[0].BytesRecv),
			Timestamp:  now,
		})
	}

	// Network (Per Interface)
	netInterfaces, _ := netstat.IOCounters(true)
	sysInterfaces, _ := net.Interfaces()
	ifaceStates := make(map[string]string)
	for _, iface := range sysInterfaces {
		state := "DOWN"
		if iface.Flags&net.FlagUp != 0 {
			state = "UP"
		}
		ifaceStates[iface.Name] = state
	}

	for _, io := range netInterfaces {
		state := ifaceStates[io.Name]
		if state == "" {
			state = "UNKNOWN"
		}
		
		labels := map[string]interface{}{"interface": io.Name, "state": state}
		
		metrics = append(metrics, Metric{
			ServerID:   a.ServerID,
			MetricType: "network_interface",
			MetricName: "bytes_sent",
			Value:      float64(io.BytesSent),
			Labels:     labels,
			Timestamp:  now,
		})
		metrics = append(metrics, Metric{
			ServerID:   a.ServerID,
			MetricType: "network_interface",
			MetricName: "bytes_recv",
			Value:      float64(io.BytesRecv),
			Labels:     labels,
			Timestamp:  now,
		})
		metrics = append(metrics, Metric{
			ServerID:   a.ServerID,
			MetricType: "network_interface",
			MetricName: "packets_sent",
			Value:      float64(io.PacketsSent),
			Labels:     labels,
			Timestamp:  now,
		})
		metrics = append(metrics, Metric{
			ServerID:   a.ServerID,
			MetricType: "network_interface",
			MetricName: "packets_recv",
			Value:      float64(io.PacketsRecv),
			Labels:     labels,
			Timestamp:  now,
		})
	}

	// Latency (Ping)
	start := time.Now()
	conn, err := net.DialTimeout("tcp", "8.8.8.8:53", 2*time.Second)
	if err == nil {
		conn.Close()
		latency := float64(time.Since(start).Milliseconds())
		metrics = append(metrics, Metric{
			ServerID:   a.ServerID,
			MetricType: "network_latency",
			MetricName: "latency_ms",
			Value:      latency,
			Timestamp:  now,
		})
	}

	return metrics, nil
}

func (a *Agent) CollectProcesses() ([]Process, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}

	var processes []Process
	for _, p := range procs {
		// Limit to top 30 processes to avoid payload bloat for now
		if len(processes) >= 30 {
			break
		}
		
		name, err := p.Name()
		if err != nil || name == "" {
			continue // Skip processes we can't get info for
		}
		
		cpu, _ := p.CPUPercent()
		mem, _ := p.MemoryPercent()
		username, _ := p.Username()
		status, _ := p.Status()
		threads, _ := p.NumThreads()
		io, _ := p.IOCounters()
		var readBytes, writeBytes uint64
		if io != nil {
			readBytes = io.ReadBytes
			writeBytes = io.WriteBytes
		}

		// Handle empty status array
		statusStr := "unknown"
		if len(status) > 0 {
			statusStr = status[0]
		}

		processes = append(processes, Process{
			PID:           p.Pid,
			Name:          name,
			CPUPercent:    cpu,
			MemoryPercent: mem,
			Username:      username,
			Status:        statusStr,
			Threads:       threads,
			IOReadBytes:   readBytes,
			IOWriteBytes:  writeBytes,
		})
	}
	return processes, nil
}

func (a *Agent) CollectContainers() ([]Container, error) {
	if a.DockerClient == nil {
		return nil, fmt.Errorf("docker client not initialized")
	}

	containers, err := a.DockerClient.ContainerList(context.Background(), types.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}

	var result []Container
	for _, c := range containers {
		name := "unknown"
		if len(c.Names) > 0 {
			name = c.Names[0]
			// Remove leading slash if present
			if len(name) > 0 && name[0] == '/' {
				name = name[1:]
			}
		}

		ports := ""
		for i, p := range c.Ports {
			if i > 0 {
				ports += ", "
			}
			ports += fmt.Sprintf("%d->%d/%s", p.PublicPort, p.PrivatePort, p.Type)
		}

		// Get container stats (simplified, might be slow for many containers)
		stats, err := a.DockerClient.ContainerStats(context.Background(), c.ID, false)
		var netRx, netTx, diskRead, diskWrite, memUsage uint64
		var cpuPercent float64
		if err == nil {
			var v types.StatsJSON
			if err := json.NewDecoder(stats.Body).Decode(&v); err == nil {
				// Network
				for _, net := range v.Networks {
					netRx += net.RxBytes
					netTx += net.TxBytes
				}
				// Disk I/O (BlkioStats)
				for _, entry := range v.BlkioStats.IoServiceBytesRecursive {
					if entry.Op == "Read" {
						diskRead += entry.Value
					} else if entry.Op == "Write" {
						diskWrite += entry.Value
					}
				}
				// Memory
				memUsage = v.MemoryStats.Usage
				if v.MemoryStats.Stats["cache"] != 0 {
					memUsage -= v.MemoryStats.Stats["cache"]
				}

				// CPU
				cpuDelta := float64(v.CPUStats.CPUUsage.TotalUsage) - float64(v.PreCPUStats.CPUUsage.TotalUsage)
				systemDelta := float64(v.CPUStats.SystemUsage) - float64(v.PreCPUStats.SystemUsage)
				onlineCPUs := float64(v.CPUStats.OnlineCPUs)
				if onlineCPUs == 0.0 {
					onlineCPUs = float64(len(v.CPUStats.CPUUsage.PercpuUsage))
				}
				if systemDelta > 0.0 && cpuDelta > 0.0 {
					cpuPercent = (cpuDelta / systemDelta) * onlineCPUs * 100.0
				}
			}
			stats.Body.Close()
		}

		result = append(result, Container{
			ID:             c.ID[:12],
			Name:           name,
			Image:          c.Image,
			State:          c.State,
			Status:         c.Status,
			Created:        c.Created,
			Ports:          ports,
			NetRxBytes:     netRx,
			NetTxBytes:     netTx,
			DiskReadBytes:  diskRead,
			DiskWriteBytes: diskWrite,
			CPUPercent:     cpuPercent,
			MemoryUsage:    memUsage,
		})
	}
	return result, nil
}

func (a *Agent) SendMetrics(metrics []Metric) error {
	req := IngestRequest{
		ServerID: a.ServerID,
		Metrics:  metrics,
	}
	jsonBody, _ := json.Marshal(req)
	resp, err := a.Client.Post(a.APIUrl+"/api/v1/ingest", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ingestion failed with status: %d", resp.StatusCode)
	}
	return nil
}

func (a *Agent) SendProcesses(processes []Process) error {
	snapshot := ProcessSnapshot{
		ServerID:  a.ServerID,
		Processes: processes,
		Timestamp: time.Now(),
	}
	jsonBody, _ := json.Marshal(snapshot)
	resp, err := a.Client.Post(a.APIUrl+"/api/v1/ingest/processes", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("process ingestion failed with status: %d", resp.StatusCode)
	}
	return nil
}

func (a *Agent) SendContainers(containers []Container) error {
	snapshot := ContainerSnapshot{
		ServerID:   a.ServerID,
		Containers: containers,
		Timestamp:  time.Now(),
	}
	jsonBody, _ := json.Marshal(snapshot)
	resp, err := a.Client.Post(a.APIUrl+"/api/v1/ingest/containers", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("container ingestion failed with status: %d", resp.StatusCode)
	}
	return nil
}

func (a *Agent) RunPortChecks() {
	// Fetch checks
	resp, err := a.Client.Get(fmt.Sprintf("%s/api/v1/ports/checks?server_id=%s", a.APIUrl, a.ServerID))
	if err != nil {
		log.Printf("Failed to fetch port checks: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	var checks []PortCheck
	if err := json.NewDecoder(resp.Body).Decode(&checks); err != nil {
		return
	}

	for _, check := range checks {
		start := time.Now()
		conn, err := net.DialTimeout(check.Protocol, fmt.Sprintf("%s:%d", check.Host, check.Port), 5*time.Second)
		duration := time.Since(start).Milliseconds()
		success := err == nil
		var errorMsg string
		if err != nil {
			errorMsg = err.Error()
		} else {
			conn.Close()
		}

		result := PortResult{
			CheckID:   check.ID,
			Success:   success,
			LatencyMS: duration,
			Error:     errorMsg,
			Timestamp: time.Now(),
		}

		// Send result
		jsonBody, _ := json.Marshal(result)
		a.Client.Post(a.APIUrl+"/api/v1/ports/results", "application/json", bytes.NewBuffer(jsonBody))
	}
}

func main() {
	// Force gopsutil to use host paths
	os.Setenv("HOST_PROC", "/host/proc")
	os.Setenv("HOST_SYS", "/host/sys")
	os.Setenv("HOST_ETC", "/host/etc")

	apiUrl := os.Getenv("API_URL")
	if apiUrl == "" {
		apiUrl = "http://backend:8080"
	}

	agent := NewAgent(apiUrl)

	// Retry registration loop
	for {
		if err := agent.Register(); err != nil {
			log.Printf("Registration failed: %v. Retrying in 5s...", err)
			time.Sleep(5 * time.Second)
			continue
		}
		break
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	portTicker := time.NewTicker(60 * time.Second)
	defer portTicker.Stop()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("Agent started for server %s (%s)", agent.Hostname, agent.ServerID)

	for {
		select {
		case <-ticker.C:
			metrics, err := agent.CollectMetrics()
			if err != nil {
				log.Printf("Error collecting metrics: %v", err)
				continue
			}

			if err := agent.SendMetrics(metrics); err != nil {
				log.Printf("Error sending metrics: %v", err)
			} else {
				log.Printf("Sent %d metrics", len(metrics))
			}
			
			// Collect and send processes
			processes, err := agent.CollectProcesses()
			if err != nil {
				log.Printf("Error collecting processes: %v", err)
			} else {
				if err := agent.SendProcesses(processes); err != nil {
					log.Printf("Error sending processes: %v", err)
				} else {
					log.Printf("Sent %d processes", len(processes))
				}
			}

			// Collect and send containers
			if agent.DockerClient != nil {
				containers, err := agent.CollectContainers()
				if err != nil {
					log.Printf("Error collecting containers: %v", err)
				} else {
					if err := agent.SendContainers(containers); err != nil {
						log.Printf("Error sending containers: %v", err)
					} else {
						log.Printf("Sent %d containers", len(containers))
					}
				}
			}
			
		case <-portTicker.C:
			go agent.RunPortChecks()
			
		case <-quit:
			log.Println("Shutting down agent...")
			return
		}
	}
}
