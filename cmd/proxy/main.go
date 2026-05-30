package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"prism/internal/api"
	"prism/internal/ca"
	"prism/internal/config"
	"prism/internal/proxy"
	"prism/internal/storage"
	"prism/web"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger, err := config.InitLogger(&cfg.Logging)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Starting Prism",
		zap.String("proxy_addr", cfg.Proxy.Listen),
		zap.String("api_addr", cfg.API.Listen),
	)

	// Initialize storage
	db, err := storage.New(cfg.Storage.Path)
	if err != nil {
		logger.Fatal("Failed to initialize storage", zap.Error(err))
	}
	defer db.Close()

	// Initialize CA manager
	caManager, err := ca.NewManager(
		cfg.TLS.CACertPath,
		cfg.TLS.CAKeyPath,
		cfg.TLS.CertCacheDir,
	)
	if err != nil {
		logger.Fatal("Failed to initialize CA manager", zap.Error(err))
	}

	logger.Info("CA certificate initialized",
		zap.String("cert_path", cfg.TLS.CACertPath),
	)

	// Create proxy server
	proxyServer := proxy.NewServer(db, caManager, logger, cfg)

	// Start background pruner for data cleanup
	// Keep 100 records per (host, method, path) for enabled hosts
	// Keep 1000 most recent records for non-enabled hosts
	proxyServer.StartPruner(time.Minute, 100, 1000)

	// Create API server with embedded web UI
	apiServer := api.NewServer(db, caManager, logger, cfg, web.DistFS())

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start proxy server
	go func() {
		logger.Info("Proxy server starting", zap.String("addr", cfg.Proxy.Listen))
		if err := proxyServer.ListenAndServe(cfg.Proxy.Listen); err != nil && err != http.ErrServerClosed {
			logger.Error("Proxy server error", zap.Error(err))
			cancel()
		}
	}()

	// Start API server
	go func() {
		logger.Info("API server starting", zap.String("addr", cfg.API.Listen))
		if err := apiServer.ListenAndServe(cfg.API.Listen); err != nil && err != http.ErrServerClosed {
			logger.Error("API server error", zap.Error(err))
			cancel()
		}
	}()

	// Wait for shutdown signal
	select {
	case sig := <-sigChan:
		logger.Info("Received shutdown signal", zap.String("signal", sig.String()))
	case <-ctx.Done():
	}

	// Graceful shutdown
	logger.Info("Shutting down servers...")
	proxyServer.Shutdown(ctx)
	apiServer.Shutdown(ctx)
	logger.Info("Shutdown complete")
}
