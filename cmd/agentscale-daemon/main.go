// Package main provides the agentscale-daemon binary.
// The daemon listens on a Unix socket and executes agents via runc.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	goruntime "runtime"
	"syscall"
	"time"

	"agentscale/pkg/daemon"
)

const version = "0.1.0"

func main() {
	socketPath := flag.String("socket", defaultSocket(), "Unix socket path")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		log.Printf("agentscale-daemon %s", version)
		os.Exit(0)
	}

	// Create server
	server := daemon.NewServer(*socketPath, version)

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("Received signal %v, shutting down...", sig)
		cancel()

		// Give server time to shutdown gracefully
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}
	}()

	log.Printf("Starting agentscale-daemon %s on %s", version, *socketPath)
	if err := server.ListenAndServe(ctx); err != nil && ctx.Err() == nil {
		log.Fatalf("Server error: %v", err)
	}
	log.Println("Daemon stopped")
}

func defaultSocket() string {
	if goruntime.GOOS == "darwin" {
		// On macOS, daemon runs inside Lima VM
		// For local testing, use /tmp
		return "/tmp/agentscale.sock"
	}
	return "/var/run/agentscale.sock"
}
