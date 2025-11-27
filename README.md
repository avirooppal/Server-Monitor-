# Real-Time Monitoring Backend

Minimal, production-ready monitoring backend with WebSocket streaming.

## 🚀 Quick Start

### Prerequisites
- Docker & Docker Compose
- Go 1.21+ (for local development)

### Run with Docker Compose

```bash
# Clone/create project structure
mkdir monitoring-backend
cd monitoring-backend

# Copy the files:
# - main.go
# - go.mod
# - go.sum (run `go mod download` to generate)
# - Dockerfile
# - docker-compose.yml

# Start the stack
docker-compose up -d

# Check logs
docker-compose logs -f backend

# Stop
docker-compose down
```

### Run Locally

```bash
# Install dependencies
go mod download

# Start PostgreSQL
docker run -d \
  --name postgres \
  -e POSTGRES_DB=monitoring \
  -e POSTGRES_USER=monitor \
  -e POSTGRES_PASSWORD=secret \
  -p 5432:5432 \
  postgres:16-alpine

# Run backend
export DATABASE_URL="postgres://monitor:secret@localhost:5432/monitoring?sslmode=disable"
go run main.go
```

## 📡 API Endpoints

### Health Check
```bash
curl http://localhost:8080/health
```

### Register Server
```bash
curl -X POST http://localhost:8080/api/v1/servers \
  -H "Content-Type: application/json" \
  -d '{
    "hostname": "server01",
    "ip_address": "192.168.1.10",
    "os": "Ubuntu 22.04",
    "labels": {"env": "prod", "role": "web"},
    "agent_version": "1.0.0"
  }'
```

### List Servers
```bash
curl http://localhost:8080/api/v1/servers
```

### Ingest Metrics (Bulk)
```bash
curl -X POST http://localhost:8080/api/v1/ingest \
  -H "Content-Type: application/json" \
  -d '{
    "server_id": "YOUR_SERVER_UUID",
    "metrics": [
      {
        "metric_type": "cpu",
        "metric_name": "usage",
        "value": 45.2,
        "labels": {"core": "0"},
        "timestamp": "2025-11-20T10:00:00Z"
      },
      {
        "metric_type": "ram",
        "metric_name": "usage_percent",
        "value": 78.5,
        "labels": {},
        "timestamp": "2025-11-20T10:00:00Z"
      }
    ]
  }'
```

### Query Metrics
```bash
# Get latest metrics for a server
curl "http://localhost:8080/api/v1/metrics?server_id=YOUR_UUID&metric_type=cpu"

# With time range
curl "http://localhost:8080/api/v1/metrics?server_id=YOUR_UUID&from=2025-11-20T00:00:00Z&to=2025-11-20T23:59:59Z"
```

### WebSocket Stream
```javascript
// Connect to real-time stream
const ws = new WebSocket('ws://localhost:8080/api/v1/stream?server_id=YOUR_UUID');

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('New metric:', data);
};

// Filter by metric type
const ws2 = new WebSocket('ws://localhost:8080/api/v1/stream?metric_type=cpu');
```

## 🏗️ Project Structure

```
monitoring-backend/
├── main.go              # All-in-one backend (DB, API, WebSocket)
├── go.mod               # Dependencies
├── go.sum               # Checksums
├── Dockerfile           # Container build
├── docker-compose.yml   # Full stack deployment
└── README.md           # This file
```

## 📊 Database Schema

### Servers Table
- `id` - UUID primary key
- `hostname` - Unique server hostname
- `ip_address` - Server IP
- `os` - Operating system
- `labels` - JSONB metadata
- `agent_version` - Agent version
- `last_seen` - Last metric received
- `created_at`, `updated_at` - Timestamps

### Metrics Table (Partitioned by Month)
- `id` - Auto-increment ID
- `server_id` - Foreign key to servers
- `metric_type` - Category (cpu, ram, disk, network)
- `metric_name` - Specific metric (usage, temperature)
- `value` - Numeric value
- `labels` - JSONB for additional context
- `timestamp` - When metric was collected

## 🔧 Configuration

Environment variables:

```bash
DATABASE_URL=postgres://user:pass@host:port/dbname?sslmode=disable
GIN_MODE=release  # or 'debug' for development
```

## 📈 Performance Features

- **Bulk Ingestion**: Uses PostgreSQL's COPY protocol for 10-100x faster inserts
- **Partitioning**: Automatic monthly partitions for metrics table
- **Indexing**: Optimized indexes for time-series queries
- **WebSocket**: Real-time metric streaming with filtering
- **Connection Pooling**: Efficient database connection management

## 🧪 Testing

### Test Server Registration
```bash
# Register a test server
SERVER_ID=$(curl -s -X POST http://localhost:8080/api/v1/servers \
  -H "Content-Type: application/json" \
  -d '{
    "hostname": "test-server",
    "ip_address": "10.0.0.1",
    "os": "Linux",
    "labels": {"env": "test"}
  }' | jq -r '.id')

echo "Server ID: $SERVER_ID"
```

### Send Test Metrics
```bash
# Send 100 test metrics
curl -X POST http://localhost:8080/api/v1/ingest \
  -H "Content-Type: application/json" \
  -d "{
    \"server_id\": \"$SERVER_ID\",
    \"metrics\": $(for i in {1..100}; do
      echo "{\"metric_type\":\"cpu\",\"metric_name\":\"usage\",\"value\":$((RANDOM % 100)),\"labels\":{},\"timestamp\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}"
    done | jq -s .)
  }"
```

### Query Metrics
```bash
curl "http://localhost:8080/api/v1/metrics?server_id=$SERVER_ID&metric_type=cpu" | jq
```

## 🎯 Next Steps

1. **Add Agent**: Create the Go agent that collects metrics
2. **Add Retention**: Implement cleanup of old metrics
3. **Add Aggregation**: Time-series aggregation endpoint
4. **Add Authentication**: API key or JWT auth
5. **Add Alerting**: Threshold-based alerts

## 📝 License

MIT