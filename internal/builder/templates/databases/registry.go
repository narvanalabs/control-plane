// Package databases provides database flake template registry and configuration.
package databases

import (
	cryptoRand "crypto/rand"
	"fmt"

	"github.com/narvanalabs/control-plane/internal/models"
)

// DatabaseType represents a supported database type.
type DatabaseType string

const (
	DatabaseTypePostgres DatabaseType = "postgres"
	DatabaseTypeMySQL    DatabaseType = "mysql"
	DatabaseTypeMariaDB  DatabaseType = "mariadb"
	DatabaseTypeMongoDB  DatabaseType = "mongodb"
	DatabaseTypeRedis    DatabaseType = "redis"
)

// ValidDatabaseTypes returns all valid database types.
func ValidDatabaseTypes() []DatabaseType {
	return []DatabaseType{
		DatabaseTypePostgres,
		DatabaseTypeMySQL,
		DatabaseTypeMariaDB,
		DatabaseTypeMongoDB,
		DatabaseTypeRedis,
	}
}

// IsValid checks if the database type is valid.
func (t DatabaseType) IsValid() bool {
	for _, valid := range ValidDatabaseTypes() {
		if t == valid {
			return true
		}
	}
	return false
}

// ConfigOption defines a configuration option for a database template.
type ConfigOption struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // string, int, bool
	Default     string `json:"default"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// DatabaseTemplate defines a database flake template configuration.
type DatabaseTemplate struct {
	Type              DatabaseType   `json:"type"`
	DisplayName       string         `json:"display_name"`
	Description       string         `json:"description"`
	DefaultVersion    string         `json:"default_version"`
	AvailableVersions []string       `json:"available_versions"`
	TemplateName      string         `json:"template_name"` // Name of the .nix.tmpl file
	ConfigOptions     []ConfigOption `json:"config_options"`
	DefaultPort       int            `json:"default_port"`
}

// Registry contains all database templates indexed by type.
var Registry = map[DatabaseType]DatabaseTemplate{
	DatabaseTypePostgres: {
		Type:              DatabaseTypePostgres,
		DisplayName:       "PostgreSQL",
		Description:       "Advanced open-source relational database",
		DefaultVersion:    "16",
		AvailableVersions: []string{"14", "15", "16", "17"},
		TemplateName:      "postgres.nix",
		DefaultPort:       5432,
		ConfigOptions: []ConfigOption{
			{Name: "storage_size", Type: "string", Default: "10Gi", Description: "Storage size for database data"},
			{Name: "max_connections", Type: "int", Default: "100", Description: "Maximum number of connections"},
			{Name: "shared_buffers", Type: "string", Default: "128MB", Description: "Shared buffer memory"},
			{Name: "backup_schedule", Type: "string", Default: "0 2 * * *", Description: "Cron schedule for backups"},
		},
	},
	DatabaseTypeMySQL: {
		Type:              DatabaseTypeMySQL,
		DisplayName:       "MySQL",
		Description:       "Popular open-source relational database",
		DefaultVersion:    "8.0",
		AvailableVersions: []string{"5.7", "8.0", "8.4"},
		TemplateName:      "mysql.nix",
		DefaultPort:       3306,
		ConfigOptions: []ConfigOption{
			{Name: "storage_size", Type: "string", Default: "10Gi", Description: "Storage size for database data"},
			{Name: "innodb_buffer_pool_size", Type: "string", Default: "128M", Description: "InnoDB buffer pool size"},
			{Name: "max_connections", Type: "int", Default: "151", Description: "Maximum number of connections"},
			{Name: "backup_schedule", Type: "string", Default: "0 2 * * *", Description: "Cron schedule for backups"},
		},
	},
	DatabaseTypeMariaDB: {
		Type:              DatabaseTypeMariaDB,
		DisplayName:       "MariaDB",
		Description:       "Community-developed fork of MySQL",
		DefaultVersion:    "11",
		AvailableVersions: []string{"10.6", "10.11", "11"},
		TemplateName:      "mariadb.nix",
		DefaultPort:       3306,
		ConfigOptions: []ConfigOption{
			{Name: "storage_size", Type: "string", Default: "10Gi", Description: "Storage size for database data"},
			{Name: "innodb_buffer_pool_size", Type: "string", Default: "128M", Description: "InnoDB buffer pool size"},
			{Name: "max_connections", Type: "int", Default: "151", Description: "Maximum number of connections"},
			{Name: "backup_schedule", Type: "string", Default: "0 2 * * *", Description: "Cron schedule for backups"},
		},
	},
	DatabaseTypeMongoDB: {
		Type:              DatabaseTypeMongoDB,
		DisplayName:       "MongoDB",
		Description:       "Document-oriented NoSQL database",
		DefaultVersion:    "7.0",
		AvailableVersions: []string{"6.0", "7.0"},
		TemplateName:      "mongodb.nix",
		DefaultPort:       27017,
		ConfigOptions: []ConfigOption{
			{Name: "storage_size", Type: "string", Default: "10Gi", Description: "Storage size for database data"},
			{Name: "wired_tiger_cache_size", Type: "string", Default: "256M", Description: "WiredTiger cache size"},
			{Name: "backup_schedule", Type: "string", Default: "0 2 * * *", Description: "Cron schedule for backups"},
		},
	},
	DatabaseTypeRedis: {
		Type:              DatabaseTypeRedis,
		DisplayName:       "Redis",
		Description:       "In-memory data structure store",
		DefaultVersion:    "7",
		AvailableVersions: []string{"6", "7"},
		TemplateName:      "redis.nix",
		DefaultPort:       6379,
		ConfigOptions: []ConfigOption{
			{Name: "maxmemory", Type: "string", Default: "256mb", Description: "Maximum memory limit"},
			{Name: "maxmemory_policy", Type: "string", Default: "allkeys-lru", Description: "Eviction policy when maxmemory is reached"},
			{Name: "appendonly", Type: "bool", Default: "yes", Description: "Enable append-only file persistence"},
			{Name: "backup_schedule", Type: "string", Default: "0 2 * * *", Description: "Cron schedule for backups"},
		},
	},
}

// GetTemplate returns the database template for a given type.
func GetTemplate(dbType DatabaseType) (*DatabaseTemplate, error) {
	template, ok := Registry[dbType]
	if !ok {
		return nil, fmt.Errorf("unknown database type: %s", dbType)
	}
	return &template, nil
}

// GetTemplateByString returns the database template for a given type string.
func GetTemplateByString(dbType string) (*DatabaseTemplate, error) {
	return GetTemplate(DatabaseType(dbType))
}

// GetDefaultVersion returns the default version for a database type.
func GetDefaultVersion(dbType DatabaseType) string {
	template, err := GetTemplate(dbType)
	if err != nil {
		return ""
	}
	return template.DefaultVersion
}

// IsValidVersion checks if a version is valid for a database type.
func IsValidVersion(dbType DatabaseType, version string) bool {
	template, err := GetTemplate(dbType)
	if err != nil {
		return false
	}
	for _, v := range template.AvailableVersions {
		if v == version {
			return true
		}
	}
	return false
}

// GetTemplateName returns the Nix template name for a database type.
func GetTemplateName(dbType DatabaseType) string {
	template, err := GetTemplate(dbType)
	if err != nil {
		return ""
	}
	return template.TemplateName
}

// GetBuildType returns the build type for database services.
// Database services always use pure-nix build type, never OCI.
// **Validates: Requirements 11.2**
func GetBuildType() models.BuildType {
	return models.BuildTypePureNix
}

// GetDefaultPort returns the default port for a database type.
func GetDefaultPort(dbType DatabaseType) int {
	template, err := GetTemplate(dbType)
	if err != nil {
		return 0
	}
	return template.DefaultPort
}

// GetConfigOptions returns the configuration options for a database type.
func GetConfigOptions(dbType DatabaseType) []ConfigOption {
	template, err := GetTemplate(dbType)
	if err != nil {
		return nil
	}
	return template.ConfigOptions
}

// GetDefaultConfig returns a map of default configuration values for a database type.
func GetDefaultConfig(dbType DatabaseType) map[string]string {
	options := GetConfigOptions(dbType)
	config := make(map[string]string)
	for _, opt := range options {
		config[opt.Name] = opt.Default
	}
	return config
}

// DatabaseTemplateData contains data for rendering database templates.
type DatabaseTemplateData struct {
	AppName         string
	ServiceName     string
	DatabaseType    DatabaseType
	DatabaseVersion string
	System          string
	Config          map[string]string
}

// NewDatabaseTemplateData creates a new DatabaseTemplateData with defaults.
func NewDatabaseTemplateData(appName, serviceName string, dbType DatabaseType, version string) *DatabaseTemplateData {
	if version == "" {
		version = GetDefaultVersion(dbType)
	}
	return &DatabaseTemplateData{
		AppName:         appName,
		ServiceName:     serviceName,
		DatabaseType:    dbType,
		DatabaseVersion: version,
		System:          models.GetCurrentSystem(),
		Config:          GetDefaultConfig(dbType),
	}
}

// WithConfig sets custom configuration values.
func (d *DatabaseTemplateData) WithConfig(config map[string]string) *DatabaseTemplateData {
	for k, v := range config {
		d.Config[k] = v
	}
	return d
}

// DatabaseCredentials contains generated credentials for a database service.
type DatabaseCredentials struct {
	Username     string `json:"username"`
	Password     string `json:"password"`
	DatabaseName string `json:"database_name"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	RootPassword string `json:"root_password,omitempty"` // For MySQL/MariaDB/MongoDB
}

// GenerateCredentials generates connection credentials for a database service.
// **Validates: Requirements 11.9**
func GenerateCredentials(dbType DatabaseType, serviceName string) (*DatabaseCredentials, error) {
	template, err := GetTemplate(dbType)
	if err != nil {
		return nil, err
	}

	// Generate secure random password
	password, err := generateSecurePassword(24)
	if err != nil {
		return nil, fmt.Errorf("generating password: %w", err)
	}

	// Generate root password for databases that need it
	var rootPassword string
	if dbType == DatabaseTypeMySQL || dbType == DatabaseTypeMariaDB || dbType == DatabaseTypeMongoDB {
		rootPassword, err = generateSecurePassword(32)
		if err != nil {
			return nil, fmt.Errorf("generating root password: %w", err)
		}
	}

	// Use service name as database name (sanitized)
	dbName := sanitizeDatabaseName(serviceName)
	username := sanitizeDatabaseName(serviceName)

	return &DatabaseCredentials{
		Username:     username,
		Password:     password,
		DatabaseName: dbName,
		Host:         "localhost",
		Port:         template.DefaultPort,
		RootPassword: rootPassword,
	}, nil
}

// GetConnectionURL returns the connection URL for a database.
func (c *DatabaseCredentials) GetConnectionURL(dbType DatabaseType) string {
	switch dbType {
	case DatabaseTypePostgres:
		return fmt.Sprintf("postgresql://%s:%s@%s:%d/%s",
			c.Username, c.Password, c.Host, c.Port, c.DatabaseName)
	case DatabaseTypeMySQL, DatabaseTypeMariaDB:
		return fmt.Sprintf("mysql://%s:%s@%s:%d/%s",
			c.Username, c.Password, c.Host, c.Port, c.DatabaseName)
	case DatabaseTypeMongoDB:
		return fmt.Sprintf("mongodb://%s:%s@%s:%d/%s",
			c.Username, c.Password, c.Host, c.Port, c.DatabaseName)
	case DatabaseTypeRedis:
		if c.Password != "" {
			return fmt.Sprintf("redis://:%s@%s:%d", c.Password, c.Host, c.Port)
		}
		return fmt.Sprintf("redis://%s:%d", c.Host, c.Port)
	default:
		return ""
	}
}

// GetSecretKeys returns the secret keys that should be stored for this database.
func (c *DatabaseCredentials) GetSecretKeys(dbType DatabaseType, serviceName string) map[string]string {
	prefix := fmt.Sprintf("%s_", sanitizeEnvVarName(serviceName))
	secrets := map[string]string{
		prefix + "DATABASE_URL": c.GetConnectionURL(dbType),
		prefix + "DB_HOST":      c.Host,
		prefix + "DB_PORT":      fmt.Sprintf("%d", c.Port),
		prefix + "DB_NAME":      c.DatabaseName,
		prefix + "DB_USER":      c.Username,
		prefix + "DB_PASSWORD":  c.Password,
	}

	// Add root password for databases that have it
	if c.RootPassword != "" {
		secrets[prefix+"DB_ROOT_PASSWORD"] = c.RootPassword
	}

	return secrets
}

// generateSecurePassword generates a cryptographically secure random password.
func generateSecurePassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)

	// Use crypto/rand for secure random generation
	_, err := cryptoRand.Read(b)
	if err != nil {
		return "", err
	}

	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b), nil
}

// sanitizeDatabaseName sanitizes a service name for use as a database name.
func sanitizeDatabaseName(name string) string {
	// Replace hyphens with underscores, remove other special characters
	result := make([]byte, 0, len(name))
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			result = append(result, byte(c))
		} else if c == '-' {
			result = append(result, '_')
		}
	}
	// Ensure it starts with a letter
	if len(result) > 0 && result[0] >= '0' && result[0] <= '9' {
		result = append([]byte{'d', 'b', '_'}, result...)
	}
	if len(result) == 0 {
		return "narvana"
	}
	return string(result)
}

// sanitizeEnvVarName sanitizes a service name for use as an environment variable prefix.
func sanitizeEnvVarName(name string) string {
	result := make([]byte, 0, len(name))
	for _, c := range name {
		if c >= 'a' && c <= 'z' {
			result = append(result, byte(c-'a'+'A')) // Convert to uppercase
		} else if (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			result = append(result, byte(c))
		} else if c == '-' {
			result = append(result, '_')
		}
	}
	if len(result) == 0 {
		return "DB"
	}
	return string(result)
}
