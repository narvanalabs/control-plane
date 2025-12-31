// Live Log Streaming for Narvana Control Plane
// Uses Server-Sent Events (SSE) to stream logs in real-time
// Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6

(function () {
    'use strict';

    // Configuration
    const MAX_LOG_LINES = 5000; // Requirements: 8.3
    const MAX_RECONNECT_ATTEMPTS = 10;
    const BASE_RECONNECT_DELAY = 1000; // 1 second
    const MAX_RECONNECT_DELAY = 30000; // 30 seconds

    // State
    let eventSource = null;
    let isPaused = false; // Requirements: 8.4, 8.5
    let reconnectAttempts = 0;
    let reconnectTimeout = null;

    // Initialize log streaming when the page loads
    document.addEventListener('DOMContentLoaded', function () {
        initLogStream();
        initAutoRefresh();
    });

    // Log streaming via SSE
    function initLogStream() {
        const logContainer = document.getElementById('live-logs');
        if (!logContainer) return;

        const appId = logContainer.dataset.appId;
        const deploymentId = logContainer.dataset.deploymentId;
        if (!appId) return;

        const streamUrl = buildStreamUrl(appId, deploymentId);

        // Set up control buttons
        setupControls(logContainer, appId, deploymentId);

        // Start streaming
        connect(streamUrl, logContainer);

        // Cleanup on page unload
        window.addEventListener('beforeunload', function () {
            cleanup();
        });
    }

    function buildStreamUrl(appId, deploymentId) {
        let url = `/api/logs/stream?app_id=${appId}`;
        if (deploymentId) {
            url += `&deployment_id=${deploymentId}`;
        }
        return url;
    }

    function connect(streamUrl, logContainer) {
        if (isPaused) return;

        // Close existing connection
        if (eventSource) {
            eventSource.close();
        }

        eventSource = new EventSource(streamUrl);

        eventSource.onopen = function () {
            console.log('Log stream connected');
            reconnectAttempts = 0;
            updateStreamStatus('connected');
        };

        // Handle named events
        eventSource.addEventListener('connected', function (event) {
            try {
                const data = JSON.parse(event.data);
                console.log('Stream connected:', data);
            } catch (e) {
                console.error('Failed to parse connected event:', e);
            }
        });

        eventSource.addEventListener('log', function (event) {
            if (isPaused) return;
            try {
                const log = JSON.parse(event.data);
                appendLog(logContainer, log);
                enforceLineLimit(logContainer); // Requirements: 8.3
            } catch (e) {
                console.error('Failed to parse log event:', e);
            }
        });

        eventSource.addEventListener('ping', function (event) {
            // Keep-alive ping, no action needed
        });

        eventSource.addEventListener('new_deployment', function (event) {
            try {
                const data = JSON.parse(event.data);
                console.log('New deployment detected:', data.deployment_id);
                // Optionally clear logs for new deployment
                // logContainer.innerHTML = '';
            } catch (e) {
                console.error('Failed to parse new_deployment event:', e);
            }
        });

        eventSource.addEventListener('deployment_status', function (event) {
            try {
                const data = JSON.parse(event.data);
                console.log('Deployment status:', data.status);
                updateDeploymentStatus(data.status);
            } catch (e) {
                console.error('Failed to parse deployment_status event:', e);
            }
        });

        // Fallback for generic messages
        eventSource.onmessage = function (event) {
            if (isPaused) return;
            try {
                const log = JSON.parse(event.data);
                if (log.message) {
                    appendLog(logContainer, log);
                    enforceLineLimit(logContainer);
                }
            } catch (e) {
                // Plain text log
                appendPlainLog(logContainer, event.data);
                enforceLineLimit(logContainer);
            }
        };

        eventSource.onerror = function () {
            console.log('Log stream disconnected');
            updateStreamStatus('disconnected');
            eventSource.close();

            // Reconnect with exponential backoff - Requirements: 8.6
            scheduleReconnect(streamUrl, logContainer);
        };
    }

    // Exponential backoff reconnection - Requirements: 8.6
    function scheduleReconnect(streamUrl, logContainer) {
        if (isPaused) return;

        if (reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
            console.log('Max reconnect attempts reached');
            updateStreamStatus('failed');
            return;
        }

        reconnectAttempts++;
        const delay = Math.min(
            BASE_RECONNECT_DELAY * Math.pow(2, reconnectAttempts - 1),
            MAX_RECONNECT_DELAY
        );

        console.log(`Reconnecting in ${delay}ms (attempt ${reconnectAttempts}/${MAX_RECONNECT_ATTEMPTS})`);
        updateStreamStatus('reconnecting', delay);

        reconnectTimeout = setTimeout(function () {
            connect(streamUrl, logContainer);
        }, delay);
    }

    // Set up pause/resume and other controls - Requirements: 8.4, 8.5
    function setupControls(logContainer, appId, deploymentId) {
        const pauseBtn = document.getElementById('pause-btn');
        const copyBtn = document.getElementById('copy-btn');
        const clearBtn = document.getElementById('clear-btn');
        const searchInput = document.getElementById('log-search');

        // Pause/Resume - Requirements: 8.4, 8.5
        if (pauseBtn) {
            pauseBtn.addEventListener('click', function () {
                isPaused = !isPaused;

                const pauseIcon = document.getElementById('pause-icon');
                const playIcon = document.getElementById('play-icon');

                if (pauseIcon) pauseIcon.classList.toggle('hidden', isPaused);
                if (playIcon) playIcon.classList.toggle('hidden', !isPaused);

                if (isPaused) {
                    // Requirements: 8.4 - Pause streaming
                    if (eventSource) {
                        eventSource.close();
                    }
                    if (reconnectTimeout) {
                        clearTimeout(reconnectTimeout);
                        reconnectTimeout = null;
                    }
                    updateStreamStatus('paused');
                } else {
                    // Requirements: 8.5 - Resume streaming
                    reconnectAttempts = 0;
                    const streamUrl = buildStreamUrl(appId, deploymentId);
                    connect(streamUrl, logContainer);
                }
            });
        }

        // Copy logs
        if (copyBtn) {
            copyBtn.addEventListener('click', function () {
                const text = logContainer.innerText;
                navigator.clipboard.writeText(text).then(function () {
                    const originalHTML = copyBtn.innerHTML;
                    copyBtn.innerHTML = '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="text-green-500"><polyline points="20 6 9 17 4 12"></polyline></svg>';
                    setTimeout(function () {
                        copyBtn.innerHTML = originalHTML;
                    }, 2000);
                });
            });
        }

        // Clear logs
        if (clearBtn) {
            clearBtn.addEventListener('click', function () {
                logContainer.innerHTML = '';
            });
        }

        // Search/filter logs
        if (searchInput) {
            searchInput.addEventListener('input', function () {
                const searchText = this.value.toLowerCase();
                const lines = logContainer.querySelectorAll('.log-line');
                lines.forEach(function (line) {
                    const content = line.textContent.toLowerCase();
                    if (content.includes(searchText) || searchText === '') {
                        line.classList.remove('hidden');
                    } else {
                        line.classList.add('hidden');
                    }
                });
                logContainer.scrollTop = logContainer.scrollHeight;
            });
        }
    }

    function appendLog(container, log) {
        const line = document.createElement('div');
        line.className = 'flex gap-2 mb-1 log-line';

        const time = new Date(log.timestamp || Date.now()).toLocaleTimeString('en-US', { hour12: false });
        const levelClass = getLevelClass(log.level);

        line.innerHTML = `
            <span class="text-zinc-500 shrink-0 font-mono text-[11px]">${time}</span>
            ${log.level ? `<span class="${levelClass} shrink-0 font-mono text-[11px] uppercase">${log.level}</span>` : ''}
            <span class="text-zinc-400 break-all">${escapeHtml(log.message)}</span>
        `;

        // Apply search filter if active
        const searchInput = document.getElementById('log-search');
        if (searchInput && searchInput.value) {
            const searchText = searchInput.value.toLowerCase();
            if (!log.message.toLowerCase().includes(searchText)) {
                line.classList.add('hidden');
            }
        }

        container.appendChild(line);
        if (!line.classList.contains('hidden')) {
            container.scrollTop = container.scrollHeight;
        }
    }

    function appendPlainLog(container, message) {
        const line = document.createElement('div');
        line.className = 'flex gap-2 mb-1 log-line text-zinc-400 break-all';
        line.textContent = message;
        container.appendChild(line);
        container.scrollTop = container.scrollHeight;
    }

    // Enforce line limit - Requirements: 8.3
    function enforceLineLimit(container) {
        while (container.children.length > MAX_LOG_LINES) {
            container.removeChild(container.firstChild);
        }
    }

    function getLevelClass(level) {
        switch (level) {
            case 'error': return 'text-red-400';
            case 'warn': return 'text-yellow-400';
            case 'info': return 'text-blue-400';
            case 'debug': return 'text-zinc-500';
            default: return 'text-zinc-400';
        }
    }

    function updateStreamStatus(status, delay) {
        const indicator = document.getElementById('stream-status');
        if (!indicator) return;

        switch (status) {
            case 'connected':
                indicator.className = 'text-xs text-green-500 animate-pulse';
                indicator.textContent = '● Live';
                break;
            case 'disconnected':
                indicator.className = 'text-xs text-yellow-500';
                indicator.textContent = '○ Disconnected';
                break;
            case 'reconnecting':
                indicator.className = 'text-xs text-yellow-500';
                indicator.textContent = `○ Reconnecting in ${Math.round(delay / 1000)}s...`;
                break;
            case 'paused':
                indicator.className = 'text-xs text-zinc-500';
                indicator.textContent = '○ Paused';
                break;
            case 'failed':
                indicator.className = 'text-xs text-red-500';
                indicator.textContent = '● Connection failed';
                break;
            default:
                indicator.className = 'text-xs text-zinc-500';
                indicator.textContent = '○ Unknown';
        }
    }

    function updateDeploymentStatus(status) {
        const statusElement = document.querySelector('[data-deployment-status]');
        if (statusElement) {
            statusElement.dataset.deploymentStatus = status;
        }
    }

    function escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    function cleanup() {
        if (eventSource) {
            eventSource.close();
            eventSource = null;
        }
        if (reconnectTimeout) {
            clearTimeout(reconnectTimeout);
            reconnectTimeout = null;
        }
    }

    // Auto-refresh for deployment/build status
    function initAutoRefresh() {
        const statusElement = document.querySelector('[data-auto-refresh]');
        if (!statusElement) return;

        const status = statusElement.dataset.status;
        const interval = parseInt(statusElement.dataset.refreshInterval) || 5000;

        // Only auto-refresh for in-progress statuses
        if (['pending', 'building', 'running', 'queued'].includes(status)) {
            setTimeout(function () {
                window.location.reload();
            }, interval);
        }
    }

    // Expose functions for manual use
    window.NarvanaLogs = {
        initLogStream: initLogStream,
        initAutoRefresh: initAutoRefresh,
        pause: function () {
            isPaused = true;
            if (eventSource) eventSource.close();
            updateStreamStatus('paused');
        },
        resume: function () {
            isPaused = false;
            reconnectAttempts = 0;
            const logContainer = document.getElementById('live-logs');
            if (logContainer) {
                const appId = logContainer.dataset.appId;
                const deploymentId = logContainer.dataset.deploymentId;
                if (appId) {
                    connect(buildStreamUrl(appId, deploymentId), logContainer);
                }
            }
        },
        isPaused: function () {
            return isPaused;
        }
    };
})();
