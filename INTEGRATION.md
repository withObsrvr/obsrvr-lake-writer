# Lake Writer Integration Guide

This guide shows how to integrate lake-writer with data ingestion pipelines.

## Architecture Overview

### Before Lake Writer (Single Writer Constraint)

```
Testnet Ingester → DuckDB Catalog (❌ locked)
Mainnet Ingester → DuckDB Catalog (⏳ waiting)
Futurenet Ingester → DuckDB Catalog (⏳ waiting)
```

**Problem:** Only one process can write to DuckDB catalog at a time.

### After Lake Writer (Multi-Network Support)

```
Testnet Ingester ──┐
                   ├──> Lake Writer (gRPC) ──> DuckDB Catalog (✅ serialized)
Mainnet Ingester ──┤
                   │
Futurenet Ingester─┘
```

**Solution:** Lake Writer accepts concurrent gRPC requests and serializes writes internally.

## Integration Patterns

### Pattern 1: Replace Direct DuckDB Writes

**Before (Direct DuckDB Appender):**
```go
// Create DuckDB appender
appender, err := conn.NewAppenderFromConn(ctx, "", "testnet.ledgers_row_v2")

// Append rows
for _, ledger := range ledgers {
    err = appender.AppendRow(
        ledger.Sequence,
        ledger.Hash,
        // ... more fields
    )
}

// Flush to DuckDB
err = appender.Flush()
```

**After (Lake Writer gRPC):**
```go
import (
    pb "github.com/withObsrvr/obsrvr-lake-writer/gen/lake_writer"
    "google.golang.org/grpc"
)

// Connect to lake-writer
conn, err := grpc.Dial("lake-writer:50099", grpc.WithTransportCredentials(insecure.NewCredentials()))
client := pb.NewLakeWriterClient(conn)

// Create Parquet batch (using Arrow or DuckDB)
parquetData, err := createParquetBatch(ledgers)

// Send batch via gRPC
resp, err := client.WriteBatch(ctx, &pb.WriteBatchRequest{
    Destination: &pb.Destination{
        Database: "testnet",
        Table:    "ledgers_row_v2",
    },
    PipelineId: "testnet-ingester",
    BatchId:    fmt.Sprintf("batch-%d-%d", startSeq, endSeq),
    Format:     pb.DataFormat_DATA_FORMAT_PARQUET,
    Payload:    parquetData,
})

if resp.Status == pb.WriteBatchResponse_STATUS_OK {
    log.Printf("Wrote %d rows", resp.RowsWritten)
}
```

### Pattern 2: Parquet Batch Creation

Use DuckDB to create Parquet batches:

```go
func createParquetBatch(tableName string, rows []map[string]interface{}) ([]byte, error) {
    // Create in-memory DuckDB
    db, err := sql.Open("duckdb", "")
    if err != nil {
        return nil, err
    }
    defer db.Close()

    // Create temp parquet file
    tmpFile := fmt.Sprintf("/tmp/batch_%s_%d.parquet", tableName, time.Now().UnixNano())
    defer os.Remove(tmpFile)

    // Export to parquet
    query := fmt.Sprintf(`
        COPY (
            SELECT %s
            FROM (VALUES %s) AS t(%s)
        ) TO '%s' (FORMAT PARQUET);
    `, columns, values, columns, tmpFile)

    _, err = db.Exec(query)
    if err != nil {
        return nil, err
    }

    // Read parquet file
    return os.ReadFile(tmpFile)
}
```

### Pattern 3: Batch ID Strategy

Use deterministic batch IDs for idempotency:

```go
func generateBatchID(database, table string, startSeq, endSeq uint32) string {
    return fmt.Sprintf("%s-%s-%d-%d", database, table, startSeq, endSeq)
}

// Lake writer will deduplicate based on batch_id
batchID := generateBatchID("testnet", "ledgers_row_v2", 1000, 1100)
```

## Configuration Examples

### Lake Writer Service

```yaml
# config/production.yaml
service:
  listen_address: ":50099"
  health_port: 8088

catalog:
  catalog_path: "./catalogs/obsrvr.duckdb"
  aws_access_key_id: "${AWS_ACCESS_KEY_ID}"
  aws_secret_access_key: "${AWS_SECRET_ACCESS_KEY}"
  aws_region: "us-west-004"
  aws_endpoint: "https://s3.us-west-004.backblazeb2.com"

databases:
  - name: "testnet"
    data_path: "s3://bucket/testnet/"
  - name: "mainnet"
    data_path: "s3://bucket/mainnet/"
  - name: "futurenet"
    data_path: "s3://bucket/futurenet/"

writer:
  queue_size: 10000
  flush_interval_ms: 100
  max_batch_size: 10000
```

### Ingester Configuration

Add lake-writer endpoint to your ingester config:

```yaml
# ducklake-ingestion config
ducklake:
  # Option 1: Use lake-writer (recommended for multi-network)
  use_lake_writer: true
  lake_writer_endpoint: "lake-writer:50099"

  # Option 2: Direct DuckDB (legacy, single writer only)
  # use_lake_writer: false
  # catalog_path: "ducklake:duckdb:./catalogs/testnet.duckdb"

  # Common settings
  data_path: "s3://bucket/testnet/"
  metadata_schema: "testnet"
  batch_size: 100
```

## Deployment Architecture

### Single Server (Development/Testing)

```
┌─────────────────────────────────────────┐
│          Single Server                  │
│                                         │
│  ┌──────────────┐                      │
│  │ Testnet      │                      │
│  │ Ingester     │──┐                   │
│  └──────────────┘  │                   │
│                    │                   │
│  ┌──────────────┐  │  ┌─────────────┐ │
│  │ Mainnet      │──┼─→│ Lake Writer │ │
│  │ Ingester     │  │  └─────────────┘ │
│  └──────────────┘  │         │         │
│                    │         ↓         │
│  ┌──────────────┐  │  ┌─────────────┐ │
│  │ Futurenet    │──┘  │   DuckDB    │ │
│  │ Ingester     │     │   Catalog   │ │
│  └──────────────┘     └─────────────┘ │
│                                         │
└─────────────────────────────────────────┘
```

### Multi-Server (Production)

```
┌────────────────┐
│ Testnet Server │     ┌──────────────────┐
│   Ingester     │────→│                  │
└────────────────┘     │                  │
                       │  Lake Writer     │
┌────────────────┐     │    (gRPC)        │
│ Mainnet Server │────→│                  │
│   Ingester     │     │  localhost:50099 │
└────────────────┘     └────────┬─────────┘
                                │
┌────────────────┐              ↓
│Futurenet Server│      ┌──────────────┐
│   Ingester     │────→ │ DuckDB       │
└────────────────┘      │ Catalog      │
                        │ (single node)│
                        └──────────────┘
```

## Testing the Integration

### 1. Start Lake Writer

```bash
cd obsrvr-lake-writer
./lake-writer -config config/local.yaml
```

### 2. Run Test Client

```bash
./test-client
```

Expected output:
```
✓ Health: status=STATUS_HEALTHY
✓ Write response: status=STATUS_OK, rows_written=1
✓ Final health: total_writes=2, total_errors=0
```

### 3. Verify Data

```bash
duckdb data/catalogs/local.duckdb

D SELECT COUNT(*) FROM testnet.ledgers_row_v2;
┌──────────┐
│    2     │
└──────────┘
```

## Error Handling

### Queue Full

```go
resp, err := client.WriteBatch(ctx, req)
if err != nil {
    if strings.Contains(err.Error(), "queue full") {
        // Backpressure: wait and retry
        time.Sleep(100 * time.Millisecond)
        return client.WriteBatch(ctx, req)
    }
    return err
}
```

### Duplicate Batch

```go
if resp.Status == pb.WriteBatchResponse_STATUS_SKIP {
    log.Printf("Batch %s already processed, skipping", req.BatchId)
    return nil // This is expected for idempotent writes
}
```

### Write Failure

```go
if resp.Status == pb.WriteBatchResponse_STATUS_PERMANENT_ERROR {
    log.Printf("Write failed: %s", resp.Message)
    // Log error, increment metrics, alert ops team
    return fmt.Errorf("permanent write error: %s", resp.Message)
}
```

## Performance Tuning

### Batch Size

```go
// Small batches: lower latency, higher overhead
batchSize := 10  // ledgers per batch

// Large batches: higher throughput, higher latency
batchSize := 1000  // ledgers per batch

// Recommended: Balance based on use case
batchSize := 100  // Good default for most cases
```

### Queue Size

```yaml
writer:
  # Small queue: backpressure faster, less memory
  queue_size: 1000

  # Large queue: absorb bursts, more memory
  queue_size: 10000
```

### Concurrent Clients

```go
// Multiple goroutines can write concurrently
for i := 0; i < numWorkers; i++ {
    go func(workerID int) {
        for batch := range batches {
            resp, err := client.WriteBatch(ctx, batch)
            // Handle response
        }
    }(i)
}
```

## Monitoring

### Health Check

```bash
curl http://localhost:8088/health
```

Response:
```json
{
  "status": "healthy",
  "service": "lake-writer",
  "version": "0.1.0"
}
```

### Metrics (Prometheus)

```bash
curl http://localhost:8088/metrics
```

Key metrics:
- `lake_writer_writes_total{database, table}` - Total writes per table
- `lake_writer_errors_total{database, table}` - Total errors per table
- `lake_writer_queue_depth` - Current queue depth
- `lake_writer_writes_per_second` - Current write rate

### gRPC Health Check

```go
resp, err := client.HealthCheck(ctx, &pb.HealthCheckRequest{})
log.Printf("Status: %s, Queue: %d, Writes: %d",
    resp.Status, resp.QueueDepth, resp.TotalWrites)
```

## Migration Path

### Phase 1: Single Network (Testnet)
1. Deploy lake-writer
2. Modify testnet ingester to use lake-writer
3. Test and validate
4. Monitor for 1 week

### Phase 2: Add Mainnet
1. Deploy mainnet ingester pointing to lake-writer
2. Verify both networks writing concurrently
3. Monitor catalog lock behavior
4. Check queue depth and write rates

### Phase 3: Add Futurenet
1. Deploy futurenet ingester pointing to lake-writer
2. Verify all 3 networks writing concurrently
3. Load test with realistic traffic
4. Tune queue sizes and batch sizes

### Phase 4: Production
1. Deploy to production with proper monitoring
2. Set up alerting for errors and queue depth
3. Document runbooks for common issues
4. Plan for scaling (multiple lake-writer instances with PostgreSQL catalog)

## Troubleshooting

### Lake Writer Not Responding

```bash
# Check if service is running
ps aux | grep lake-writer

# Check health endpoint
curl http://localhost:8088/health

# Check gRPC port
netstat -tlnp | grep 50099
```

### High Queue Depth

```bash
# Check metrics
curl http://localhost:8088/metrics | grep queue_depth

# Possible causes:
# 1. DuckDB writes are slow (check catalog file size)
# 2. Too many concurrent clients
# 3. Network issues

# Solutions:
# 1. Increase queue_size in config
# 2. Reduce batch frequency from clients
# 3. Scale to multiple lake-writer instances
```

### Write Errors

```bash
# Check lake-writer logs
tail -f /var/log/lake-writer.log

# Common errors:
# - "table not found" → Schema mismatch
# - "catalog locked" → Multiple writers (bug)
# - "invalid parquet" → Corrupt batch data
```

## Next Steps

1. Review the test client code: `go/cmd/test-client/main.go`
2. Modify your ingester to use the integration pattern
3. Test locally with multiple ingesters
4. Deploy to staging environment
5. Monitor and tune performance
6. Deploy to production

## Support

- Issues: https://github.com/withObsrvr/obsrvr-lake-writer/issues
- Documentation: See README.md and PITCH.md
- Examples: See `go/cmd/test-client/` for working examples
