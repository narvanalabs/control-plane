// Live Log Streaming for Narvana Control Plane
// Uses Server-Sent Events (SSE) to stream logs in real-time

(function () {
    'use strict';

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

        const streamUrl = `/api/logs/stream?app_id=${appId}${deploymentId ? `&deployment_id=${deploymentId}` : ''}`;

        let eventSource = null;
        let reconnectAttempts = 0;
        const maxReconnectAttempts = 5;

        function connect() {
            eventSource = new EventSource(streamUrl);

            eventSource.onopen = function () {
                console.log('Log stream connected');
                reconnectAttempts = 0;
                updateStreamStatus('connected');
            };

            eventSource.onmessage = function (event) {
                try {
                    const log = JSON.parse(event.data);
                    appendLog(logContainer, log);
                } catch (e) {
                    // Plain text log
                    appendPlainLog(logContainer, event.data);
                }
            };

            eventSource.onerror = function () {
                console.log('Log stream disconnected');
                updateStreamStatus('disconnected');
                eventSource.close();

                // Reconnect with exponential backoff
                if (reconnectAttempts < maxReconnectAttempts) {
                    reconnectAttempts++;
                    const delay = Math.min(1000 * Math.pow(2, reconnectAttempts), 30000);
                    setTimeout(connect, delay);
                }
            };
        }

        connect();

        // Cleanup on page unload
        window.addEventListener('beforeunload', function () {
            if (eventSource) {
                eventSource.close();
            }
        });
    }

    function appendLog(container, log) {
        const line = document.createElement('div');
        line.className = 'flex gap-2';

        const time = new Date(log.timestamp).toLocaleTimeString('en-US', { hour12: false });
        line.innerHTML = `
      <span class="text-zinc-500">${time}</span>
      <span class="${getLevelClass(log.level)}">${log.level}</span>
      <span>${escapeHtml(log.message)}</span>
    `;

        container.appendChild(line);
        container.scrollTop = container.scrollHeight;
    }

    function appendPlainLog(container, message) {
        const line = document.createElement('div');
        line.textContent = message;
        container.appendChild(line);
        container.scrollTop = container.scrollHeight;
    }

    function getLevelClass(level) {
        switch (level) {
            case 'error': return 'text-red-400';
            case 'warn': return 'text-yellow-400';
            case 'info': return 'text-blue-400';
            default: return 'text-zinc-400';
        }
    }

    function updateStreamStatus(status) {
        const indicator = document.getElementById('stream-status');
        if (!indicator) return;

        if (status === 'connected') {
            indicator.className = 'animate-pulse text-green-500';
            indicator.textContent = '● Live';
        } else {
            indicator.className = 'text-yellow-500';
            indicator.textContent = '○ Reconnecting...';
        }
    }

    function escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
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
        initAutoRefresh: initAutoRefresh
    };
})();
