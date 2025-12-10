package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/withObsrvr/obsrvr-lake-writer/go/internal/catalog"
	"github.com/withObsrvr/obsrvr-lake-writer/go/internal/metrics"
	"github.com/withObsrvr/obsrvr-lake-writer/go/internal/writer"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v3"

	pb "github.com/withObsrvr/obsrvr-lake-writer/gen/lake_writer"
)

const (
	version = "0.1.0"
)

// Config represents the service configuration
type Config struct {
	Service struct {
		Name          string `yaml:"name"`
		ListenAddress string `yaml:"listen_address"`
		HealthPort    int    `yaml:"health_port"`
	} `yaml:"service"`

	Catalog struct {
		CatalogPath       string `yaml:"catalog_path"`
		AWSAccessKeyID    string `yaml:"aws_access_key_id"`
		AWSSecretKey      string `yaml:"aws_secret_access_key"`
		AWSRegion         string `yaml:"aws_region"`
		AWSEndpoint       string `yaml:"aws_endpoint"`
	} `yaml:"catalog"`

	Databases []struct {
		Name     string `yaml:"name"`
		DataPath string `yaml:"data_path"`
	} `yaml:"databases"`

	Writer struct {
		QueueSize       int `yaml:"queue_size"`
		FlushIntervalMs int `yaml:"flush_interval_ms"`
		MaxBatchSize    int `yaml:"max_batch_size"`
	} `yaml:"writer"`

	Logging struct {
		Level  string `yaml:"level"`
		Format string `yaml:"format"`
	} `yaml:"logging"`
}

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config/local.yaml", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize metrics
	metricsCollector := metrics.NewCollector()

	// Convert database configs
	databases := make([]catalog.DatabaseConfig, len(cfg.Databases))
	for i, db := range cfg.Databases {
		databases[i] = catalog.DatabaseConfig{
			Name:     db.Name,
			DataPath: db.DataPath,
		}
	}

	// Initialize catalog manager
	catalogMgr, err := catalog.NewManager(ctx, catalog.ManagerConfig{
		CatalogPath:    cfg.Catalog.CatalogPath,
		AWSAccessKeyID: cfg.Catalog.AWSAccessKeyID,
		AWSSecretKey:   cfg.Catalog.AWSSecretKey,
		AWSRegion:      cfg.Catalog.AWSRegion,
		AWSEndpoint:    cfg.Catalog.AWSEndpoint,
		Databases:      databases,
	})
	if err != nil {
		log.Fatalf("Failed to initialize catalog: %v", err)
	}
	defer catalogMgr.Close()

	// Initialize lake writer
	lakeWriter, err := writer.New(ctx, writer.Config{
		QueueSize:       cfg.Writer.QueueSize,
		FlushIntervalMs: cfg.Writer.FlushIntervalMs,
		MaxBatchSize:    cfg.Writer.MaxBatchSize,
		Catalog:         catalogMgr,
		Metrics:         metricsCollector,
	})
	if err != nil {
		log.Fatalf("Failed to initialize writer: %v", err)
	}
	defer lakeWriter.Close()

	// Start gRPC server
	grpcServer := grpc.NewServer()
	pb.RegisterLakeWriterServer(grpcServer, lakeWriter)

	listener, err := net.Listen("tcp", cfg.Service.ListenAddress)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Start health/metrics HTTP server
	go startHealthServer(cfg.Service.HealthPort, metricsCollector)

	// Start gRPC server in goroutine
	go func() {
		log.Printf("Starting Lake Writer v%s on %s", version, cfg.Service.ListenAddress)
		if err := grpcServer.Serve(listener); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	log.Println("Shutting down gracefully...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop gRPC server
	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
		log.Println("gRPC server stopped")
	case <-shutdownCtx.Done():
		log.Println("Shutdown timeout, forcing stop")
		grpcServer.Stop()
	}

	log.Println("Shutdown complete")
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	// Expand environment variables
	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Set defaults
	if cfg.Service.Name == "" {
		cfg.Service.Name = "lake-writer"
	}
	if cfg.Service.ListenAddress == "" {
		cfg.Service.ListenAddress = ":50099"
	}
	if cfg.Service.HealthPort == 0 {
		cfg.Service.HealthPort = 8088
	}
	if cfg.Writer.QueueSize == 0 {
		cfg.Writer.QueueSize = 1000
	}
	if cfg.Writer.FlushIntervalMs == 0 {
		cfg.Writer.FlushIntervalMs = 100
	}
	if cfg.Writer.MaxBatchSize == 0 {
		cfg.Writer.MaxBatchSize = 1000
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}

	return &cfg, nil
}

func startHealthServer(port int, metrics *metrics.Collector) {
	mux := http.NewServeMux()

	// Health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"healthy","service":"lake-writer","version":"%s"}`, version)
	})

	// Metrics endpoint
	mux.Handle("/metrics", metrics.Handler())

	addr := fmt.Sprintf(":%d", port)
	log.Printf("Starting health/metrics server on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Health server failed: %v", err)
	}
}
