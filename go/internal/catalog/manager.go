package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"

	_ "github.com/duckdb/duckdb-go/v2"
)

// ManagerConfig holds catalog manager configuration
type ManagerConfig struct {
	CatalogPath    string
	AWSAccessKeyID string
	AWSSecretKey   string
	AWSRegion      string
	AWSEndpoint    string
	Databases      []struct {
		Name     string
		DataPath string
	}
}

// Manager handles DuckDB catalog operations
type Manager struct {
	cfg ManagerConfig
	db  *sql.DB
	mu  sync.Mutex // Serialize all writes
	ctx context.Context

	// Track which databases/tables we've created
	databases map[string]bool
	tables    map[string]bool // key: "database.table"
}

// NewManager creates a new catalog manager
func NewManager(ctx context.Context, cfg ManagerConfig) (*Manager, error) {
	// Open DuckDB connection
	db, err := sql.Open("duckdb", cfg.CatalogPath)
	if err != nil {
		return nil, fmt.Errorf("open duckdb: %w", err)
	}

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping duckdb: %w", err)
	}

	m := &Manager{
		cfg:       cfg,
		db:        db,
		ctx:       ctx,
		databases: make(map[string]bool),
		tables:    make(map[string]bool),
	}

	// Configure S3 credentials if provided
	if cfg.AWSAccessKeyID != "" && cfg.AWSSecretKey != "" {
		if err := m.configureS3(); err != nil {
			db.Close()
			return nil, fmt.Errorf("configure s3: %w", err)
		}
	}

	// Ensure databases exist
	for _, dbCfg := range cfg.Databases {
		if err := m.ensureDatabase(dbCfg.Name, dbCfg.DataPath); err != nil {
			db.Close()
			return nil, fmt.Errorf("ensure database %s: %w", dbCfg.Name, err)
		}
	}

	log.Printf("Catalog manager initialized: %s", cfg.CatalogPath)

	return m, nil
}

// configureS3 sets up S3/B2 credentials in DuckDB
func (m *Manager) configureS3() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Install and load httpfs extension for S3 support
	if _, err := m.db.ExecContext(m.ctx, "INSTALL httpfs;"); err != nil {
		return fmt.Errorf("install httpfs: %w", err)
	}
	if _, err := m.db.ExecContext(m.ctx, "LOAD httpfs;"); err != nil {
		return fmt.Errorf("load httpfs: %w", err)
	}

	// Set S3 credentials
	queries := []string{
		fmt.Sprintf("SET s3_access_key_id='%s';", m.cfg.AWSAccessKeyID),
		fmt.Sprintf("SET s3_secret_access_key='%s';", m.cfg.AWSSecretKey),
		fmt.Sprintf("SET s3_region='%s';", m.cfg.AWSRegion),
		fmt.Sprintf("SET s3_endpoint='%s';", m.cfg.AWSEndpoint),
		"SET s3_url_style='path';",
	}

	for _, q := range queries {
		if _, err := m.db.ExecContext(m.ctx, q); err != nil {
			return fmt.Errorf("configure s3: %w", err)
		}
	}

	log.Println("S3 credentials configured")
	return nil
}

// ensureDatabase creates a database (schema) if it doesn't exist
func (m *Manager) ensureDatabase(name, dataPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.databases[name] {
		return nil // Already created
	}

	query := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s;", name)
	if _, err := m.db.ExecContext(m.ctx, query); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	m.databases[name] = true
	log.Printf("Ensured database (schema): %s (data_path: %s)", name, dataPath)

	return nil
}

// EnsureTable creates a table if it doesn't exist
func (m *Manager) EnsureTable(database, table, schema string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s.%s", database, table)
	if m.tables[key] {
		return nil // Already created
	}

	// TODO: Parse schema and create table
	// For now, this is a placeholder
	// In actual implementation, we'd parse the Arrow schema or SQL DDL

	query := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s.%s (%s);", database, table, schema)
	if _, err := m.db.ExecContext(m.ctx, query); err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	m.tables[key] = true
	log.Printf("Ensured table: %s.%s", database, table)

	return nil
}

// WriteBatch writes a batch of data to a table
func (m *Manager) WriteBatch(database, table string, payload []byte, format string) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// TODO: Implement actual write logic
	// For v1, this would:
	// 1. Parse payload (Arrow IPC / Parquet / JSONL)
	// 2. INSERT INTO database.table SELECT * FROM read_arrow/read_parquet(...)
	// 3. Return row count

	log.Printf("WriteBatch: database=%s, table=%s, format=%s, size=%d bytes",
		database, table, format, len(payload))

	// Placeholder: return 0 rows written
	return 0, nil
}

// GetTableInfo retrieves table metadata
func (m *Manager) GetTableInfo(database, table string) (rowCount, fileCount uint64, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// TODO: Query table stats from DuckDB
	// For now, return zeros

	return 0, 0, nil
}

// CompactTable triggers table optimization
func (m *Manager) CompactTable(database, table string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// TODO: Execute OPTIMIZE TABLE or similar DuckDB operation
	log.Printf("CompactTable: database=%s, table=%s (not implemented)", database, table)

	return nil
}

// Close closes the catalog connection
func (m *Manager) Close() error {
	log.Println("Closing catalog manager...")
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}
