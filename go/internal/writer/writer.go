package writer

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/withObsrvr/obsrvr-lake-writer/go/internal/catalog"
	"github.com/withObsrvr/obsrvr-lake-writer/go/internal/metrics"

	pb "github.com/withObsrvr/obsrvr-lake-writer/gen/lake_writer"
)

// Config holds the writer configuration
type Config struct {
	QueueSize       int
	FlushIntervalMs int
	MaxBatchSize    int
	Catalog         *catalog.Manager
	Metrics         *metrics.Collector
}

// Writer implements the LakeWriter gRPC service
type Writer struct {
	pb.UnimplementedLakeWriterServer

	cfg     Config
	queue   chan *writeRequest
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	catalog *catalog.Manager
	metrics *metrics.Collector

	// Batch deduplication cache (nice-to-have)
	seenBatches sync.Map // map[string]bool
}

type writeRequest struct {
	req      *pb.WriteBatchRequest
	respChan chan *writeResponse
}

type writeResponse struct {
	resp *pb.WriteBatchResponse
	err  error
}

// New creates a new Writer instance
func New(ctx context.Context, cfg Config) (*Writer, error) {
	if cfg.Catalog == nil {
		return nil, fmt.Errorf("catalog manager is required")
	}
	if cfg.Metrics == nil {
		return nil, fmt.Errorf("metrics collector is required")
	}

	writerCtx, cancel := context.WithCancel(ctx)

	w := &Writer{
		cfg:     cfg,
		queue:   make(chan *writeRequest, cfg.QueueSize),
		ctx:     writerCtx,
		cancel:  cancel,
		catalog: cfg.Catalog,
		metrics: cfg.Metrics,
	}

	// Start the writer goroutine (single writer)
	w.wg.Add(1)
	go w.writerLoop()

	log.Printf("Lake writer initialized (queue_size=%d, flush_interval=%dms)",
		cfg.QueueSize, cfg.FlushIntervalMs)

	return w, nil
}

// WriteBatch implements the gRPC WriteBatch method
func (w *Writer) WriteBatch(ctx context.Context, req *pb.WriteBatchRequest) (*pb.WriteBatchResponse, error) {
	// Check for duplicate batch_id (nice-to-have)
	if req.BatchId != "" {
		if _, seen := w.seenBatches.LoadOrStore(req.BatchId, true); seen {
			return &pb.WriteBatchResponse{
				Status:      pb.WriteBatchResponse_STATUS_SKIP,
				Message:     "duplicate batch_id",
				BatchId:     req.BatchId,
				RowsWritten: 0,
			}, nil
		}
	}

	// Create response channel
	respChan := make(chan *writeResponse, 1)

	// Queue the write request
	select {
	case w.queue <- &writeRequest{req: req, respChan: respChan}:
		// Successfully queued
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-w.ctx.Done():
		return nil, fmt.Errorf("writer is shutting down")
	}

	// Wait for response
	select {
	case resp := <-respChan:
		if resp.err != nil {
			w.metrics.RecordError(req.Destination.Database, req.Destination.Table)
			return nil, resp.err
		}
		w.metrics.RecordWrite(req.Destination.Database, req.Destination.Table, resp.resp.RowsWritten)
		return resp.resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-w.ctx.Done():
		return nil, fmt.Errorf("writer is shutting down")
	}
}

// RegisterTable implements the gRPC RegisterTable method
func (w *Writer) RegisterTable(ctx context.Context, req *pb.RegisterTableRequest) (*pb.RegisterTableResponse, error) {
	// TODO: Implement table registration
	// For now, tables are auto-created on first write
	return &pb.RegisterTableResponse{
		Created:   false,
		TablePath: "",
	}, nil
}

// CompactTable implements the gRPC CompactTable method
func (w *Writer) CompactTable(ctx context.Context, req *pb.CompactTableRequest) (*pb.CompactTableResponse, error) {
	// TODO: Implement compaction trigger
	// For v1, this can be done manually
	return &pb.CompactTableResponse{
		Accepted: false,
		JobId:    "",
	}, nil
}

// GetTableInfo implements the gRPC GetTableInfo method
func (w *Writer) GetTableInfo(ctx context.Context, req *pb.GetTableInfoRequest) (*pb.GetTableInfoResponse, error) {
	// TODO: Implement table info query
	return &pb.GetTableInfoResponse{
		Destination: req.Destination,
	}, nil
}

// HealthCheck implements the gRPC HealthCheck method
func (w *Writer) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	queueDepth := uint64(len(w.queue))
	status := pb.HealthCheckResponse_STATUS_HEALTHY

	if queueDepth > uint64(w.cfg.QueueSize)*8/10 {
		status = pb.HealthCheckResponse_STATUS_DEGRADED
	}

	stats := w.metrics.GetStats()

	return &pb.HealthCheckResponse{
		Status:          status,
		Message:         "operational",
		QueueDepth:      queueDepth,
		WritesPerSecond: stats.WritesPerSecond,
		TotalWrites:     stats.TotalWrites,
		TotalErrors:     stats.TotalErrors,
	}, nil
}

// writerLoop is the main writer goroutine (single writer constraint)
func (w *Writer) writerLoop() {
	defer w.wg.Done()

	ticker := time.NewTicker(time.Duration(w.cfg.FlushIntervalMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case req := <-w.queue:
			// Process write request
			resp := w.processWrite(req.req)
			req.respChan <- resp
			close(req.respChan)

		case <-ticker.C:
			// Periodic flush (if buffering is implemented)
			// For v1, we write immediately, so this is a no-op

		case <-w.ctx.Done():
			// Drain queue before shutting down
			log.Println("Draining write queue before shutdown...")
			w.drainQueue()
			return
		}
	}
}

// processWrite handles a single write request
func (w *Writer) processWrite(req *pb.WriteBatchRequest) *writeResponse {
	// TODO: Implement actual write logic
	// For now, return a mock success response

	log.Printf("Processing write: database=%s, table=%s, batch_id=%s",
		req.Destination.Database, req.Destination.Table, req.BatchId)

	// Validate destination
	if req.Destination == nil || req.Destination.Database == "" || req.Destination.Table == "" {
		return &writeResponse{
			resp: nil,
			err:  fmt.Errorf("invalid destination"),
		}
	}

	// Validate payload
	if len(req.Payload) == 0 {
		return &writeResponse{
			resp: nil,
			err:  fmt.Errorf("empty payload"),
		}
	}

	// TODO: Parse Arrow IPC / Parquet payload
	// TODO: Insert into DuckDB via catalog manager
	// TODO: Return actual row count and file URIs

	// Mock response for now
	return &writeResponse{
		resp: &pb.WriteBatchResponse{
			Status:      pb.WriteBatchResponse_STATUS_OK,
			Message:     "write successful",
			RowsWritten: 0, // TODO: actual row count
			BatchId:     req.BatchId,
		},
		err: nil,
	}
}

// drainQueue processes remaining write requests
func (w *Writer) drainQueue() {
	for {
		select {
		case req := <-w.queue:
			resp := w.processWrite(req.req)
			req.respChan <- resp
			close(req.respChan)
		default:
			log.Println("Queue drained")
			return
		}
	}
}

// Close gracefully shuts down the writer
func (w *Writer) Close() {
	log.Println("Closing lake writer...")
	w.cancel()
	w.wg.Wait()
	close(w.queue)
	log.Println("Lake writer closed")
}
