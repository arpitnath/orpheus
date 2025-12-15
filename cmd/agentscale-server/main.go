package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"agentscale/pkg/config"
	"agentscale/pkg/server"
)

func main() {
	// Command-line flags
	agentDir := flag.String("agent", ".", "Path to agent directory")
	port := flag.String("port", "8080", "Server port")
	tier := flag.String("tier", "pro", "Scaling tier (free/pro/teams)")
	flag.Parse()

	// Validate tier
	validTiers := map[string]bool{"free": true, "pro": true, "teams": true}
	if !validTiers[*tier] {
		log.Fatalf("Invalid tier: %s (valid: free, pro, teams)", *tier)
	}

	// Load agent configuration
	cfg, err := config.Load(*agentDir)
	if err != nil {
		log.Fatalf("Failed to load agent config from %s: %v", *agentDir, err)
	}

	log.Printf("Loaded agent: %s", cfg.Name)

	// Create server
	addr := ":" + *port
	srv, err := server.New(cfg, addr, *tier)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Setup signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		log.Printf("Starting AgentScale server on %s (tier=%s)", addr, *tier)
		log.Printf("Endpoints:")
		log.Printf("  POST /invoke  - Execute agent")
		log.Printf("  GET  /health  - Health check")
		log.Printf("  GET  /stats   - Queue/pool statistics")

		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	sig := <-sigCh
	log.Printf("Received signal %v, shutting down...", sig)

	// Graceful shutdown with 30 second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Shutdown error: %v", err)
		os.Exit(1)
	}

	log.Printf("Server stopped")
}
