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
			disk_total, disk_available, cached_paths, last_heartbeat, registered_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
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
			disk_total, disk_available, cached_paths, last_heartbeat, registered_at
		FROM nodes
		WHERE id = $1`

	node := &models.Node{Resources: &models.NodeResources{}}

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

	return node, nil
}

// List retrieves all registered nodes.
func (s *NodeStore) List(ctx context.Context) ([]*models.Node, error) {
	query := `
		SELECT id, hostname, address, grpc_port, healthy,
			cpu_total, cpu_available, memory_total, memory_available,
			disk_total, disk_available, cached_paths, last_heartbeat, registered_at
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
			disk_total, disk_available, cached_paths, last_heartbeat, registered_at
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
			disk_total, disk_available, cached_paths, last_heartbeat, registered_at
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
		node := &models.Node{Resources: &models.NodeResources{}}

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
			pq.Array(&node.CachedPaths),
			&node.LastHeartbeat,
			&node.RegisteredAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning node row: %w", err)
		}

		nodes = append(nodes, node)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating node rows: %w", err)
	}

	return nodes, nil
}
