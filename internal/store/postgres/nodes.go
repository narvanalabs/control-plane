package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/lib/pq"
	"github.com/narvanalabs/control-plane/internal/models"
)

// NodeStore implements store.NodeStore using PostgreSQL.
type NodeStore struct {
	db     *sql.DB
	tx     *sql.Tx
	logger *slog.Logger
}

// conn returns the queryable connection (transaction or database).
func (s *NodeStore) conn() queryable {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

// Register registers a new node or updates an existing one.
func (s *NodeStore) Register(ctx context.Context, node *models.Node) error {
	query := `
		INSERT INTO nodes (id, hostname, address, grpc_port, healthy, 
			cpu_total, cpu_available, memory_total, memory_available,
			disk_total, disk_available, 
			nix_store_total, nix_store_used, nix_store_available, nix_store_usage_percent,
			container_storage_total, container_storage_used, container_storage_available, container_storage_usage_percent,
			cached_paths, last_heartbeat, registered_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
		ON CONFLICT (id) DO UPDATE SET
			hostname = EXCLUDED.hostname,
			address = EXCLUDED.address,
			grpc_port = EXCLUDED.grpc_port,
			healthy = EXCLUDED.healthy,
			cpu_total = EXCLUDED.cpu_total,
			cpu_available = EXCLUDED.cpu_available,
			memory_total = EXCLUDED.memory_total,
			memory_available = EXCLUDED.memory_available,
			disk_total = EXCLUDED.disk_total,
			disk_available = EXCLUDED.disk_available,
			nix_store_total = EXCLUDED.nix_store_total,
			nix_store_used = EXCLUDED.nix_store_used,
			nix_store_available = EXCLUDED.nix_store_available,
			nix_store_usage_percent = EXCLUDED.nix_store_usage_percent,
			container_storage_total = EXCLUDED.container_storage_total,
			container_storage_used = EXCLUDED.container_storage_used,
			container_storage_available = EXCLUDED.container_storage_available,
			container_storage_usage_percent = EXCLUDED.container_storage_usage_percent,
			cached_paths = EXCLUDED.cached_paths,
			last_heartbeat = EXCLUDED.last_heartbeat
		RETURNING id, registered_at`

	now := time.Now().UTC()
	if node.LastHeartbeat.IsZero() {
		node.LastHeartbeat = now
	}
	if node.RegisteredAt.IsZero() {
		node.RegisteredAt = now
	}

	resources := node.Resources
	if resources == nil {
		resources = &models.NodeResources{}
	}

	// Extract disk metrics
	var nixStoreTotal, nixStoreUsed, nixStoreAvailable int64
	var nixStoreUsagePercent float64
	var containerStorageTotal, containerStorageUsed, containerStorageAvailable int64
	var containerStorageUsagePercent float64

	if node.DiskMetrics != nil {
		if node.DiskMetrics.NixStore != nil {
			nixStoreTotal = node.DiskMetrics.NixStore.Total
			nixStoreUsed = node.DiskMetrics.NixStore.Used
			nixStoreAvailable = node.DiskMetrics.NixStore.Available
			nixStoreUsagePercent = node.DiskMetrics.NixStore.UsagePercent
		}
		if node.DiskMetrics.ContainerStorage != nil {
			containerStorageTotal = node.DiskMetrics.ContainerStorage.Total
			containerStorageUsed = node.DiskMetrics.ContainerStorage.Used
			containerStorageAvailable = node.DiskMetrics.ContainerStorage.Available
			containerStorageUsagePercent = node.DiskMetrics.ContainerStorage.UsagePercent
		}
	}

	err := s.conn().QueryRowContext(ctx, query,
		node.ID,
		node.Hostname,
		node.Address,
		node.GRPCPort,
		node.Healthy,
		resources.CPUTotal,
		resources.CPUAvailable,
		resources.MemoryTotal,
		resources.MemoryAvailable,
		resources.DiskTotal,
		resources.DiskAvailable,
		nixStoreTotal,
		nixStoreUsed,
		nixStoreAvailable,
		nixStoreUsagePercent,
		containerStorageTotal,
		containerStorageUsed,
		containerStorageAvailable,
		containerStorageUsagePercent,
		pq.Array(node.CachedPaths),
		node.LastHeartbeat,
		node.RegisteredAt,
	).Scan(&node.ID, &node.RegisteredAt)

	if err != nil {
		return fmt.Errorf("registering node: %w", err)
	}

	return nil
}

// Get retrieves a node by ID.
func (s *NodeStore) Get(ctx context.Context, id string) (*models.Node, error) {
	query := `
		SELECT id, hostname, address, grpc_port, healthy,
			cpu_total, cpu_available, memory_total, memory_available,
			disk_total, disk_available,
			COALESCE(nix_store_total, 0), COALESCE(nix_store_used, 0), 
			COALESCE(nix_store_available, 0), COALESCE(nix_store_usage_percent, 0),
			COALESCE(container_storage_total, 0), COALESCE(container_storage_used, 0),
			COALESCE(container_storage_available, 0), COALESCE(container_storage_usage_percent, 0),
			cached_paths, last_heartbeat, registered_at
		FROM nodes
		WHERE id = $1`

	node := &models.Node{
		Resources:   &models.NodeResources{},
		DiskMetrics: &models.NodeDiskMetrics{},
	}

	var nixStoreTotal, nixStoreUsed, nixStoreAvailable int64
	var nixStoreUsagePercent float64
	var containerStorageTotal, containerStorageUsed, containerStorageAvailable int64
	var containerStorageUsagePercent float64

	err := s.conn().QueryRowContext(ctx, query, id).Scan(
		&node.ID,
		&node.Hostname,
		&node.Address,
		&node.GRPCPort,
		&node.Healthy,
		&node.Resources.CPUTotal,
		&node.Resources.CPUAvailable,
		&node.Resources.MemoryTotal,
		&node.Resources.MemoryAvailable,
		&node.Resources.DiskTotal,
		&node.Resources.DiskAvailable,
		&nixStoreTotal,
		&nixStoreUsed,
		&nixStoreAvailable,
		&nixStoreUsagePercent,
		&containerStorageTotal,
		&containerStorageUsed,
		&containerStorageAvailable,
		&containerStorageUsagePercent,
		pq.Array(&node.CachedPaths),
		&node.LastHeartbeat,
		&node.RegisteredAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying node: %w", err)
	}

	// Populate disk metrics if any values are non-zero
	if nixStoreTotal > 0 || nixStoreUsed > 0 {
		node.DiskMetrics.NixStore = &models.DiskStats{
			Path:         "/nix/store",
			Total:        nixStoreTotal,
			Used:         nixStoreUsed,
			Available:    nixStoreAvailable,
			UsagePercent: nixStoreUsagePercent,
		}
	}
	if containerStorageTotal > 0 || containerStorageUsed > 0 {
		node.DiskMetrics.ContainerStorage = &models.DiskStats{
			Path:         "/var/lib/containers",
			Total:        containerStorageTotal,
			Used:         containerStorageUsed,
			Available:    containerStorageAvailable,
			UsagePercent: containerStorageUsagePercent,
		}
	}

	return node, nil
}

// List retrieves all registered nodes.
func (s *NodeStore) List(ctx context.Context) ([]*models.Node, error) {
	query := `
		SELECT id, hostname, address, grpc_port, healthy,
			cpu_total, cpu_available, memory_total, memory_available,
			disk_total, disk_available,
			COALESCE(nix_store_total, 0), COALESCE(nix_store_used, 0), 
			COALESCE(nix_store_available, 0), COALESCE(nix_store_usage_percent, 0),
			COALESCE(container_storage_total, 0), COALESCE(container_storage_used, 0),
			COALESCE(container_storage_available, 0), COALESCE(container_storage_usage_percent, 0),
			cached_paths, last_heartbeat, registered_at
		FROM nodes
		ORDER BY registered_at DESC`

	rows, err := s.conn().QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying nodes: %w", err)
	}
	defer rows.Close()

	return s.scanNodes(rows)
}

// UpdateHeartbeat updates a node's last heartbeat timestamp and resource metrics.
func (s *NodeStore) UpdateHeartbeat(ctx context.Context, id string, resources *models.NodeResources) error {
	query := `
		UPDATE nodes
		SET last_heartbeat = $2,
			cpu_total = $3, cpu_available = $4,
			memory_total = $5, memory_available = $6,
			disk_total = $7, disk_available = $8,
			healthy = true
		WHERE id = $1`

	now := time.Now().UTC()

	result, err := s.conn().ExecContext(ctx, query,
		id,
		now,
		resources.CPUTotal,
		resources.CPUAvailable,
		resources.MemoryTotal,
		resources.MemoryAvailable,
		resources.DiskTotal,
		resources.DiskAvailable,
	)
	if err != nil {
		return fmt.Errorf("updating heartbeat: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// UpdateHeartbeatWithDiskMetrics updates a node's heartbeat with detailed disk metrics.
// **Validates: Requirements 20.1**
func (s *NodeStore) UpdateHeartbeatWithDiskMetrics(ctx context.Context, id string, resources *models.NodeResources, diskMetrics *models.NodeDiskMetrics) error {
	query := `
		UPDATE nodes
		SET last_heartbeat = $2,
			cpu_total = $3, cpu_available = $4,
			memory_total = $5, memory_available = $6,
			disk_total = $7, disk_available = $8,
			nix_store_total = $9, nix_store_used = $10,
			nix_store_available = $11, nix_store_usage_percent = $12,
			container_storage_total = $13, container_storage_used = $14,
			container_storage_available = $15, container_storage_usage_percent = $16,
			healthy = true
		WHERE id = $1`

	now := time.Now().UTC()

	// Extract disk metrics
	var nixStoreTotal, nixStoreUsed, nixStoreAvailable int64
	var nixStoreUsagePercent float64
	var containerStorageTotal, containerStorageUsed, containerStorageAvailable int64
	var containerStorageUsagePercent float64

	if diskMetrics != nil {
		if diskMetrics.NixStore != nil {
			nixStoreTotal = diskMetrics.NixStore.Total
			nixStoreUsed = diskMetrics.NixStore.Used
			nixStoreAvailable = diskMetrics.NixStore.Available
			nixStoreUsagePercent = diskMetrics.NixStore.UsagePercent
		}
		if diskMetrics.ContainerStorage != nil {
			containerStorageTotal = diskMetrics.ContainerStorage.Total
			containerStorageUsed = diskMetrics.ContainerStorage.Used
			containerStorageAvailable = diskMetrics.ContainerStorage.Available
			containerStorageUsagePercent = diskMetrics.ContainerStorage.UsagePercent
		}
	}

	result, err := s.conn().ExecContext(ctx, query,
		id,
		now,
		resources.CPUTotal,
		resources.CPUAvailable,
		resources.MemoryTotal,
		resources.MemoryAvailable,
		resources.DiskTotal,
		resources.DiskAvailable,
		nixStoreTotal,
		nixStoreUsed,
		nixStoreAvailable,
		nixStoreUsagePercent,
		containerStorageTotal,
		containerStorageUsed,
		containerStorageAvailable,
		containerStorageUsagePercent,
	)
	if err != nil {
		return fmt.Errorf("updating heartbeat with disk metrics: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// UpdateHealth updates a node's health status.
func (s *NodeStore) UpdateHealth(ctx context.Context, id string, healthy bool) error {
	query := `UPDATE nodes SET healthy = $2 WHERE id = $1`

	result, err := s.conn().ExecContext(ctx, query, id, healthy)
	if err != nil {
		return fmt.Errorf("updating health: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// ListHealthy retrieves all healthy nodes.
func (s *NodeStore) ListHealthy(ctx context.Context) ([]*models.Node, error) {
	query := `
		SELECT id, hostname, address, grpc_port, healthy,
			cpu_total, cpu_available, memory_total, memory_available,
			disk_total, disk_available,
			COALESCE(nix_store_total, 0), COALESCE(nix_store_used, 0), 
			COALESCE(nix_store_available, 0), COALESCE(nix_store_usage_percent, 0),
			COALESCE(container_storage_total, 0), COALESCE(container_storage_used, 0),
			COALESCE(container_storage_available, 0), COALESCE(container_storage_usage_percent, 0),
			cached_paths, last_heartbeat, registered_at
		FROM nodes
		WHERE healthy = true
		ORDER BY registered_at DESC`

	rows, err := s.conn().QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying healthy nodes: %w", err)
	}
	defer rows.Close()

	return s.scanNodes(rows)
}

// ListWithClosure retrieves nodes that have a specific store path cached.
func (s *NodeStore) ListWithClosure(ctx context.Context, storePath string) ([]*models.Node, error) {
	query := `
		SELECT id, hostname, address, grpc_port, healthy,
			cpu_total, cpu_available, memory_total, memory_available,
			disk_total, disk_available,
			COALESCE(nix_store_total, 0), COALESCE(nix_store_used, 0), 
			COALESCE(nix_store_available, 0), COALESCE(nix_store_usage_percent, 0),
			COALESCE(container_storage_total, 0), COALESCE(container_storage_used, 0),
			COALESCE(container_storage_available, 0), COALESCE(container_storage_usage_percent, 0),
			cached_paths, last_heartbeat, registered_at
		FROM nodes
		WHERE $1 = ANY(cached_paths)
		ORDER BY registered_at DESC`

	rows, err := s.conn().QueryContext(ctx, query, storePath)
	if err != nil {
		return nil, fmt.Errorf("querying nodes with closure: %w", err)
	}
	defer rows.Close()

	return s.scanNodes(rows)
}

// scanNodes scans multiple node rows.
func (s *NodeStore) scanNodes(rows *sql.Rows) ([]*models.Node, error) {
	var nodes []*models.Node

	for rows.Next() {
		node := &models.Node{
			Resources:   &models.NodeResources{},
			DiskMetrics: &models.NodeDiskMetrics{},
		}

		var nixStoreTotal, nixStoreUsed, nixStoreAvailable int64
		var nixStoreUsagePercent float64
		var containerStorageTotal, containerStorageUsed, containerStorageAvailable int64
		var containerStorageUsagePercent float64

		err := rows.Scan(
			&node.ID,
			&node.Hostname,
			&node.Address,
			&node.GRPCPort,
			&node.Healthy,
			&node.Resources.CPUTotal,
			&node.Resources.CPUAvailable,
			&node.Resources.MemoryTotal,
			&node.Resources.MemoryAvailable,
			&node.Resources.DiskTotal,
			&node.Resources.DiskAvailable,
			&nixStoreTotal,
			&nixStoreUsed,
			&nixStoreAvailable,
			&nixStoreUsagePercent,
			&containerStorageTotal,
			&containerStorageUsed,
			&containerStorageAvailable,
			&containerStorageUsagePercent,
			pq.Array(&node.CachedPaths),
			&node.LastHeartbeat,
			&node.RegisteredAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning node row: %w", err)
		}

		// Populate disk metrics if any values are non-zero
		if nixStoreTotal > 0 || nixStoreUsed > 0 {
			node.DiskMetrics.NixStore = &models.DiskStats{
				Path:         "/nix/store",
				Total:        nixStoreTotal,
				Used:         nixStoreUsed,
				Available:    nixStoreAvailable,
				UsagePercent: nixStoreUsagePercent,
			}
		}
		if containerStorageTotal > 0 || containerStorageUsed > 0 {
			node.DiskMetrics.ContainerStorage = &models.DiskStats{
				Path:         "/var/lib/containers",
				Total:        containerStorageTotal,
				Used:         containerStorageUsed,
				Available:    containerStorageAvailable,
				UsagePercent: containerStorageUsagePercent,
			}
		}

		nodes = append(nodes, node)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating node rows: %w", err)
	}

	return nodes, nil
}
