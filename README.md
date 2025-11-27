# Server Monitor

A real-time server monitoring solution with a Go backend, lightweight agent, and responsive web dashboard.

## Features

- **Real-Time Monitoring**: Live updates for CPU, RAM, Disk, and Network usage via WebSockets.
- **Host Metrics**: Collects metrics from the underlying host system (even when running in Docker).
- **Process Monitoring**: Tracks top processes by CPU and Memory usage.
- **Container Monitoring**: Monitors Docker containers running on the host.
- **Historical Data**: View historical trends for network usage.
- **Responsive Dashboard**: Clean, dark-mode UI built with vanilla HTML/CSS/JS.

## Technology Stack

- **Backend**: Go (Gin framework), PostgreSQL (TimescaleDB-ready schema).
- **Agent**: Go (gopsutil for metrics), Docker SDK.
- **Frontend**: Vanilla JavaScript, CSS, HTML.
- **Infrastructure**: Docker, Docker Compose.

## Quick Start

### Prerequisites

- Docker & Docker Compose

### Installation

1. Clone the repository:
   ```bash
   git clone <repository-url>
   cd server-monitor
   ```

2. Start the application:
   ```bash
   docker-compose up -d --build
   ```

3. Access the dashboard:
   Open http://localhost:8082/web/ in your browser.

## Project Structure

- `agent/`: Go agent that collects system metrics and sends them to the backend.
- `backend/`: Go API server that ingests metrics and serves the frontend.
- `web/`: Static frontend files (HTML, CSS, JS).
- `docker-compose.yml`: Orchestration for Agent, Backend, and PostgreSQL.

## Configuration

The agent is configured via environment variables in `docker-compose.yml`:

- `API_URL`: URL of the backend API (default: `http://backend:8080`).
- `HOST_PROC`: Path to host /proc (default: `/host/proc`).
- `HOST_SYS`: Path to host /sys (default: `/host/sys`).
- `HOST_ETC`: Path to host /etc (default: `/host/etc`).

## API Endpoints

- `GET /api/v1/servers`: List registered servers.
- `GET /api/v1/metrics`: Query historical metrics.
- `GET /api/v1/stream`: WebSocket endpoint for real-time updates.

