package catalog

import (
	"context"
	"os"
	"testing"
)

func TestManagerBasic(t *testing.T) {
	// Create temporary catalog directory
	tmpDir := t.TempDir()
	catalogPath := tmpDir + "/test.duckdb"

	// Create manager
	cfg := ManagerConfig{
		CatalogPath: catalogPath,
		Databases: []DatabaseConfig{
			{Name: "testnet", DataPath: tmpDir + "/testnet"},
		},
	}

	ctx := context.Background()
	mgr, err := NewManager(ctx, cfg)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.Close()

	// Test table creation
	err = mgr.EnsureTable("testnet", "ledgers_row_v2")
	if err != nil {
		t.Fatalf("EnsureTable failed: %v", err)
	}

	t.Log("✓ Manager initialized and table created successfully")
}

func TestWriteBatchWithParquet(t *testing.T) {
	// Create temporary catalog directory
	tmpDir := t.TempDir()
	catalogPath := tmpDir + "/test.duckdb"

	// Create manager
	cfg := ManagerConfig{
		CatalogPath: catalogPath,
		Databases: []DatabaseConfig{
			{Name: "testnet", DataPath: tmpDir + "/testnet"},
		},
	}

	ctx := context.Background()
	mgr, err := NewManager(ctx, cfg)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.Close()

	// Create a simple test Parquet file using DuckDB
	// This tests the read_parquet integration
	testParquetPath := tmpDir + "/test_data.parquet"

	// Use DuckDB to create test parquet data
	createQuery := `
		COPY (
			SELECT
				1000 AS sequence,
				'abcd1234' AS ledger_hash,
				'prev5678' AS previous_ledger_hash,
				TIMESTAMP '2025-01-01 00:00:00' AS closed_at,
				21 AS protocol_version,
				1000000000::BIGINT AS total_coins,
				100000::BIGINT AS fee_pool,
				100 AS base_fee,
				500000 AS base_reserve,
				1000 AS max_tx_set_size,
				10 AS successful_tx_count,
				2 AS failed_tx_count,
				TIMESTAMP '2025-01-01 00:00:01' AS ingestion_timestamp,
				1000::BIGINT AS ledger_range,
				12 AS transaction_count,
				50 AS operation_count,
				48 AS tx_set_operation_count,
				100::BIGINT AS soroban_fee_write1kb,
				'node123' AS node_id,
				'sig456' AS signature,
				'header_data' AS ledger_header,
				1024::BIGINT AS bucket_list_size,
				2048::BIGINT AS live_soroban_state_size,
				5 AS evicted_keys_count,
				'era1' AS era_id,
				'v1' AS version_label
		) TO '` + testParquetPath + `' (FORMAT PARQUET);
	`

	_, err = mgr.db.ExecContext(ctx, createQuery)
	if err != nil {
		t.Fatalf("Failed to create test parquet: %v", err)
	}

	// Read the parquet file
	payload, err := os.ReadFile(testParquetPath)
	if err != nil {
		t.Fatalf("Failed to read test parquet: %v", err)
	}

	// Write batch
	rowsWritten, err := mgr.WriteBatch("testnet", "ledgers_row_v2", payload, "parquet")
	if err != nil {
		t.Fatalf("WriteBatch failed: %v", err)
	}

	if rowsWritten != 1 {
		t.Errorf("Expected 1 row written, got %d", rowsWritten)
	}

	// Verify data was written by querying it back
	var count int
	err = mgr.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM testnet.ledgers_row_v2").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query data: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 row in table, got %d", count)
	}

	// Verify the actual data
	var sequence int64
	var ledgerHash string
	err = mgr.db.QueryRowContext(ctx,
		"SELECT sequence, ledger_hash FROM testnet.ledgers_row_v2").Scan(&sequence, &ledgerHash)
	if err != nil {
		t.Fatalf("Failed to query row data: %v", err)
	}

	if sequence != 1000 {
		t.Errorf("Expected sequence=1000, got %d", sequence)
	}
	if ledgerHash != "abcd1234" {
		t.Errorf("Expected ledger_hash='abcd1234', got '%s'", ledgerHash)
	}

	t.Logf("✓ Successfully wrote and verified %d rows", rowsWritten)
}
