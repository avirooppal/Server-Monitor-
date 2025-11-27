/**
 * Server Monitor Application
 * Handles WebSocket connections, data fetching, and UI updates.
 */

class App {
    constructor() {
        this.state = {
            currentView: 'overview',
            serverId: null,
            ws: null,
            isConnected: false,
            lastUpdate: null,
            metrics: {
                cpu: {},
                ram: {},
                network: {},
                network_interfaces: {},
                disk: {},
                partitions: {},
                processes: [],
                containers: []
            },
            networkHistory: {}, // { interface: { rx: [], tx: [], lastUpdate: ts } }
            prevProcesses: {}, // { pid: { read: val, write: val, ts: time } }
            prevCpuTimes: null
        };

        this.elements = {};
        this.init();
    }

    init() {
        this.cacheDOM();
        this.bindEvents();
        this.loadInitialData();
    }

    cacheDOM() {
        // Navigation
        this.elements.navItems = document.querySelectorAll('.nav-item');
        this.elements.views = document.querySelectorAll('.view');

        // Status
        this.elements.statusIndicator = document.querySelector('.status-indicator');
        this.elements.statusLabel = document.querySelector('.status-label');
        this.elements.lastUpdateTime = document.getElementById('last-update-time');

        // Server Info
        this.elements.serverHostname = document.getElementById('server-hostname');
        this.elements.serverIp = document.getElementById('server-ip');
        this.elements.serverOs = document.getElementById('server-os');
        this.elements.serverUptime = document.getElementById('server-uptime');

        // System Info
        this.elements.sysCpuModel = document.getElementById('sys-cpu-model');
        this.elements.sysKernel = document.getElementById('sys-kernel');
        this.elements.sysTotalRam = document.getElementById('sys-total-ram');
        this.elements.sysCores = document.getElementById('sys-cores');

        // CPU
        this.elements.cpuUsageVal = document.getElementById('cpu-usage-val');
        this.elements.cpuLoadVal = document.getElementById('cpu-load-val');
        this.elements.cpuCoresList = document.getElementById('cpu-cores-list');

        // Memory
        this.elements.ramUsagePercent = document.getElementById('ram-usage-percent');
        this.elements.ramUsageAbs = document.getElementById('ram-usage-abs');
        this.elements.swapUsagePercent = document.getElementById('swap-usage-percent');
        this.elements.swapUsageAbs = document.getElementById('swap-usage-abs');

        // Disk
        this.elements.diskIoRead = document.getElementById('disk-io-read');
        this.elements.diskIoWrite = document.getElementById('disk-io-write');
        this.elements.diskPartitionsList = document.getElementById('disk-partitions-list');

        // Network
        this.elements.netInRate = document.getElementById('net-in-rate');
        this.elements.netOutRate = document.getElementById('net-out-rate');
        this.elements.netLatencyVal = document.getElementById('net-latency-val');
        this.elements.netInterfacesList = document.getElementById('net-interfaces-list');

        // Tables
        this.elements.processesBody = document.getElementById('processes-tbody');
        this.elements.containersBody = document.getElementById('containers-tbody');
    }

    bindEvents() {
        this.elements.navItems.forEach(item => {
            item.addEventListener('click', (e) => {
                const viewId = e.currentTarget.dataset.view;
                this.switchView(viewId);
            });
        });
    }

    switchView(viewId) {
        this.elements.navItems.forEach(item => {
            item.classList.toggle('active', item.dataset.view === viewId);
        });
        this.elements.views.forEach(view => {
            view.classList.toggle('active', view.id === `view-${viewId}`);
        });
        this.state.currentView = viewId;
    }

    async loadInitialData() {
        try {
            const servers = await this.fetchAPI('/api/v1/servers');
            if (servers && servers.length > 0) {
                servers.sort((a, b) => new Date(b.last_seen) - new Date(a.last_seen));
                const activeServer = servers[0];

                this.state.serverId = activeServer.id;
                this.updateServerInfo(activeServer);
                this.connectWebSocket();
                this.fetchAllMetrics();
            }
        } catch (error) {
            console.error('Failed to load initial data:', error);
            this.elements.serverHostname.textContent = 'Connection Failed';
        }
    }

    updateServerInfo(server) {
        this.elements.serverHostname.textContent = server.hostname;
        this.elements.serverIp.textContent = server.ip_address;
        this.elements.serverOs.textContent = server.os;
        this.elements.serverUptime.textContent = server.uptime;

        this.elements.sysCpuModel.textContent = server.cpu_model;
        this.elements.sysKernel.textContent = server.kernel_version;
        this.elements.sysTotalRam.textContent = server.total_ram;
    }

    connectWebSocket() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/api/v1/stream?server_id=${this.state.serverId}`;

        this.state.ws = new WebSocket(wsUrl);

        this.state.ws.onopen = () => {
            this.state.isConnected = true;
            this.updateConnectionStatus();
        };

        this.state.ws.onclose = () => {
            this.state.isConnected = false;
            this.updateConnectionStatus();
            setTimeout(() => this.connectWebSocket(), 3000);
        };

        this.state.ws.onmessage = (event) => {
            try {
                const msg = JSON.parse(event.data);
                this.handleWebSocketMessage(msg);
            } catch (e) {
                console.error('WS Parse Error:', e);
            }
        };
    }

    updateConnectionStatus() {
        if (this.state.isConnected) {
            this.elements.statusIndicator.classList.add('connected');
            this.elements.statusLabel.textContent = 'Connected';
        } else {
            this.elements.statusIndicator.classList.remove('connected');
            this.elements.statusLabel.textContent = 'Disconnected';
        }
    }

    handleWebSocketMessage(msg) {
        this.state.lastUpdate = new Date();
        this.elements.lastUpdateTime.textContent = this.state.lastUpdate.toLocaleTimeString();

        switch (msg.type) {
            case 'metric':
                this.handleMetricUpdate(msg.data);
                break;
            case 'process_snapshot':
                this.handleProcessSnapshot(msg.data);
                break;
            case 'container_snapshot':
                this.handleContainerSnapshot(msg.data);
                break;
        }
    }

    handleMetricUpdate(metric) {
        if (metric.server_id !== this.state.serverId) return;

        const type = metric.metric_type;
        const name = metric.metric_name;
        const val = metric.value;
        const labels = metric.labels || {};

        if (!this.state.metrics[type]) this.state.metrics[type] = {};

        switch (type) {
            case 'cpu_usage':
                if (name === 'cpu_total') {
                    this.updateText(this.elements.cpuUsageVal, val.toFixed(1) + '%');
                }
                break;
            case 'vcpu_usage':
                this.state.metrics.vcpu_usage = this.state.metrics.vcpu_usage || {};
                this.state.metrics.vcpu_usage[name] = val;
                this.renderVcpuList();
                break;
            case 'load_average':
                if (name === 'load1') this.state.metrics.load1 = val;
                if (name === 'load5') this.state.metrics.load5 = val;
                if (name === 'load15') this.state.metrics.load15 = val;
                this.updateText(this.elements.cpuLoadVal,
                    `${(this.state.metrics.load1 || 0).toFixed(2)} / ${(this.state.metrics.load5 || 0).toFixed(2)} / ${(this.state.metrics.load15 || 0).toFixed(2)}`);
                break;
            case 'memory_usage':
                if (name === 'memory_used_percent') this.updateText(this.elements.ramUsagePercent, val.toFixed(1) + '%');
                if (name === 'memory_used') this.state.metrics.ramUsed = val;
                if (name === 'memory_total') this.state.metrics.ramTotal = val;
                if (this.state.metrics.ramUsed && this.state.metrics.ramTotal) {
                    this.updateText(this.elements.ramUsageAbs,
                        `${(this.state.metrics.ramUsed / 1024 / 1024 / 1024).toFixed(1)} GB / ${(this.state.metrics.ramTotal / 1024 / 1024 / 1024).toFixed(1)} GB`);
                }
                break;
            case 'swap_usage':
                if (name === 'swap_used_percent') this.updateText(this.elements.swapUsagePercent, val.toFixed(1) + '%');
                if (name === 'swap_used') this.state.metrics.swapUsed = val;
                if (name === 'swap_total') this.state.metrics.swapTotal = val;
                if (this.state.metrics.swapUsed && this.state.metrics.swapTotal) {
                    this.updateText(this.elements.swapUsageAbs,
                        `${(this.state.metrics.swapUsed / 1024 / 1024 / 1024).toFixed(1)} GB / ${(this.state.metrics.swapTotal / 1024 / 1024 / 1024).toFixed(1)} GB`);
                }
                break;
            case 'network_latency':
                this.updateText(this.elements.netLatencyVal, `${val.toFixed(0)} ms`);
                break;
            case 'network_interface':
                this.handleNetworkInterface(name, val, labels);
                break;
            case 'disk_usage':
                this.handleDiskUsage(name, val, labels);
                break;
        }
    }

    handleNetworkInterface(name, val, labels) {
        const iface = labels.interface;
        if (!iface) return;

        if (!this.state.metrics.network_interfaces[iface]) {
            this.state.metrics.network_interfaces[iface] = { state: labels.state || 'UNKNOWN' };
        }

        // Update current value
        this.state.metrics.network_interfaces[iface][name] = val;
        this.state.metrics.network_interfaces[iface].state = labels.state || this.state.metrics.network_interfaces[iface].state;

        // Update History for Rates
        if (!this.state.networkHistory[iface]) {
            this.state.networkHistory[iface] = { rx: [], tx: [], lastUpdate: Date.now() };
        }

        const now = Date.now();
        const history = this.state.networkHistory[iface];

        // We need to calculate RATE (bytes/sec) from cumulative bytes
        // But the metric stream gives us current cumulative bytes.
        // We need to store [timestamp, value] to calc rate.

        if (name === 'bytes_recv') {
            this.updateInterfaceHistory(history.rx, val, now);
        } else if (name === 'bytes_sent') {
            this.updateInterfaceHistory(history.tx, val, now);
        }

        this.renderNetworkInterfaces();
    }

    updateInterfaceHistory(historyArray, val, now) {
        // historyArray contains { ts, val, rate }
        // We only keep last 60 seconds of RATES.

        // If this is the first point, just store it
        if (historyArray.length === 0) {
            historyArray.push({ ts: now, val: val, rate: 0 });
            return;
        }

        const last = historyArray[historyArray.length - 1];
        const timeDiff = (now - last.ts) / 1000; // seconds

        if (timeDiff > 0) {
            const rate = (val - last.val) / timeDiff; // bytes/sec
            historyArray.push({ ts: now, val: val, rate: Math.max(0, rate) });

            // Prune old data (> 60s)
            while (historyArray.length > 0 && (now - historyArray[0].ts) > 65000) {
                historyArray.shift();
            }
        }
    }

    handleDiskUsage(name, val, labels) {
        const path = labels.path;
        if (!path) return;

        if (!this.state.metrics.partitions[path]) {
            this.state.metrics.partitions[path] = {};
        }

        this.state.metrics.partitions[path][name] = val;
        this.renderPartitions();
    }

    renderVcpuList() {
        const vcpus = Object.entries(this.state.metrics.vcpu_usage || {})
            .sort((a, b) => {
                const idxA = parseInt(a[0].split('_')[1]);
                const idxB = parseInt(b[0].split('_')[1]);
                return idxA - idxB;
            });

        if (vcpus.length === 0) return;

        this.updateText(this.elements.sysCores, vcpus.length.toString());

        const html = vcpus.map(([name, val]) => {
            const idx = name.split('_')[1];
            return `
                <li>
                    <span class="key">Core ${idx}</span>
                    <span class="val">${val.toFixed(0)}%</span>
                </li>
            `;
        }).join('');

        if (this.elements.cpuCoresList) {
            this.elements.cpuCoresList.innerHTML = html;
        }
    }

    renderNetworkInterfaces() {
        const interfaces = this.state.metrics.network_interfaces;
        const history = this.state.networkHistory;

        const html = Object.keys(interfaces).sort().map(iface => {
            const data = interfaces[iface];
            const hist = history[iface] || { rx: [], tx: [] };

            // Calculate Averages
            const rx1s = this.calculateAverage(hist.rx, 1);
            const rx10s = this.calculateAverage(hist.rx, 10);
            const rx1m = this.calculateAverage(hist.rx, 60);

            const tx1s = this.calculateAverage(hist.tx, 1);
            const tx10s = this.calculateAverage(hist.tx, 10);
            const tx1m = this.calculateAverage(hist.tx, 60);

            return `
                <li class="interface-item">
                    <div class="interface-header">
                        <span class="key">${iface}</span>
                        <span class="badge ${data.state === 'UP' ? 'success' : 'error'}">${data.state}</span>
                    </div>
                    <div class="interface-stats">
                        <div class="stat-row">
                            <span class="label">In:</span>
                            <span class="val">${this.formatBits(rx1s)}</span>
                            <span class="sub-val">(${this.formatBits(rx10s)} / ${this.formatBits(rx1m)})</span>
                        </div>
                        <div class="stat-row">
                            <span class="label">Out:</span>
                            <span class="val">${this.formatBits(tx1s)}</span>
                            <span class="sub-val">(${this.formatBits(tx10s)} / ${this.formatBits(tx1m)})</span>
                        </div>
                    </div>
                </li>
            `;
        }).join('');

        if (this.elements.netInterfacesList) {
            this.elements.netInterfacesList.innerHTML = html;
        }
    }

    calculateAverage(historyArray, seconds) {
        if (!historyArray || historyArray.length === 0) return 0;
        const now = Date.now();
        // Filter points within the window
        const points = historyArray.filter(p => (now - p.ts) <= seconds * 1000);
        if (points.length === 0) return 0;

        const sum = points.reduce((acc, p) => acc + p.rate, 0);
        return sum / points.length;
    }

    renderPartitions() {
        const partitions = this.state.metrics.partitions;
        const html = Object.keys(partitions).sort().map(path => {
            const p = partitions[path];
            const used = p.disk_used || 0;
            const total = p.disk_total || 0;
            const percent = p.disk_used_percent || 0;

            return `
                <li>
                    <span class="key">${path}</span>
                    <div class="partition-details">
                        <span class="val">${percent.toFixed(0)}%</span>
                        <span class="sub-val">${this.formatBytes(used)} / ${this.formatBytes(total)}</span>
                    </div>
                </li>
            `;
        }).join('');

        this.elements.diskPartitionsList.innerHTML = html || '<li>No partitions</li>';
    }

    handleProcessSnapshot(data) {
        const processes = data.processes || [];
        const now = Date.now();

        // Calculate I/O Rates
        processes.forEach(p => {
            const prev = this.state.prevProcesses[p.pid];
            if (prev) {
                const timeDiff = (now - prev.ts) / 1000;
                if (timeDiff > 0) {
                    p.read_rate = Math.max(0, (p.io_read_bytes - prev.read) / timeDiff);
                    p.write_rate = Math.max(0, (p.io_write_bytes - prev.write) / timeDiff);
                }
            } else {
                p.read_rate = 0;
                p.write_rate = 0;
            }

            // Update prev state
            this.state.prevProcesses[p.pid] = {
                read: p.io_read_bytes,
                write: p.io_write_bytes,
                ts: now
            };
        });

        // Prune old processes from prev state
        const currentPids = new Set(processes.map(p => p.pid));
        for (const pid in this.state.prevProcesses) {
            if (!currentPids.has(parseInt(pid))) {
                delete this.state.prevProcesses[pid];
            }
        }

        this.state.metrics.processes = processes;
        this.renderProcesses(processes);
    }

    handleContainerSnapshot(data) {
        const containers = data.containers || [];
        this.state.metrics.containers = containers;
        this.renderContainers(containers);
    }

    renderProcesses(processes) {
        if (processes.length === 0) {
            this.elements.processesBody.innerHTML = '<tr><td colspan="7" class="empty">No processes found</td></tr>';
            return;
        }

        this.elements.processesBody.innerHTML = processes.map(p => `
            <tr>
                <td>${p.pid}</td>
                <td>${p.name}</td>
                <td>${p.username || '-'}</td>
                <td>${p.cpu_percent.toFixed(1)}%</td>
                <td>${p.memory_percent.toFixed(1)}%</td>
                <td>${this.formatBytes(p.read_rate || 0)}/s</td>
                <td>${this.formatBytes(p.write_rate || 0)}/s</td>
            </tr>
        `).join('');
    }

    renderContainers(containers) {
        if (containers.length === 0) {
            this.elements.containersBody.innerHTML = '<tr><td colspan="6" class="empty">No containers found</td></tr>';
            return;
        }

        this.elements.containersBody.innerHTML = containers.map(c => `
            <tr>
                <td>${c.name}</td>
                <td>${c.state}</td>
                <td>${(c.cpu_percent || 0).toFixed(1)}%</td>
                <td>${this.formatBytes(c.memory_usage || 0)}</td>
                <td>RX: ${this.formatBytes(c.net_rx_bytes || 0)} / TX: ${this.formatBytes(c.net_tx_bytes || 0)}</td>
                <td>R: ${this.formatBytes(c.disk_read_bytes || 0)} / W: ${this.formatBytes(c.disk_write_bytes || 0)}</td>
            </tr>
        `).join('');
    }

    async fetchAllMetrics() {
        if (!this.state.serverId) return;

        // Fetch initial data for rates if needed, but WebSocket handles updates.
        // We still fetch disk partitions initially to populate the list quickly.
        const diskMetrics = await this.fetchAPI(`/api/v1/metrics?server_id=${this.state.serverId}&metric_type=disk_usage&limit=20`);
        if (diskMetrics && diskMetrics.metrics) {
            diskMetrics.metrics.forEach(m => {
                this.handleDiskUsage(m.metric_name, m.value, m.labels);
            });
        }
    }

    updateText(element, text) {
        if (element && element.textContent !== text) {
            element.textContent = text;
        }
    }

    formatBytes(bytes) {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
    }

    formatBits(bytesPerSec) {
        const bits = bytesPerSec * 8;
        if (bits === 0) return '0 bps';
        const k = 1000;
        const sizes = ['bps', 'Kbps', 'Mbps', 'Gbps'];
        const i = Math.floor(Math.log(bits) / Math.log(k));
        return parseFloat((bits / Math.pow(k, i)).toFixed(1)) + ' ' + (sizes[i] || 'bps');
    }

    async fetchAPI(url) {
        const separator = url.includes('?') ? '&' : '?';
        const response = await fetch(`${url}${separator}_t=${Date.now()}`);
        if (!response.ok) throw new Error(`API Error: ${response.status}`);
        return await response.json();
    }
}

document.addEventListener('DOMContentLoaded', () => {
    window.app = new App();
    setInterval(() => {
        if (window.app.state.serverId) {
            window.app.fetchAllMetrics();
        }
    }, 5000);
});
