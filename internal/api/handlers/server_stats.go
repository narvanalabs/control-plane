package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/narvanalabs/control-plane/internal/models"
)

type ServerStatsHandler struct {
	logger *slog.Logger
}

func NewServerStatsHandler(logger *slog.Logger) *ServerStatsHandler {
	return &ServerStatsHandler{logger: logger}
}

type SystemStats struct {
	Resources *models.NodeResources `json:"resources"`
	Uptime    float64               `json:"uptime"`
	OS        string                `json:"os"`
	Hostname  string                `json:"hostname"`
	Timestamp int64                 `json:"timestamp"`
}

func (h *ServerStatsHandler) Get(w http.ResponseWriter, r *http.Request) {
	stats := h.collectStats()
	WriteJSON(w, http.StatusOK, stats)
}

func (h *ServerStatsHandler) Stream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			stats := h.collectStats()
			jsonData, _ := json.Marshal(stats)
			fmt.Fprintf(w, "data: %s\n\n", jsonData)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}
}

func (h *ServerStatsHandler) collectStats() SystemStats {
	resources := &models.NodeResources{}

	// Memory
	if mem, err := readMemInfo(); err == nil {
		resources.MemoryTotal = mem.Total
		resources.MemoryAvailable = mem.Available
	}

	// CPU
	if cpuUsage, err := h.getCPUUsage(); err == nil {
		resources.CPUTotal = 100
		resources.CPUAvailable = 100 - cpuUsage
	}

	// Disk
	if disk, err := h.getDiskUsage("/"); err == nil {
		resources.DiskTotal = disk.Total
		resources.DiskAvailable = disk.Available
	}

	// Uptime
	uptime := float64(0)
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		parts := strings.Fields(string(data))
		if len(parts) > 0 {
			uptime, _ = strconv.ParseFloat(parts[0], 64)
		}
	}

	hostname, _ := os.Hostname()

	return SystemStats{
		Resources: resources,
		Uptime:    uptime,
		OS:        "Linux",
		Hostname:  hostname,
		Timestamp: time.Now().Unix(),
	}
}

type memInfo struct {
	Total     int64
	Available int64
}

func readMemInfo() (memInfo, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return memInfo{}, err
	}
	defer file.Close()

	var res memInfo
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		val, _ := strconv.ParseInt(parts[1], 10, 64)
		key := strings.TrimSuffix(parts[0], ":")

		switch key {
		case "MemTotal":
			res.Total = val * 1024
		case "MemAvailable":
			res.Available = val * 1024
		}
	}
	return res, nil
}

func (h *ServerStatsHandler) getCPUUsage() (float64, error) {
	readCPUStats := func() (idle, total uint64) {
		data, err := os.ReadFile("/proc/stat")
		if err != nil {
			return 0, 0
		}
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "cpu ") {
				fields := strings.Fields(line)
				for i := 1; i < len(fields); i++ {
					val, _ := strconv.ParseUint(fields[i], 10, 64)
					total += val
					if i == 4 { // idle field
						idle = val
					}
				}
				return
			}
		}
		return
	}

	idle1, total1 := readCPUStats()
	time.Sleep(100 * time.Millisecond)
	idle2, total2 := readCPUStats()

	if total2 == total1 {
		return 0, nil
	}

	idleTicks := float64(idle2 - idle1)
	totalTicks := float64(total2 - total1)
	usage := 100 * (1 - idleTicks/totalTicks)
	return usage, nil
}

type diskUsage struct {
	Total     int64
	Available int64
}

func (h *ServerStatsHandler) getDiskUsage(path string) (diskUsage, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return diskUsage{}, err
	}

	return diskUsage{
		Total:     int64(stat.Blocks) * int64(stat.Bsize),
		Available: int64(stat.Bavail) * int64(stat.Bsize),
	}, nil
}
