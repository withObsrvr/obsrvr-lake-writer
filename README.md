# Obsrvr Lake Writer

**Version:** 0.1.0
**Status:** Development (Week 1 - Shape Up Cycle)

Serialized write service for DuckLake multi-network ingestion. Solves the single-writer constraint of DuckDB catalog servers by centralizing all writes through a gRPC API.

---

## Problem

DuckDB catalog server supports **single-writer access only**, but we have multiple network ingestion processes (testnet, mainnet, futurenet) that all need to write to the same catalog.

**Lake Writer** solves this by:
- Accepting writes from many clients via gRPC
- Serializing all writes internally through a single DuckDB connection
- Maintaining catalog consistency and data inlining benefits

---

## Architecture

```
┌─────────────────┐
│ testnet-ingest  │────┐
└─────────────────┘    │
                       │
┌─────────────────┐    │     ┌──────────────────┐      ┌─────────────────┐
│ mainnet-ingest  │────┼────►│  lake-writer     │─────►│ DuckDB Catalog  │
└─────────────────┘    │     │  (gRPC server)   │      │    Server       │
                       │     │                  │      └─────────────────┘
┌─────────────────┐    │     │ Port 50099       │              │
│futurenet-ingest │────┘     │ Health: 8088     │              │
└─────────────────┘          └──────────────────┘              ▼
                                                       ┌─────────────────┐
                                                       │  DuckLake       │
                                                       │  (S3/B2)        │
                                                       └─────────────────┘
```

---

## Quick Start

### Development Setup (Nix)

```bash
# Enter development environment
nix develop

# Generate protobuf code
make gen-proto

# Vendor dependencies
make vendor

# Build
make build

# Run locally
./lake-writer -config config/local.yaml
```

### Manual Setup

```bash
# Install dependencies
cd go
GOWORK=off go mod download

# Generate proto
make gen-proto

# Build
make build

# Run
./lake-writer -config config/local.yaml
```

---

## Configuration

### Local Development (`config/local.yaml`)

```yaml
service:
  name: "lake-writer"
  listen_address: ":50099"
  health_port: 8088

catalog:
  catalog_path: "ducklake:duckdb:./data/catalogs/local.duckdb"

databases:
  - name: "testnet"
    data_path: "./data/testnet/"
  - name: "mainnet"
    data_path: "./data/mainnet/"
  - name: "futurenet"
    data_path: "./data/futurenet/"

writer:
  queue_size: 1000
  flush_interval_ms: 100
  max_batch_size: 1000
```

### Production (`config/production.yaml`)

```yaml
service:
  name: "lake-writer"
  listen_address: ":50099"
  health_port: 8088

catalog:
  catalog_path: "ducklake:duckdb:s3://obsrvr-lake/catalogs/obsrvr.duckdb"
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
```

---

## gRPC API

### Proto Definition

See [`protos/lake_writer/lake_writer.proto`](protos/lake_writer/lake_writer.proto)

### Key RPCs

#### WriteBatch

Write a batch of data to a table.

```protobuf
rpc WriteBatch(WriteBatchRequest) returns (WriteBatchResponse);

message WriteBatchRequest {
  Destination destination;        // database + table
  string pipeline_id;             // for tracing
  string batch_id;                // idempotency key
  DataFormat format;              // ARROW_IPC, PARQUET, JSON_LINES
  bytes payload;                  // serialized data
  TableWriteMode table_write_mode; // CREATE_IF_MISSING, etc.
}

message WriteBatchResponse {
  Status status;                  // OK, SKIP, RETRYABLE, ERROR
  string message;
  uint64 rows_written;
  string batch_id;                // echo for idempotency
}
```

#### HealthCheck

Check service health and get metrics.

```protobuf
rpc HealthCheck(HealthCheckRequest) returns (HealthCheckResponse);

message HealthCheckResponse {
  Status status;                  // HEALTHY, DEGRADED, UNHEALTHY
  uint64 queue_depth;
  uint64 writes_per_second;
  uint64 total_writes;
  uint64 total_errors;
}
```

---

## Metrics

### Prometheus Metrics (`:8088/metrics`)

```
# Total batches written
lake_writer_batches_total{database="testnet",table="ledgers_row_v2"} 42

# Total errors
lake_writer_errors_total{database="testnet",table="ledgers_row_v2"} 0

# Current queue depth
lake_writer_queue_depth 5

# Write duration histogram
lake_writer_write_duration_seconds{database="testnet",table="ledgers_row_v2"} ...

# Batch size histogram
lake_writer_batch_size_rows ...
```

### Health Endpoint (`:8088/health`)

```bash
curl http://localhost:8088/health

{
  "status": "healthy",
  "service": "lake-writer",
  "version": "0.1.0"
}
```

---

## Client Integration

### Modify ducklake-ingestion-obsrvr-v3

Add lake-writer sink mode:

```yaml
# ducklake-ingestion-obsrvr-v3 config
source:
  endpoint: "stellar-live-source-testnet:50053"
  network_passphrase: "Test SDF Network ; September 2015"

# NEW: Lake Writer mode
use_lake_writer: true
lake_writer_endpoint: "lake-writer:50099"
network_name: "testnet"
```

In Go code:

```go
import pb "github.com/withObsrvr/obsrvr-lake-writer/gen/lake_writer"

// Connect to lake-writer
conn, _ := grpc.Dial("lake-writer:50099", grpc.WithInsecure())
client := pb.NewLakeWriterClient(conn)

// Write batch
resp, err := client.WriteBatch(ctx, &pb.WriteBatchRequest{
    Destination: &pb.Destination{
        Database: "testnet",
        Table:    "ledgers_row_v2",
    },
    PipelineId: "ducklake-ingestion-testnet",
    BatchId:    fmt.Sprintf("testnet-%d-%d", startLedger, endLedger),
    Format:     pb.DataFormat_DATA_FORMAT_ARROW_IPC,
    Payload:    arrowIPCBytes,
    TableWriteMode: pb.TableWriteMode_TABLE_WRITE_MODE_CREATE_IF_MISSING,
})
```

---

## Development Workflow

### Building

```bash
# Generate protobuf code
make gen-proto

# Build binary
make build

# Build with Nix
nix build

# Build Docker image
make docker-build
```

### Testing

```bash
# Run tests
make test

# Run locally
make run
```

### Linting

```bash
# Format code
cd go && gofmt -w .

# Vet code
cd go && go vet ./...
```

---

## Deployment

### Docker

```bash
# Build image
docker build -t obsrvr-lake-writer:latest .

# Run container
docker run -p 50099:50099 -p 8088:8088 \
  -e AWS_ACCESS_KEY_ID=xxx \
  -e AWS_SECRET_ACCESS_KEY=yyy \
  obsrvr-lake-writer:latest
```

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: lake-writer
spec:
  replicas: 1  # Single instance (single writer constraint)
  selector:
    matchLabels:
      app: lake-writer
  template:
    metadata:
      labels:
        app: lake-writer
    spec:
      containers:
      - name: lake-writer
        image: obsrvr-lake-writer:latest
        ports:
        - containerPort: 50099
          name: grpc
        - containerPort: 8088
          name: metrics
        env:
        - name: AWS_ACCESS_KEY_ID
          valueFrom:
            secretKeyRef:
              name: s3-credentials
              key: access-key-id
        - name: AWS_SECRET_ACCESS_KEY
          valueFrom:
            secretKeyRef:
              name: s3-credentials
              key: secret-access-key
        volumeMounts:
        - name: config
          mountPath: /config
        args:
        - "-config"
        - "/config/production.yaml"
      volumes:
      - name: config
        configMap:
          name: lake-writer-config
---
apiVersion: v1
kind: Service
metadata:
  name: lake-writer
spec:
  selector:
    app: lake-writer
  ports:
  - name: grpc
    port: 50099
    targetPort: 50099
  - name: metrics
    port: 8088
    targetPort: 8088
```

---

## Troubleshooting

### Queue Full Errors

**Symptom:** Clients get `queue full` errors

**Solution:**
- Increase `queue_size` in config
- Check if catalog writes are slow (network latency to S3)
- Scale up lake-writer resources (CPU/memory)

### Catalog Lock Errors

**Symptom:** DuckDB lock errors in logs

**Solution:**
- Ensure only ONE lake-writer instance is running
- Check for rogue direct writers to catalog

### Slow Write Performance

**Symptom:** Low `writes_per_second` metric

**Solution:**
- Check network latency to S3/B2
- Increase DuckDB buffer sizes
- Enable batch coalescing (future feature)

---

## Roadmap

### Week 1 (Current) - Must Have ✅

- [x] Proto API definition
- [x] gRPC server boilerplate
- [x] Basic WriteBatch implementation
- [ ] Arrow IPC payload parsing
- [ ] DuckDB write integration
- [ ] Multi-network routing
- [ ] Error handling & retries
- [ ] Metrics & observability

### Future - Nice to Have 🎯

- [ ] RegisterTable RPC implementation
- [ ] Batch deduplication cache
- [ ] Write buffering/coalescing
- [ ] CompactTable RPC
- [ ] GetTableInfo RPC
- [ ] Multi-format support (Parquet, JSONL)

---

## Contributing

### Code Structure

```
obsrvr-lake-writer/
├── protos/                # Protocol buffer definitions
│   └── lake_writer/
│       └── lake_writer.proto
├── go/
│   ├── cmd/
│   │   └── server/
│   │       └── main.go   # Service entry point
│   ├── internal/
│   │   ├── writer/       # gRPC service implementation
│   │   ├── catalog/      # DuckDB catalog manager
│   │   └── metrics/      # Prometheus metrics
│   └── gen/              # Generated protobuf code
├── config/               # Configuration files
├── docs/                 # Additional documentation
├── Makefile              # Build targets
├── flake.nix             # Nix development environment
└── README.md             # This file
```

### Pull Request Guidelines

1. All PRs must have passing tests
2. Follow existing code style (use `gofmt`)
3. Update documentation for API changes
4. Keep PRs focused and small

---

## License

Apache 2.0

---

## References

- [Shape Up Pitch](PITCH.md) - Full project pitch and scope
- [DuckLake Architecture](/home/tillman/Documents/ttp-processor-demo/docs/)
- [Proto API Spec](protos/lake_writer/lake_writer.proto)

---

**Status:** Week 1 development in progress

**Next Steps:**
1. Implement Arrow IPC parsing
2. Integrate DuckDB writes via catalog manager
3. Add integration tests with mock ingester
4. Deploy to staging environment
