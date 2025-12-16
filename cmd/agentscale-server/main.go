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
	configPath := flag.String("config", "./agentscale.yaml", "Path to server config file")
	flag.Parse()

	// Load server configuration
	serverCfg, err := config.LoadServerConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load server config from %s: %v", *configPath, err)
	}

	log.Printf("Loaded server config with %d agents:", len(serverCfg.Agents))
	for agentID, deployment := range serverCfg.Agents {
		log.Printf("  - %s: %s (min=%d, max=%d, queue=%d)",
			agentID,
			deployment.AgentConfig.Name,
			deployment.Scaling.MinWorkers,
			deployment.Scaling.MaxWorkers,
			deployment.Scaling.QueueSize)
	}

	// Create server
	srv, err := server.New(serverCfg)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Setup signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		log.Printf("Starting AgentScale multi-agent server on :%d", serverCfg.Server.Port)
		log.Printf("Endpoints:")
		log.Printf("  POST /invoke?agent=<id>  - Execute agent")
		log.Printf("  GET  /health             - List all agents")
		log.Printf("  GET  /stats              - All agent stats")
		log.Printf("  GET  /stats?agent=<id>   - Specific agent stats")

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
