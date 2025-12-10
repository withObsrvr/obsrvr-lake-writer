# CLAUDE.md

This file provides guidance to Claude Code when working with the Lake Writer service.

## Project Overview

Lake Writer is a gRPC service that solves the single-writer constraint of DuckDB catalog servers by centralizing all writes through an API. It enables multiple network ingestion processes (testnet, mainnet, futurenet) to write to a shared DuckLake catalog concurrently.

### Architecture

```
Multiple Ingesters → Lake Writer (gRPC) → DuckDB Catalog Server (single writer) → DuckLake (S3/B2)
```

## Core Principles

1. **Single Writer Guarantee**: Only one Lake Writer instance writes to a given catalog
2. **Queue-Based Serialization**: Multiple client requests are queued and processed sequentially
3. **Must-Have First**: Focus on WriteBatch RPC; other RPCs are nice-to-have
4. **Fixed Time, Variable Scope**: 1 week appetite; cut nice-to-haves if needed

## Build Commands

### Development

```bash
# Enter Nix development environment
nix develop

# Generate protobuf code
make gen-proto

# Vendor dependencies
make vendor

# Build binary
make build

# Run locally
./lake-writer -config config/local.yaml
```

### Production Build

```bash
# Build with Nix
nix build

# Build Docker image
make docker-build

# Or build Docker from Nix
nix build .#docker
docker load < result
```

## Key Files

- **`protos/lake_writer/lake_writer.proto`** - gRPC API definition
- **`go/cmd/server/main.go`** - Service entry point
- **`go/internal/writer/writer.go`** - gRPC service implementation
- **`go/internal/catalog/manager.go`** - DuckDB catalog management
- **`go/internal/metrics/collector.go`** - Prometheus metrics
- **`config/local.yaml`** - Local development config
- **`config/production.yaml`** - Production config
- **`PITCH.md`** - Shape Up pitch document

## Implementation Status (Week 1)

### ✅ Completed (Must-Have)
- Proto API definition
- gRPC server boilerplate
- Basic WriteBatch skeleton
- Catalog manager structure
- Metrics collector
- Configuration system

### 🚧 In Progress (Must-Have)
- Arrow IPC payload parsing
- DuckDB write integration
- Error handling & retries
- Full observability

### 💭 Future (Nice-to-Have)
- RegisterTable implementation
- Batch deduplication
- CompactTable implementation
- GetTableInfo implementation
- Multi-format support (Parquet, JSONL)

## Development Guidelines

### Adding New Features

1. **Check Scope**: Is this must-have or nice-to-have? (See PITCH.md)
2. **Update Todos**: Use TodoWrite to track progress
3. **Follow Patterns**: Match existing code style (internal/ packages, error handling)
4. **Test**: Add tests for new functionality
5. **Document**: Update README and inline comments

### Code Style

- Use `gofmt` for formatting
- Follow Go best practices (effective Go)
- Keep functions focused and small
- Use descriptive variable names
- Add comments for non-obvious logic

### Testing

```bash
# Run tests
make test

# Test manually
./lake-writer -config config/local.yaml

# Check health
curl http://localhost:8088/health

# Check metrics
curl http://localhost:8088/metrics
```

## Integration with Other Services

### Client Side (ducklake-ingestion-obsrvr-v3)

The ingestion service should be modified to:

1. Add lake-writer gRPC client
2. Send batches via WriteBatch RPC instead of direct DuckDB writes
3. Handle backpressure (queue full errors)

Example config:
```yaml
use_lake_writer: true
lake_writer_endpoint: "lake-writer:50099"
network_name: "testnet"
```

### Catalog Structure

```
obsrvr.duckdb (catalog)
├── testnet (database/schema)
│   ├── ledgers_row_v2
│   ├── transactions_row_v2
│   └── ... (19 tables total)
├── mainnet (database/schema)
│   ├── ledgers_row_v2
│   └── ...
└── futurenet (database/schema)
    └── ...
```

## Troubleshooting

### Build Errors

- **Proto generation fails**: Run `make clean` then `make gen-proto`
- **Vendor errors**: Run `make vendor` to refresh dependencies
- **Nix build fails**: Check `flake.lock` is up to date

### Runtime Errors

- **Catalog lock errors**: Ensure only one Lake Writer instance is running
- **Queue full errors**: Increase `queue_size` in config or scale up resources
- **S3 connection errors**: Check AWS credentials and endpoint configuration

## Dependencies

- **DuckDB Go Driver**: `github.com/duckdb/duckdb-go/v2` (official driver)
- **gRPC**: `google.golang.org/grpc` v1.67+
- **Prometheus**: `github.com/prometheus/client_golang` v1.20+
- **YAML**: `gopkg.in/yaml.v3`

## Shape Up Progress Tracking

Use TodoWrite tool to track progress through the week:
- Day 1-2: Left side of hill (figuring out)
- Day 3-4: Right side of hill (making it happen)
- Day 5: Shipping
- Day 6-7: Mandatory cool-down

If stuck on left side at 50% time → Cut scope immediately

## References

- [Shape Up Methodology](https://basecamp.com/shapeup)
- [OBSRVR Architecture Docs](/home/tillman/Documents/ttp-processor-demo/docs/)
- [DuckDB Documentation](https://duckdb.org/docs/)
- [Go gRPC Guide](https://grpc.io/docs/languages/go/)
