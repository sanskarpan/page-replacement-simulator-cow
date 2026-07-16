// Application state
const app = {
    ws: null,
    connected: false,
    currentAlgorithm: 'LRU',
    processes: [],
    frames: [],
    metrics: null,
};

// Initialize the application
function init() {
    console.log('Initializing Page Replacement Simulator...');

    // Connect WebSocket
    connectWebSocket();

    // Setup event listeners
    setupEventListeners();

    // Load scenarios
    loadScenarios();

    // Start periodic updates
    setInterval(updateUI, 1000);
}

// WebSocket connection
function connectWebSocket() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/api/ws`;

    console.log('Connecting to WebSocket:', wsUrl);
    app.ws = new WebSocket(wsUrl);

    app.ws.onopen = () => {
        console.log('WebSocket connected');
        app.connected = true;
        updateConnectionStatus(true);

        // Subscribe to metrics
        app.ws.send(JSON.stringify({
            type: 'subscribe_metrics'
        }));
    };

    app.ws.onclose = () => {
        console.log('WebSocket disconnected');
        app.connected = false;
        updateConnectionStatus(false);

        // Reconnect after 3 seconds
        setTimeout(connectWebSocket, 3000);
    };

    app.ws.onerror = (error) => {
        console.error('WebSocket error:', error);
    };

    app.ws.onmessage = (event) => {
        try {
            const message = JSON.parse(event.data);
            handleWebSocketMessage(message);
        } catch (e) {
            console.error('Failed to parse WebSocket message:', e);
        }
    };
}

// Handle WebSocket messages
function handleWebSocketMessage(message) {
    console.log('WebSocket message:', message.type);

    switch (message.type) {
        case 'initial_state':
        case 'state_update':
            if (message.status) {
                updateSystemStatus(message.status);
            }
            if (message.processes) {
                app.processes = message.processes;
                updateProcessList();
            }
            if (message.frames) {
                app.frames = message.frames;
                visualizeFrames();
            }
            break;

        case 'metrics_update':
            app.metrics = message.metrics;
            updateMetrics();
            break;

        case 'process_created':
        case 'process_terminated':
        case 'process_forked':
        case 'page_fault':
        case 'page_eviction':
        case 'cow_copy':
        case 'memory_access':
        case 'algorithm_changed':
        case 'simulation_complete':
        case 'simulation_error':
        case 'system_reset':
            addEvent(message.type, message);
            requestStateUpdate();
            break;
    }
}

// Update connection status
function updateConnectionStatus(connected) {
    const indicator = document.getElementById('connection-status');
    const text = document.getElementById('connection-text');

    if (connected) {
        indicator.classList.add('connected');
        text.textContent = 'Connected';
    } else {
        indicator.classList.remove('connected');
        text.textContent = 'Disconnected';
    }
}

// Setup event listeners
function setupEventListeners() {
    // Algorithm selection
    document.querySelectorAll('.algo-btn').forEach(btn => {
        btn.addEventListener('click', async () => {
            const algorithm = btn.dataset.algo;
            await setAlgorithm(algorithm);

            // Update UI
            document.querySelectorAll('.algo-btn').forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            app.currentAlgorithm = algorithm;
            document.getElementById('current-algorithm').textContent = algorithm;
        });
    });

    // Create process form
    document.getElementById('create-process-form').addEventListener('submit', async (e) => {
        e.preventDefault();

        const name = document.getElementById('process-name').value;
        const priority = parseInt(document.getElementById('process-priority').value);
        const virtualPages = parseInt(document.getElementById('virtual-pages').value);

        await createProcess(name, priority, virtualPages);

        // Reset form
        e.target.reset();
    });

    // Reset button
    document.getElementById('reset-btn').addEventListener('click', async () => {
        if (confirm('Are you sure you want to reset the system?')) {
            await resetSystem();
        }
    });
}

// API calls
async function apiCall(endpoint, method = 'GET', body = null) {
    const options = {
        method,
        headers: {
            'Content-Type': 'application/json'
        }
    };

    if (body) {
        options.body = JSON.stringify(body);
    }

    try {
        const response = await fetch(`/api${endpoint}`, options);
        if (!response.ok) {
            throw new Error(`API error: ${response.statusText}`);
        }
        return await response.json();
    } catch (error) {
        console.error('API call failed:', error);
        throw error;
    }
}

async function createProcess(name, priority, virtualPages) {
    return apiCall('/processes', 'POST', {
        name,
        priority,
        virtual_pages: virtualPages
    });
}

async function terminateProcess(pid) {
    return apiCall(`/processes/${pid}`, 'DELETE');
}

async function forkProcess(pid) {
    return apiCall(`/processes/${pid}/fork`, 'POST');
}

async function setAlgorithm(algorithm) {
    return apiCall('/memory/algorithm', 'POST', { algorithm });
}

async function runSimulation(scenario) {
    return apiCall('/simulation/run', 'POST', { scenario });
}

async function resetSystem() {
    return apiCall('/reset', 'POST');
}

async function loadScenarios() {
    try {
        const scenarios = await apiCall('/simulation/scenarios');
        const container = document.getElementById('scenarios-list');

        container.innerHTML = scenarios.map(s => `
            <button class="scenario-btn" data-scenario="${escapeHtml(s.name)}">
                <span class="scenario-name">${escapeHtml(s.name)}</span>
                <span class="scenario-desc">${escapeHtml(s.description)}</span>
            </button>
        `).join('');

        // Add click handlers
        container.querySelectorAll('.scenario-btn').forEach(btn => {
            btn.addEventListener('click', async () => {
                const scenario = btn.dataset.scenario;
                // Capture desc text before textContent wipes child nodes.
                const descEl = btn.querySelector('.scenario-desc');
                const desc = descEl ? descEl.textContent : '';
                btn.disabled = true;
                btn.textContent = 'Running...';

                try {
                    await runSimulation(scenario);
                } catch (error) {
                    console.error('Simulation failed:', error);
                } finally {
                    setTimeout(() => {
                        btn.disabled = false;
                        btn.innerHTML = `
                            <span class="scenario-name">${escapeHtml(scenario)}</span>
                            <span class="scenario-desc">${escapeHtml(desc)}</span>
                        `;
                    }, 1000);
                }
            });
        });
    } catch (error) {
        console.error('Failed to load scenarios:', error);
    }
}

// Request state update
function requestStateUpdate() {
    if (app.ws && app.connected) {
        app.ws.send(JSON.stringify({
            type: 'request_state'
        }));
    }
}

// Update system status
function updateSystemStatus(status) {
    if (!status) return;

    // Update uptime
    if (status.Uptime) {
        const seconds = Math.floor(status.Uptime / 1000000000);
        const minutes = Math.floor(seconds / 60);
        const hours = Math.floor(minutes / 60);

        let uptimeStr = '';
        if (hours > 0) {
            uptimeStr = `${hours}h ${minutes % 60}m`;
        } else if (minutes > 0) {
            uptimeStr = `${minutes}m ${seconds % 60}s`;
        } else {
            uptimeStr = `${seconds}s`;
        }

        document.getElementById('uptime').textContent = uptimeStr;
    }

    // Update process count
    if (status.ProcessCount !== undefined) {
        document.getElementById('process-count').textContent = status.ProcessCount;
    }

    // Update algorithm
    if (status.AlgorithmName) {
        app.currentAlgorithm = status.AlgorithmName;
        document.getElementById('current-algorithm').textContent = status.AlgorithmName;
    }

    // Update metrics
    if (status.Metrics) {
        app.metrics = status.Metrics;
        updateMetrics();
    }
}

// Update metrics display
function updateMetrics() {
    if (!app.metrics) return;

    const m = app.metrics;

    // Main metrics
    const faultRate = (m.PageFaultRate * 100).toFixed(1);
    const hitRate = (m.PageHitRate * 100).toFixed(1);
    const memUsage = m.TotalFrames > 0 ? ((m.UsedFrames / m.TotalFrames) * 100).toFixed(1) : 0;

    document.getElementById('fault-rate').textContent = `${faultRate}%`;
    document.getElementById('hit-rate').textContent = `${hitRate}%`;
    document.getElementById('memory-usage').textContent = `${memUsage}%`;
    document.getElementById('cow-copies').textContent = m.CoWCopies || 0;

    // Statistics
    document.getElementById('total-accesses').textContent = m.TotalAccesses || 0;
    document.getElementById('page-faults').textContent = m.PageFaults || 0;
    document.getElementById('page-hits').textContent = m.PageHits || 0;
    document.getElementById('evictions').textContent = m.Evictions || 0;
    document.getElementById('used-frames').textContent = m.UsedFrames || 0;
    document.getElementById('free-frames').textContent = m.FreeFrames || 0;
    document.getElementById('shared-pages').textContent = m.SharedPages || 0;
    document.getElementById('dirty-pages').textContent = m.DirtyPages || 0;
}

// HTML escaping utility
function escapeHtml(str) {
    const div = document.createElement('div');
    div.appendChild(document.createTextNode(str));
    return div.innerHTML;
}

// Update process list
function updateProcessList() {
    const container = document.getElementById('process-list');

    if (!app.processes || app.processes.length === 0) {
        container.innerHTML = '<p class="empty-state">No processes running</p>';
        return;
    }

    container.innerHTML = app.processes.map(p => `
        <div class="process-item">
            <div class="process-header">
                <span class="process-name">${escapeHtml(p.Name)} (${escapeHtml(p.ID)})</span>
                <div class="process-actions">
                    <button class="fork-btn" data-pid="${escapeHtml(p.ID)}">Fork</button>
                    <button class="delete-btn kill-btn" data-pid="${escapeHtml(p.ID)}">Kill</button>
                </div>
            </div>
            <div class="process-stats">
                <div>State: ${escapeHtml(p.State)}</div>
                <div>Fault Rate: ${(p.PageFaultRate * 100).toFixed(1)}%</div>
                <div>Hit Rate: ${(p.PageHitRate * 100).toFixed(1)}%</div>
                <div>Pages: ${p.PresentPages}/${p.TotalPages}</div>
                ${p.SharedPages > 0 ? `<div>Shared: ${p.SharedPages}</div>` : ''}
                ${p.CoWCopies > 0 ? `<div>CoW Copies: ${p.CoWCopies}</div>` : ''}
            </div>
        </div>
    `).join('');

    container.querySelectorAll('.fork-btn').forEach(btn => {
        btn.addEventListener('click', () => handleForkProcess(btn.dataset.pid));
    });
    container.querySelectorAll('.kill-btn').forEach(btn => {
        btn.addEventListener('click', () => handleTerminateProcess(btn.dataset.pid));
    });
}

// Process actions
async function handleTerminateProcess(pid) {
    try {
        await terminateProcess(pid);
    } catch (error) {
        console.error('Failed to terminate process:', error);
        alert('Failed to terminate process');
    }
}

async function handleForkProcess(pid) {
    try {
        await forkProcess(pid);
    } catch (error) {
        console.error('Failed to fork process:', error);
        alert('Failed to fork process');
    }
}

// Visualize frames using D3.js
function visualizeFrames() {
    const container = d3.select('#frame-visualization');
    container.html('');

    if (!app.frames || app.frames.length === 0) {
        container.append('p')
            .attr('class', 'empty-state')
            .text('No frames to display');
        return;
    }

    const width = container.node().offsetWidth;
    const frameSize = 70;
    const framesPerRow = Math.floor(width / frameSize);

    const svg = container.append('svg')
        .attr('width', width)
        .attr('height', Math.ceil(app.frames.length / framesPerRow) * frameSize);

    const frames = svg.selectAll('.frame-rect')
        .data(app.frames)
        .enter()
        .append('g')
        .attr('class', 'frame-group')
        .attr('transform', (d, i) => {
            const row = Math.floor(i / framesPerRow);
            const col = i % framesPerRow;
            return `translate(${col * frameSize}, ${row * frameSize})`;
        });

    // Draw frame rectangles
    frames.append('rect')
        .attr('width', frameSize - 10)
        .attr('height', frameSize - 10)
        .attr('x', 5)
        .attr('y', 5)
        .attr('rx', 6)
        .attr('fill', d => {
            if (d.Free) return '#ecf0f1';
            if (d.Modified) return '#e74c3c';
            if (d.ProcessID) return '#667eea';
            return '#95a5a6';
        })
        .attr('stroke', d => {
            if (d.Free) return '#bdc3c7';
            if (d.Modified) return '#c0392b';
            return '#5568d3';
        })
        .attr('stroke-width', 2);

    // Frame ID
    frames.append('text')
        .attr('x', frameSize / 2)
        .attr('y', 25)
        .attr('text-anchor', 'middle')
        .attr('fill', d => d.Free ? '#333' : '#fff')
        .attr('font-size', '14px')
        .attr('font-weight', 'bold')
        .text(d => `F${d.ID}`);

    // Page ID (if not free)
    frames.append('text')
        .attr('x', frameSize / 2)
        .attr('y', 42)
        .attr('text-anchor', 'middle')
        .attr('fill', d => d.Free ? '#333' : '#fff')
        .attr('font-size', '10px')
        .attr('opacity', 0.8)
        .text(d => d.Free ? '' : `P${d.PageID}`);

    // Process ID (if not free)
    frames.append('text')
        .attr('x', frameSize / 2)
        .attr('y', 55)
        .attr('text-anchor', 'middle')
        .attr('fill', d => d.Free ? '#333' : '#fff')
        .attr('font-size', '9px')
        .attr('opacity', 0.7)
        .text(d => d.Free ? '' : d.ProcessID || '');
}

// Add event to log
const maxEvents = 50;
const eventLog = [];

function addEvent(type, data) {
    const event = {
        type,
        data,
        timestamp: new Date()
    };

    eventLog.unshift(event);
    if (eventLog.length > maxEvents) {
        eventLog.pop();
    }

    updateEventLog();
}

function updateEventLog() {
    const container = document.getElementById('event-log');

    container.innerHTML = eventLog.map(e => {
        const time = e.timestamp.toLocaleTimeString();
        const typeDisplay = e.type.replace(/_/g, ' ').toUpperCase();

        let dataStr = '';
        if (e.data) {
            if (e.data.process_id) dataStr += `PID: ${escapeHtml(String(e.data.process_id))}`;
            if (e.data.virtual_page) dataStr += ` Page: ${escapeHtml(String(e.data.virtual_page))}`;
            if (e.data.scenario) dataStr += `Scenario: ${escapeHtml(String(e.data.scenario))}`;
            if (e.data.algorithm) dataStr += `Algo: ${escapeHtml(String(e.data.algorithm))}`;
        }

        return `
            <div class="event-item">
                <span class="event-time">${time}</span>
                <span class="event-type">${typeDisplay}</span>
                <span class="event-data">${dataStr}</span>
            </div>
        `;
    }).join('');
}

// Periodic UI update
function updateUI() {
    // Request state update if connected
    if (app.connected && app.ws) {
        requestStateUpdate();
    }
}

// Initialize on load
window.addEventListener('DOMContentLoaded', init);
