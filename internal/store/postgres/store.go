// Package postgres provides PostgreSQL implementation of the store interfaces.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/narvanalabs/control-plane/internal/store"
)

// PostgresStore implements the Store interface using PostgreSQL.
	github      *GitHubStore
	githubAccounts *GitHubAccountStore
	settings    *SettingsStore
}

// Config holds PostgreSQL connection configuration.
type Config struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig(dsn string) *Config {
	return &Config{
		DSN:             dsn,
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 1 * time.Minute,
	}
}

// NewPostgresStore creates a new PostgreSQL store with the given configuration.
func NewPostgresStore(cfg *Config, logger *slog.Logger) (*PostgresStore, error) {
	if logger == nil {
		logger = slog.Default()
	}

	db, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	s := &PostgresStore{
		db:     db,
		logger: logger,
	}

	// Initialize sub-stores
	s.apps = &AppStore{db: db, logger: logger}
	s.deployments = &DeploymentStore{db: db, logger: logger}
	s.nodes = &NodeStore{db: db, logger: logger}
	s.builds = &BuildStore{db: db, logger: logger}
	s.secrets = &SecretStore{db: db, logger: logger}
	s.logs = &LogStore{db: db, logger: logger}
	s.users = &UserStore{db: db, logger: logger}
	s.github = &GitHubStore{db: db, logger: logger}
	s.githubAccounts = &GitHubAccountStore{db: db, logger: logger}
	s.settings = &SettingsStore{db: db, logger: logger}

	logger.Info("connected to PostgreSQL database")
	return s, nil
}


// Apps returns the AppStore.
func (s *PostgresStore) Apps() store.AppStore {
	return s.apps
}

// Deployments returns the DeploymentStore.
func (s *PostgresStore) Deployments() store.DeploymentStore {
	return s.deployments
}

// Nodes returns the NodeStore.
func (s *PostgresStore) Nodes() store.NodeStore {
	return s.nodes
}

// Builds returns the BuildStore.
func (s *PostgresStore) Builds() store.BuildStore {
	return s.builds
}

// Secrets returns the SecretStore.
func (s *PostgresStore) Secrets() store.SecretStore {
	return s.secrets
}

// Logs returns the LogStore.
func (s *PostgresStore) Logs() store.LogStore {
	return s.logs
}

// Users returns the UserStore.
func (s *PostgresStore) Users() store.UserStore {
	return s.users
}

// GitHub returns the GitHubStore.
func (s *PostgresStore) GitHub() store.GitHubStore {
	return s.github
}

// GitHubAccounts returns the GitHubAccountStore.
func (s *PostgresStore) GitHubAccounts() store.GitHubAccountStore {
	return s.githubAccounts
}

// Settings returns the SettingsStore.
func (s *PostgresStore) Settings() store.SettingsStore {
	return s.settings
}

// WithTx executes the given function within a database transaction.
func (s *PostgresStore) WithTx(ctx context.Context, fn func(store.Store) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	// Create a transaction-scoped store
	txStore := &txStore{
		tx:     tx,
		logger: s.logger,
	}

	// Execute the function
	if err := fn(txStore); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			s.logger.Error("failed to rollback transaction", "error", rbErr)
		}
		return err
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// Close closes the database connection.
func (s *PostgresStore) Close() error {
	s.logger.Info("closing PostgreSQL connection")
	return s.db.Close()
}

// DB returns the underlying database connection.
// This is useful for components that need direct database access.
func (s *PostgresStore) DB() *sql.DB {
	return s.db
}

// txStore wraps a transaction and implements the Store interface.
	github      *GitHubStore
	githubAccounts *GitHubAccountStore
	settings    *SettingsStore
}

func (s *txStore) Apps() store.AppStore {
	if s.apps == nil {
		s.apps = &AppStore{tx: s.tx, logger: s.logger}
	}
	return s.apps
}

func (s *txStore) Deployments() store.DeploymentStore {
	if s.deployments == nil {
		s.deployments = &DeploymentStore{tx: s.tx, logger: s.logger}
	}
	return s.deployments
}

func (s *txStore) Nodes() store.NodeStore {
	if s.nodes == nil {
		s.nodes = &NodeStore{tx: s.tx, logger: s.logger}
	}
	return s.nodes
}

func (s *txStore) Builds() store.BuildStore {
	if s.builds == nil {
		s.builds = &BuildStore{tx: s.tx, logger: s.logger}
	}
	return s.builds
}

func (s *txStore) Secrets() store.SecretStore {
	if s.secrets == nil {
		s.secrets = &SecretStore{tx: s.tx, logger: s.logger}
	}
	return s.secrets
}

func (s *txStore) Logs() store.LogStore {
	if s.logs == nil {
		s.logs = &LogStore{tx: s.tx, logger: s.logger}
	}
	return s.logs
}

func (s *txStore) Users() store.UserStore {
	if s.users == nil {
		s.users = &UserStore{tx: s.tx, logger: s.logger}
	}
	return s.users
}

func (s *txStore) GitHub() store.GitHubStore {
	if s.github == nil {
		s.github = &GitHubStore{tx: s.tx, logger: s.logger}
	}
	return s.github
}

func (s *txStore) GitHubAccounts() store.GitHubAccountStore {
	if s.githubAccounts == nil {
		s.githubAccounts = &GitHubAccountStore{tx: s.tx, logger: s.logger}
	}
	return s.githubAccounts
}

func (s *txStore) Settings() store.SettingsStore {
	if s.settings == nil {
		s.settings = &SettingsStore{db: s.tx, logger: s.logger}
	}
	return s.settings
}

func (s *txStore) WithTx(ctx context.Context, fn func(store.Store) error) error {
	// Already in a transaction, just execute the function
	return fn(s)
}

func (s *txStore) Close() error {
	// No-op for transaction store
	return nil
}

// queryable is an interface that both *sql.DB and *sql.Tx implement.
type queryable interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
