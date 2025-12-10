# Lake Writer - Operational Runbook

**Version:** 0.1.0
**Last Updated:** 2025-12-10
**Service:** Lake Writer - DuckDB Write Serialization Service

## Table of Contents

1. [Service Overview](#service-overview)
2. [Quick Reference](#quick-reference)
3. [Daily Operations](#daily-operations)
4. [Common Procedures](#common-procedures)
5. [Incident Response](#incident-response)
6. [Monitoring & Alerts](#monitoring--alerts)
7. [Performance Tuning](#performance-tuning)
8. [Escalation](#escalation)

---

## Service Overview

### What is Lake Writer?

Lake Writer is a gRPC service that serializes concurrent write requests from multiple Stellar network ingesters (testnet, mainnet, futurenet) into a single DuckDB catalog.

**Critical Constraint:** DuckDB requires single-writer access. Lake Writer solves this by queueing all writes and processing them serially.

### Architecture Summary

```
3 Network Ingesters → Lake Writer (Queue) → DuckDB Catalog
                        (gRPC API)           (Single Writer)
```

### Key Metrics

- **SLA Target:** 99.9% uptime
- **Expected Throughput:** 100+ writes/sec
- **Expected Queue Depth:** < 50% capacity during normal operation
- **Error Rate Target:** < 0.1%

### Service Dependencies

**Upstream:**
- ducklake-ingestion-obsrvr-v3 (testnet)
- ducklake-ingestion-obsrvr-v3 (mainnet)
- ducklake-ingestion-obsrvr-v3 (futurenet)

**Downstream:**
- DuckDB catalog file (local disk or S3)
- Backblaze B2 / S3 (for Parquet data storage)

**Monitoring:**
- Prometheus (metrics collection)
- Alertmanager (alerting)
- Grafana (dashboards)

---

## Quick Reference

### Service Endpoints

| Endpoint | Port | Purpose |
|----------|------|---------|
| gRPC API | 50099 | WriteBatch RPC |
| Health Check | 8088 | /health endpoint |
| Metrics | 8088 | /metrics (Prometheus) |

### Common Commands

**Systemd:**
```bash
# Status check
sudo systemctl status lake-writer

# Start/Stop/Restart
sudo systemctl start lake-writer
sudo systemctl stop lake-writer
sudo systemctl restart lake-writer

# View logs (live)
sudo journalctl -u lake-writer -f

# View logs (last hour)
sudo journalctl -u lake-writer --since "1 hour ago"
```

**Docker:**
```bash
# Check status
docker ps | grep lake-writer

# View logs
docker logs -f lake-writer

# Restart
docker restart lake-writer

# Stop
docker stop lake-writer
```

**Kubernetes:**
```bash
# Check status
kubectl get pods -n obsrvr -l app=lake-writer

# View logs
kubectl logs -f deployment/lake-writer -n obsrvr

# Restart
kubectl rollout restart deployment/lake-writer -n obsrvr

# Check health
kubectl exec -it deployment/lake-writer -n obsrvr -- curl localhost:8088/health
```

### Health Check

```bash
# Quick health check
curl http://localhost:8088/health

# Expected response (healthy)
{
  "status": "healthy",
  "service": "lake-writer",
  "version": "0.1.0"
}
```

### Metrics Query

```bash
# All metrics
curl http://localhost:8088/metrics

# Key metrics
curl -s http://localhost:8088/metrics | grep -E "lake_writer_(total_writes|total_errors|queue_depth|writes_per_second)"
```

---

## Daily Operations

### Morning Health Check

**Run this checklist every morning:**

1. **Service Status**
   ```bash
   sudo systemctl status lake-writer
   # Expected: active (running)
   ```

2. **Health Endpoint**
   ```bash
   curl http://localhost:8088/health
   # Expected: {"status":"healthy"}
   ```

3. **Error Rate**
   ```bash
   curl -s http://localhost:8088/metrics | grep lake_writer_total_errors
   # Check if error count increased overnight
   ```

4. **Queue Depth**
   ```bash
   curl -s http://localhost:8088/health | jq '.queue_depth'
   # Expected: < 500 (< 50% of capacity)
   ```

5. **Disk Space**
   ```bash
   df -h /opt/lake-writer/data
   # Expected: < 80% used
   ```

6. **Log Errors**
   ```bash
   sudo journalctl -u lake-writer --since "24 hours ago" | grep ERROR | wc -l
   # Expected: 0 or very low
   ```

**Checklist Template:**
```
[ ] Service running
[ ] Health endpoint responding
[ ] Error rate acceptable
[ ] Queue depth normal
[ ] Disk space available
[ ] No critical log errors
```

### Weekly Maintenance

**Run this checklist every Monday:**

1. **Review Metrics Trends**
   - Check Grafana dashboard for the past week
   - Look for throughput degradation
   - Monitor queue depth trends

2. **Check Catalog Size**
   ```bash
   ls -lh /opt/lake-writer/data/catalogs/
   # Monitor growth rate
   ```

3. **Verify Backups**
   ```bash
   ls -lh /backups/ | grep lake-writer
   # Ensure weekly backup exists
   ```

4. **Log Analysis**
   ```bash
   sudo journalctl -u lake-writer --since "7 days ago" | grep WARN | sort | uniq -c | sort -rn
   # Review warning patterns
   ```

5. **Performance Review**
   - Review write latency in Grafana
   - Check for throughput bottlenecks
   - Assess need for capacity increase

---

## Common Procedures

### Procedure 1: Service Restart

**When:** After configuration changes, during high queue depth, or as part of troubleshooting.

**Steps:**

1. **Check current queue depth**
   ```bash
   curl -s http://localhost:8088/health | jq '.queue_depth'
   ```
   - If queue > 100, wait for it to drain before restart
   - Watch queue: `watch -n 5 'curl -s http://localhost:8088/health | jq .queue_depth'`

2. **Stop service (graceful)**
   ```bash
   sudo systemctl stop lake-writer
   ```
   - Service will drain queue before exiting
   - Check logs to confirm shutdown: `sudo journalctl -u lake-writer -f`

3. **Verify service stopped**
   ```bash
   sudo systemctl status lake-writer
   # Expected: inactive (dead)
   ```

4. **Start service**
   ```bash
   sudo systemctl start lake-writer
   ```

5. **Verify startup**
   ```bash
   # Check logs
   sudo journalctl -u lake-writer -f
   # Expected: "Initialized 3 databases", "Starting Lake Writer"

   # Check health
   sleep 5 && curl http://localhost:8088/health
   ```

6. **Monitor for 5 minutes**
   ```bash
   watch -n 10 'curl -s http://localhost:8088/health'
   ```

**Rollback:** If service fails to start, check logs and revert configuration changes.

### Procedure 2: Configuration Update

**When:** Changing queue size, database paths, S3 credentials, etc.

**Steps:**

1. **Backup current config**
   ```bash
   sudo cp /opt/lake-writer/config/production.yaml /opt/lake-writer/config/production.yaml.backup-$(date +%Y%m%d)
   ```

2. **Edit configuration**
   ```bash
   sudo nano /opt/lake-writer/config/production.yaml
   ```

3. **Validate syntax (YAML)**
   ```bash
   # Use YAML linter if available
   python3 -c "import yaml; yaml.safe_load(open('/opt/lake-writer/config/production.yaml'))"
   ```

4. **Restart service** (see Procedure 1)

5. **Verify configuration applied**
   ```bash
   sudo journalctl -u lake-writer -n 50 | grep -E "queue_size|data_path"
   ```

6. **Monitor for 10 minutes**
   ```bash
   watch -n 10 'curl -s http://localhost:8088/health'
   ```

**Rollback:**
```bash
sudo cp /opt/lake-writer/config/production.yaml.backup-YYYYMMDD /opt/lake-writer/config/production.yaml
sudo systemctl restart lake-writer
```

### Procedure 3: Disk Space Cleanup

**When:** Disk usage > 80%

**Steps:**

1. **Identify large files**
   ```bash
   sudo du -h /opt/lake-writer/data | sort -rh | head -20
   ```

2. **Check log files**
   ```bash
   sudo journalctl --disk-usage
   # If > 4GB:
   sudo journalctl --vacuum-time=30d
   ```

3. **Check old catalog backups**
   ```bash
   ls -lh /backups/lake-writer-*
   # Delete backups older than 30 days if needed
   ```

4. **Check DuckDB catalog size**
   ```bash
   ls -lh /opt/lake-writer/data/catalogs/*.duckdb
   ```
   - If very large (> 50GB), consider VACUUM operation (advanced)

5. **Verify disk space recovered**
   ```bash
   df -h /opt/lake-writer/data
   ```

### Procedure 4: Queue Full Response

**When:** Alert fires: "LakeWriterQueueFull" or queue_depth >= 9000

**Severity:** WARNING

**Steps:**

1. **Check current queue depth**
   ```bash
   curl -s http://localhost:8088/health | jq .
   ```

2. **Check write rate**
   ```bash
   curl -s http://localhost:8088/metrics | grep writes_per_second
   ```

3. **Check for slow writes**
   ```bash
   sudo journalctl -u lake-writer | tail -100 | grep "duration_ms"
   # Look for unusually high durations
   ```

4. **Check disk I/O**
   ```bash
   iostat -x 1 5
   # Look for high %util on data disk
   ```

5. **Temporary mitigation:**
   - If queue is draining (depth decreasing), monitor and wait
   - If queue is growing, consider:
     a. Increase queue size (requires restart)
     b. Ask ingesters to slow down (backpressure)
     c. Check for disk I/O bottleneck

6. **Increase queue size (if needed):**
   ```bash
   # Edit config
   sudo nano /opt/lake-writer/config/production.yaml
   # Change: queue.max_size: 20000

   # Restart (see Procedure 1)
   sudo systemctl restart lake-writer
   ```

7. **Monitor for recovery**
   ```bash
   watch -n 5 'curl -s http://localhost:8088/health | jq .queue_depth'
   ```

### Procedure 5: Binary Upgrade

**When:** New version available

**Steps:**

1. **Download/build new binary**
   ```bash
   cd /opt/lake-writer/build
   nix build
   ```

2. **Stop service**
   ```bash
   sudo systemctl stop lake-writer
   ```

3. **Backup current binary**
   ```bash
   sudo cp /opt/lake-writer/bin/lake-writer /opt/lake-writer/bin/lake-writer.backup-$(date +%Y%m%d)
   ```

4. **Install new binary**
   ```bash
   sudo cp result/bin/server /opt/lake-writer/bin/lake-writer
   sudo chown obsrvr:obsrvr /opt/lake-writer/bin/lake-writer
   sudo chmod +x /opt/lake-writer/bin/lake-writer
   ```

5. **Verify binary**
   ```bash
   /opt/lake-writer/bin/lake-writer -version
   # Or check logs after start
   ```

6. **Start service**
   ```bash
   sudo systemctl start lake-writer
   ```

7. **Monitor startup**
   ```bash
   sudo journalctl -u lake-writer -f
   ```

8. **Verify health**
   ```bash
   curl http://localhost:8088/health
   ```

9. **Monitor for 30 minutes**
   - Watch metrics dashboard
   - Check for errors in logs
   - Verify write success rate

**Rollback:**
```bash
sudo systemctl stop lake-writer
sudo cp /opt/lake-writer/bin/lake-writer.backup-YYYYMMDD /opt/lake-writer/bin/lake-writer
sudo systemctl start lake-writer
```

---

## Incident Response

### Incident Classification

| Severity | Definition | Response Time |
|----------|-----------|---------------|
| **P0 - Critical** | Service down, no writes processing | Immediate (< 5 min) |
| **P1 - High** | Degraded performance, high error rate | < 15 minutes |
| **P2 - Medium** | Elevated queue, warnings in logs | < 1 hour |
| **P3 - Low** | Informational, no user impact | Next business day |

### P0 Incident: Service Down

**Symptoms:**
- Health endpoint not responding
- Service not running
- All writes failing

**Response:**

1. **Acknowledge alert**
   ```bash
   # Silence alert in Alertmanager (if applicable)
   ```

2. **Check service status**
   ```bash
   sudo systemctl status lake-writer
   ```

3. **Check logs for crash**
   ```bash
   sudo journalctl -u lake-writer -n 100 --no-pager
   ```

4. **Common causes:**
   - **Config error:** Fix config, restart
   - **Disk full:** Free space (see Procedure 3)
   - **Catalog corruption:** Restore from backup
   - **Binary missing:** Reinstall binary

5. **Restart service**
   ```bash
   sudo systemctl start lake-writer
   ```

6. **If fails to start:**
   - Check config syntax
   - Verify file permissions
   - Check disk space
   - Restore from backup if needed

7. **Verify recovery**
   ```bash
   curl http://localhost:8088/health
   ```

8. **Notify stakeholders**
   - Update status page
   - Notify ingesters
   - Post incident channel update

9. **Post-incident:**
   - Write incident report
   - Identify root cause
   - Create prevention tasks

### P1 Incident: High Error Rate

**Symptoms:**
- `lake_writer_total_errors` increasing rapidly
- Alert: "LakeWriterHighErrorRate"

**Response:**

1. **Check error rate**
   ```bash
   curl -s http://localhost:8088/metrics | grep lake_writer_total_errors
   ```

2. **Check recent errors in logs**
   ```bash
   sudo journalctl -u lake-writer --since "15 minutes ago" | grep ERROR
   ```

3. **Common error types:**

   **a. Schema mismatch:**
   ```
   Error: failed to write batch: column count mismatch
   ```
   - Action: Contact ingester team, schema may have changed

   **b. S3/B2 connectivity:**
   ```
   Error: failed to write parquet: connection refused
   ```
   - Action: Check S3/B2 status, verify credentials

   **c. Catalog locked:**
   ```
   Error: Could not set lock on file
   ```
   - Action: Check for external DuckDB connections

4. **Temporary mitigation:**
   - Errors are logged but don't stop service
   - Failed batches will be retried by ingesters
   - Monitor error rate trend

5. **Fix root cause** (based on error type)

6. **Verify recovery**
   ```bash
   watch -n 10 'curl -s http://localhost:8088/metrics | grep total_errors'
   ```

### P2 Incident: Elevated Queue Depth

See **Procedure 4: Queue Full Response**

### P3 Incident: Performance Degradation

**Symptoms:**
- Writes/sec decreasing over time
- Write latency increasing
- Queue depth slowly growing

**Response:**

1. **Check current performance**
   ```bash
   curl -s http://localhost:8088/metrics | grep writes_per_second
   ```

2. **Check disk I/O**
   ```bash
   iostat -x 1 10
   ```

3. **Check catalog file size**
   ```bash
   ls -lh /opt/lake-writer/data/catalogs/*.duckdb
   ```

4. **Check system resources**
   ```bash
   top -p $(pgrep lake-writer)
   free -h
   ```

5. **Optimization options:**
   - Move catalog to faster SSD
   - Increase system RAM
   - Consider DuckDB VACUUM (if catalog very large)
   - Add monitoring for I/O bottlenecks

6. **Schedule maintenance window** for optimization

---

## Monitoring & Alerts

### Key Metrics to Monitor

| Metric | Normal Range | Warning Threshold | Critical Threshold |
|--------|--------------|-------------------|-------------------|
| `lake_writer_writes_per_second` | 50-200 | < 20 | < 5 |
| `lake_writer_queue_depth` | 0-500 | 5000-9000 | > 9000 |
| `lake_writer_total_errors` (rate) | 0 | > 0.01/sec | > 0.1/sec |
| Disk usage (catalog) | < 70% | 70-85% | > 85% |
| Memory usage | < 4GB | 4-6GB | > 7GB |
| CPU usage | < 50% | 50-80% | > 80% |

### Alert Rules

**Critical Alerts (P0):**
```yaml
- alert: LakeWriterDown
  expr: up{job="lake-writer"} == 0
  for: 1m
  annotations:
    summary: "Lake Writer service is down"
    runbook: "Check service status, logs, restart if needed"
```

**High Priority Alerts (P1):**
```yaml
- alert: LakeWriterHighErrorRate
  expr: rate(lake_writer_total_errors[5m]) > 0.01
  for: 5m
  annotations:
    summary: "Lake Writer error rate elevated"
    runbook: "Check logs for error patterns"
```

**Warning Alerts (P2):**
```yaml
- alert: LakeWriterQueueFull
  expr: lake_writer_queue_depth >= 9000
  for: 5m
  annotations:
    summary: "Lake Writer queue approaching capacity"
    runbook: "Follow Queue Full Response procedure"
```

### Grafana Dashboards

**Key Panels:**
1. **Write Throughput** - `lake_writer_writes_per_second` (graph)
2. **Queue Depth** - `lake_writer_queue_depth` (graph)
3. **Error Rate** - `rate(lake_writer_total_errors[5m])` (graph)
4. **Health Status** - `up{job="lake-writer"}` (singlestat)
5. **Disk Usage** - File system metrics
6. **Memory Usage** - Process memory

**Dashboard URL:** [Add your Grafana dashboard URL]

---

## Performance Tuning

### Queue Size Tuning

**Default:** 10,000 batches

**Increase if:**
- Queue consistently > 50% during normal operation
- Bursty traffic patterns from ingesters

**Decrease if:**
- Memory usage too high
- Queue never exceeds 10% capacity

**How to change:**
```yaml
queue:
  max_size: 20000  # Adjust as needed
```

### Disk I/O Optimization

**Check current I/O:**
```bash
iostat -x 1 10
```

**Optimizations:**
1. **Use SSD for catalog file**
   - 10x performance improvement over HDD

2. **Separate data and catalog disks**
   - Catalog on fast local SSD
   - Parquet data on S3/B2

3. **Adjust I/O scheduler (SSD):**
   ```bash
   echo "none" | sudo tee /sys/block/nvme0n1/queue/scheduler
   ```

### Memory Tuning

**Current usage:**
```bash
ps aux | grep lake-writer | awk '{print $6/1024 " MB"}'
```

**DuckDB memory limit** (future config option):
```yaml
catalog:
  duckdb_config:
    memory_limit: "4GB"
```

### System Limits

**File descriptors:**
```bash
# In systemd service
LimitNOFILE=65536
```

**Verify:**
```bash
cat /proc/$(pgrep lake-writer)/limits | grep "open files"
```

---

## Escalation

### On-Call Contacts

| Role | Contact | Escalation Time |
|------|---------|-----------------|
| Primary On-Call | [Name/Slack] | Immediate |
| Secondary On-Call | [Name/Slack] | 15 minutes |
| Engineering Lead | [Name/Slack] | 30 minutes |
| Database Expert | [Name/Slack] | As needed |

### Escalation Criteria

**Escalate to Secondary after 15 minutes if:**
- Unable to determine root cause
- Service won't restart
- Data corruption suspected

**Escalate to Engineering Lead after 30 minutes if:**
- Incident not resolved
- Workaround not found
- User impact ongoing

**Escalate to Database Expert if:**
- Catalog corruption detected
- DuckDB-specific errors
- Performance tuning needed

### Communication Channels

- **Incident Channel:** #incident-lake-writer (Slack)
- **Status Updates:** #status-updates (Slack)
- **Engineering:** #engineering (Slack)

### Post-Incident Review

**Within 24 hours of P0/P1 incident:**

1. **Write incident report**
   - Timeline of events
   - Root cause analysis
   - Impact assessment
   - Resolution steps

2. **Identify action items**
   - Preventive measures
   - Monitoring improvements
   - Documentation updates

3. **Schedule review meeting**
   - Share learnings
   - Update runbook
   - Assign follow-up tasks

---

## Appendix

### Useful Commands Cheat Sheet

```bash
# Quick health check
curl -s http://localhost:8088/health | jq .

# Watch metrics live
watch -n 5 'curl -s http://localhost:8088/metrics | grep -E "writes_per_second|queue_depth|total_errors"'

# Check queue depth trend
for i in {1..10}; do curl -s http://localhost:8088/health | jq .queue_depth; sleep 5; done

# Count errors in last hour
sudo journalctl -u lake-writer --since "1 hour ago" | grep ERROR | wc -l

# Check disk space
df -h /opt/lake-writer/data

# Check process resources
top -p $(pgrep lake-writer)

# Follow logs with filtering
sudo journalctl -u lake-writer -f | grep -E "ERROR|WARN|Batch written"

# Test gRPC endpoint (if grpcurl installed)
grpcurl -plaintext localhost:50099 list
```

### Log Patterns

**Normal operation:**
```
INFO Batch written successfully batch_id=testnet-12345 rows=100 duration_ms=8
```

**Warning (duplicate batch):**
```
WARN Duplicate batch_id, skipping batch_id=testnet-12345
```

**Error (write failure):**
```
ERROR Failed to write batch error="schema mismatch" batch_id=testnet-12345
```

**Critical (service start failure):**
```
FATAL Failed to initialize catalog error="permission denied"
```

### File Locations

**Binary:**
- Systemd: `/opt/lake-writer/bin/lake-writer`
- Docker: `/bin/server` (inside container)

**Configuration:**
- Systemd: `/opt/lake-writer/config/production.yaml`
- Docker: Volume-mounted at `/config/`

**Data:**
- Catalog: `/opt/lake-writer/data/catalogs/`
- Parquet: S3/B2 or `/opt/lake-writer/data/`

**Logs:**
- Systemd: `journalctl -u lake-writer`
- Docker: `docker logs lake-writer`
- Kubernetes: `kubectl logs deployment/lake-writer`

**Backups:**
- `/backups/lake-writer-YYYYMMDD.tar.gz`

---

## Document Version History

| Version | Date | Changes |
|---------|------|---------|
| 0.1.0 | 2025-12-10 | Initial runbook for Day 5 ship |

---

**End of Runbook**

For deployment instructions, see [DEPLOYMENT.md](DEPLOYMENT.md)
For integration guide, see [INTEGRATION.md](INTEGRATION.md)
For technical details, see [README.md](README.md)
