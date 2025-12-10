package metrics

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Collector handles metrics collection and exposure
type Collector struct {
	// Counters
	totalWrites uint64
	totalErrors uint64
	totalBytes  uint64

	// Rate tracking
	mu               sync.Mutex
	recentWrites     []timestampedWrite
	writesPerSecond  uint64

	// Prometheus metrics
	writesTotal    *prometheus.CounterVec
	errorsTotal    *prometheus.CounterVec
	bytesTotal     *prometheus.CounterVec
	queueDepth     prometheus.Gauge
	writeDuration  *prometheus.HistogramVec
	batchSizeHist  prometheus.Histogram

	registry *prometheus.Registry
}

type timestampedWrite struct {
	timestamp time.Time
	count     uint64
}

// NewCollector creates a new metrics collector
func NewCollector() *Collector {
	registry := prometheus.NewRegistry()

	writesTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "lake_writer_batches_total",
			Help: "Total number of batches written",
		},
		[]string{"database", "table"},
	)

	errorsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "lake_writer_errors_total",
			Help: "Total number of write errors",
		},
		[]string{"database", "table"},
	)

	bytesTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "lake_writer_bytes_total",
			Help: "Total bytes written",
		},
		[]string{"database", "table"},
	)

	queueDepth := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "lake_writer_queue_depth",
			Help: "Current write queue depth",
		},
	)

	writeDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "lake_writer_write_duration_seconds",
			Help:    "Write operation duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"database", "table"},
	)

	batchSizeHist := prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "lake_writer_batch_size_rows",
			Help:    "Batch size in rows",
			Buckets: []float64{1, 10, 50, 100, 500, 1000, 5000, 10000},
		},
	)

	registry.MustRegister(writesTotal)
	registry.MustRegister(errorsTotal)
	registry.MustRegister(bytesTotal)
	registry.MustRegister(queueDepth)
	registry.MustRegister(writeDuration)
	registry.MustRegister(batchSizeHist)

	return &Collector{
		writesTotal:    writesTotal,
		errorsTotal:    errorsTotal,
		bytesTotal:     bytesTotal,
		queueDepth:     queueDepth,
		writeDuration:  writeDuration,
		batchSizeHist:  batchSizeHist,
		registry:       registry,
		recentWrites:   make([]timestampedWrite, 0, 60),
	}
}

// RecordWrite records a successful write
func (c *Collector) RecordWrite(database, table string, rowCount uint64) {
	atomic.AddUint64(&c.totalWrites, 1)

	c.writesTotal.WithLabelValues(database, table).Inc()
	c.batchSizeHist.Observe(float64(rowCount))

	// Track for rate calculation
	c.mu.Lock()
	c.recentWrites = append(c.recentWrites, timestampedWrite{
		timestamp: time.Now(),
		count:     1,
	})
	// Keep only last 60 seconds
	cutoff := time.Now().Add(-60 * time.Second)
	for i, w := range c.recentWrites {
		if w.timestamp.After(cutoff) {
			c.recentWrites = c.recentWrites[i:]
			break
		}
	}
	// Calculate writes/sec
	if len(c.recentWrites) > 0 {
		duration := time.Since(c.recentWrites[0].timestamp).Seconds()
		if duration > 0 {
			atomic.StoreUint64(&c.writesPerSecond, uint64(float64(len(c.recentWrites))/duration))
		}
	}
	c.mu.Unlock()
}

// RecordError records a write error
func (c *Collector) RecordError(database, table string) {
	atomic.AddUint64(&c.totalErrors, 1)
	c.errorsTotal.WithLabelValues(database, table).Inc()
}

// RecordBytes records bytes written
func (c *Collector) RecordBytes(database, table string, bytes uint64) {
	atomic.AddUint64(&c.totalBytes, bytes)
	c.bytesTotal.WithLabelValues(database, table).Add(float64(bytes))
}

// RecordDuration records write operation duration
func (c *Collector) RecordDuration(database, table string, duration time.Duration) {
	c.writeDuration.WithLabelValues(database, table).Observe(duration.Seconds())
}

// UpdateQueueDepth updates the queue depth gauge
func (c *Collector) UpdateQueueDepth(depth int) {
	c.queueDepth.Set(float64(depth))
}

// GetStats returns current statistics
func (c *Collector) GetStats() Stats {
	return Stats{
		TotalWrites:     atomic.LoadUint64(&c.totalWrites),
		TotalErrors:     atomic.LoadUint64(&c.totalErrors),
		TotalBytes:      atomic.LoadUint64(&c.totalBytes),
		WritesPerSecond: atomic.LoadUint64(&c.writesPerSecond),
	}
}

// Stats holds current metrics statistics
type Stats struct {
	TotalWrites     uint64
	TotalErrors     uint64
	TotalBytes      uint64
	WritesPerSecond uint64
}

// Handler returns the HTTP handler for Prometheus metrics
func (c *Collector) Handler() http.Handler {
	return promhttp.HandlerFor(c.registry, promhttp.HandlerOpts{})
}
