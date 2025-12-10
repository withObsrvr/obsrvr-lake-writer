# Lake Writer Service – Shape Up Pitch

**Cycle:** 1 week (5 working days)
**Team:** Solo development
**Created:** 2025-12-10

---

## Problem

We have three network ingestion processes (testnet, mainnet, futurenet) that all need to write to a single DuckLake catalog. The DuckDB catalog server only supports **single-writer** access, but we need multiple concurrent ingestion pipelines.

**Current blocker:**
- `ducklake-ingestion-obsrvr-v3` tries to write directly to DuckDB catalog
- Running 3 instances = 3 writers = catalog lock conflicts ❌
- Can't use PostgreSQL catalog because we need **data inlining** support

**What we need:**
A dedicated `lake-writer` service that serializes all writes to the DuckDB catalog, allowing many pipelines to write logically while maintaining single-writer constraint physically.

---

## Appetite

**1 week** (fixed time, variable scope)

This is a **must-have** blocker for production deployment of multi-network ingestion.

### Time Budget Breakdown

- **Day 1-2:** Core gRPC API + single-table writes
- **Day 3:** Multi-table batch writes + error handling
- **Day 4:** Basic compaction triggers + observability
- **Day 5:** Integration testing + deployment setup

---

## Solution

### Fat-Marker Sketch

```
┌─────────────────┐
│ testnet-ingest  │────┐
└─────────────────┘    │
                       │
┌─────────────────┐    │     ┌──────────────────┐
│ mainnet-ingest  │────┼────►│  lake-writer     │
└─────────────────┘    │     │  (gRPC server)   │
                       │     │                  │
┌─────────────────┐    │     │ - WriteBatch     │
│futurenet-ingest │────┘     │ - RegisterTable  │
└─────────────────┘          │ - CompactTable   │
                             │ - GetTableInfo   │
                             └────────┬─────────┘
                                      │
                                      ▼
                             ┌────────────────────┐
                             │ DuckDB Catalog     │
                             │ Server (single     │
                             │ writer)            │
                             └────────┬───────────┘
                                      │
                                      ▼
                             ┌────────────────────┐
                             │ DuckLake (S3/B2)   │
                             │ - testnet/         │
                             │ - mainnet/         │
                             │ - futurenet/       │
                             └────────────────────┘
```

### Core Components

**1. gRPC Server (Go)**
- Listen on port 50099
- Handle concurrent requests (queue internally)
- Single DuckDB connection for writes
- Multiple goroutines for request handling → funnel to single writer

**2. Proto API (4 RPCs)**
```proto
service LakeWriter {
  rpc WriteBatch(WriteBatchRequest) returns (WriteBatchResponse);
  rpc RegisterTable(RegisterTableRequest) returns (RegisterTableResponse);
  rpc CompactTable(CompactTableRequest) returns (CompactTableResponse);
  rpc GetTableInfo(GetTableInfoRequest) returns (GetTableInfoResponse);
}
```

**3. Write Queue**
- Buffered channel for write requests
- Single writer goroutine drains queue
- Backpressure when queue is full
- Idempotent writes (batch_id deduplication)

**4. DuckDB Integration**
- Uses official `github.com/duckdb/duckdb-go/v2`
- Connects to DuckDB catalog server via connection string
- Handles Arrow IPC, Parquet, JSONL formats
- Auto-creates tables based on schema descriptors

---

## Scope Line

### Must Have ✅ (Non-negotiable)

1. **WriteBatch RPC**
   - Accept Arrow IPC batches
   - Route to correct database/table (testnet/mainnet/futurenet)
   - Serialize writes through single DuckDB connection
   - Return success/failure + row count

2. **Table Management**
   - Create tables if missing (CREATE TABLE IF NOT EXISTS)
   - Support 19 table types from `ducklake-ingestion-obsrvr-v3`
   - Schema evolution (additive only for v1)

3. **Multi-Network Support**
   - Three databases: testnet, mainnet, futurenet
   - Each with 19 tables (ledgers_row_v2, transactions_row_v2, etc.)
   - Isolated data paths in S3

4. **Error Handling**
   - Retry on transient errors
   - Return clear error messages to clients
   - Log all write operations

5. **Basic Observability**
   - Metrics: writes/sec, queue depth, errors
   - Health endpoint (port 8088)
   - Prometheus metrics

### Nice to Have 🎯 (Include if time permits)

1. **RegisterTable RPC**
   - Pre-create tables with explicit schemas
   - Useful for control plane integration

2. **Batch Deduplication**
   - Track batch_id to prevent duplicate writes
   - In-memory cache (LRU, last 10k batch IDs)

3. **Write Buffering**
   - Micro-batch small writes before flushing
   - Configurable flush interval (100ms default)

### Could Have 💭 (Cut first if needed)

1. **CompactTable RPC**
   - Trigger DuckDB OPTIMIZE operations
   - Can be done manually via CLI in v1

2. **GetTableInfo RPC**
   - Query table stats (row count, file count)
   - Nice for debugging, not critical path

3. **Multi-format Support**
   - Parquet and JSONL inputs
   - v1 can be Arrow-only

4. **Partition Hints**
   - Advanced partitioning strategies
   - Can use simple ledger_range for v1

---

## Rabbit Holes

### Don't Do These ❌

1. **Don't build a query engine**
   - This is write-only
   - Reads go directly to DuckDB catalog

2. **Don't implement complex scheduling**
   - No job queue, cron, or workflow engine
   - Just process writes as they come

3. **Don't add authentication in v1**
   - Runs in private network
   - Trust ingesters for now
   - Add auth in v2 if needed

4. **Don't optimize for millions of QPS**
   - Target: 100-200 writes/sec (plenty for 3 networks)
   - Simple buffered channel is enough

5. **Don't make it a distributed system**
   - Single binary, single process
   - No consensus, no sharding
   - Simple and reliable

6. **Don't build custom catalog logic**
   - Use DuckDB catalog server as-is
   - No custom metadata storage

---

## No-Gos

### Explicitly Out of Scope

- ❌ Multi-tenancy (v1 = one catalog for all networks)
- ❌ Authentication/authorization (private network only)
- ❌ Read queries (use DuckDB directly)
- ❌ Schema validation beyond basic type checking
- ❌ Custom compression strategies
- ❌ Distributed deployment
- ❌ Transaction coordination across tables
- ❌ Change data capture / audit logs
- ❌ Custom storage backends (DuckLake only)

---

## Done Looks Like

### Success Demo

**Given:** Three ingestion processes running simultaneously

**When:** All send batches to lake-writer

**Then:**
1. No catalog lock errors ✅
2. Data appears in correct database (testnet/mainnet/futurenet) ✅
3. All 19 tables populated correctly ✅
4. Metrics show writes/sec and queue depth ✅
5. Health endpoint returns 200 OK ✅

### Acceptance Test

```bash
# Terminal 1: Start lake-writer
./lake-writer -config config.yaml

# Terminal 2: Start testnet ingester
./ducklake-ingestion-obsrvr-v3 -config testnet-config.yaml

# Terminal 3: Start mainnet ingester
./ducklake-ingestion-obsrvr-v3 -config mainnet-config.yaml

# Terminal 4: Check data
duckdb
D ATTACH 'ducklake:duckdb:s3://obsrvr-lake/catalogs/obsrvr.duckdb' AS cat;
D SELECT COUNT(*) FROM cat.testnet.ledgers_row_v2;  -- > 0
D SELECT COUNT(*) FROM cat.mainnet.ledgers_row_v2;  -- > 0

# Terminal 5: Check metrics
curl http://localhost:8088/metrics
# Should see:
# lake_writer_batches_total{network="testnet"} 42
# lake_writer_batches_total{network="mainnet"} 38
# lake_writer_queue_depth 2
```

**Ship criteria:**
- ✅ All three networks writing concurrently
- ✅ No lock errors in logs
- ✅ Data queryable via DuckDB
- ✅ Metrics endpoint working
- ✅ README with deployment instructions

---

## Technical Risks & Mitigations

### Risk 1: DuckDB Catalog Server Connection Issues

**Concern:** What if connection to catalog server is flaky?

**Mitigation:**
- Implement retry with exponential backoff
- Buffer writes in memory during reconnect
- Max buffer size = 1000 batches (prevent OOM)
- Drop and error if buffer fills (fail fast)

### Risk 2: Write Throughput Too Low

**Concern:** What if single writer is too slow?

**Mitigation:**
- Profile early (Day 2)
- Target: 100 writes/sec minimum
- If too slow: add batch coalescing (combine small writes)
- DuckDB native ingestion is very fast (should be fine)

### Risk 3: Schema Mismatches

**Concern:** What if ingester sends wrong schema?

**Mitigation:**
- Validate schema on first write to new table
- Cache validated schemas in memory
- Return clear error on mismatch
- Don't auto-evolve in v1 (fail instead)

### Risk 4: Queue Backpressure

**Concern:** What if queue fills up?

**Mitigation:**
- Configurable queue size (default: 1000)
- Block client when full (backpressure)
- Expose queue depth metric
- Alert when >80% full

---

## Dependencies

### External

- DuckDB catalog server (must be running)
- S3/Backblaze B2 (for data storage)
- Network connectivity to catalog and S3

### Internal

- `github.com/duckdb/duckdb-go/v2` (official Go driver)
- `google.golang.org/grpc` (gRPC server)
- `google.golang.org/protobuf` (proto definitions)
- Arrow Go libraries (for Arrow IPC parsing)

### Blocked By

None – can start immediately

### Blocks

- Multi-network ingestion deployment
- Production Bronze layer architecture
- Customer Silver data APIs (downstream)

---

## Deployment

### Configuration

```yaml
# config.yaml
service:
  name: "lake-writer"
  listen_address: ":50099"
  health_port: 8088

catalog:
  catalog_path: "ducklake:duckdb:s3://obsrvr-lake/catalogs/obsrvr.duckdb"

  # S3/B2 credentials
  aws_access_key_id: "${AWS_ACCESS_KEY_ID}"
  aws_secret_access_key: "${AWS_SECRET_ACCESS_KEY}"
  aws_region: "us-west-004"
  aws_endpoint: "https://s3.us-west-004.backblazeb2.com"

databases:
  - name: "testnet"
    data_path: "s3://obsrvr-lake/testnet/"
  - name: "mainnet"
    data_path: "s3://obsrvr-lake/mainnet/"
  - name: "futurenet"
    data_path: "s3://obsrvr-lake/futurenet/"

writer:
  queue_size: 1000
  flush_interval_ms: 100
  max_batch_size: 1000

logging:
  level: "info"
  format: "json"
```

### Running

```bash
# Local development
nix develop
make build
./lake-writer -config config.yaml

# Docker
docker build -t lake-writer:latest .
docker run -p 50099:50099 -p 8088:8088 \
  -e AWS_ACCESS_KEY_ID=xxx \
  -e AWS_SECRET_ACCESS_KEY=yyy \
  lake-writer:latest

# Nix
nix build
nix run
```

---

## Timeline & Hill Chart

### Day 1-2: Figuring Out (Left Side 0-50%)

- [ ] Proto definitions
- [ ] gRPC server boilerplate
- [ ] DuckDB connection setup
- [ ] Basic WriteBatch (single table)

**Milestone:** Can write one batch to one table

### Day 3: Making It Happen (Right Side 50-75%)

- [ ] Multi-table support (19 tables)
- [ ] Multi-network routing
- [ ] Error handling & retries
- [ ] Queue implementation

**Milestone:** Can write to all tables in all networks

### Day 4: Making It Happen (Right Side 75-90%)

- [ ] Metrics & observability
- [ ] Health endpoint
- [ ] Config file parsing
- [ ] Table auto-creation

**Milestone:** Production-ready metrics and config

### Day 5: Making It Happen (Right Side 90-100%)

- [ ] Integration tests
- [ ] README & documentation
- [ ] Docker build
- [ ] Deployment validation

**Milestone:** Shipped and documented

### Cool-Down (Day 6-7 – Mandatory Break)

- Fix bugs from testing
- Improve error messages
- Add logging
- Write deployment guide

---

## Open Questions

1. **Q:** Should we support PostgreSQL catalog as fallback?
   **A:** No – DuckDB catalog only for v1. Keep it simple.

2. **Q:** How do we handle catalog migration/evolution?
   **A:** Out of scope for v1. Manually manage with SQL scripts.

3. **Q:** What about monitoring/alerting integration?
   **A:** Prometheus metrics only. Let ops team configure alerts.

4. **Q:** Do we need graceful shutdown?
   **A:** Yes (nice-to-have). Drain queue on SIGTERM before exiting.

---

## References

- [LakeWriter API Proto Sketch](/docs/LAKE_WRITER_API.md)
- [DuckLake Ingestion v3 README](/home/tillman/Documents/ttp-processor-demo/ducklake-ingestion-obsrvr-v3/README.md)
- [OBSRVR Architecture Docs](/home/tillman/Documents/ttp-processor-demo/docs/)

---

## Sign-Off

**Ready to bet:** ✅ Yes

**Appetite confirmed:** 1 week (fixed)

**Scope line agreed:** Must-have only for v1, cut nice-to-haves if needed

**Ship-or-kill deadline:** Friday EOD (no extensions)

---

**Let's build it. 🚀**
