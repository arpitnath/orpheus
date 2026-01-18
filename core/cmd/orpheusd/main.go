// Package main provides the orpheusd binary.
// The daemon listens on a Unix socket and executes agents via runc.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	goruntime "runtime"
	"syscall"
	"time"

	"orpheus/daemon/pkg/daemon"
	"orpheus/daemon/pkg/execlog"
)

const version = "0.1.0"

func main() {
	// Check for subcommands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "serve", "start", "run":
			// Explicit serve command - continue to server logic
			os.Args = append(os.Args[:1], os.Args[2:]...) // Remove subcommand from args
		case "--help", "-h", "help":
			printHelp()
			return
		case "--version", "-v", "version":
			fmt.Printf("orpheusd %s\n", version)
			return
		}
	}

	// Default: Run server
	runServer()
}

func runServer() {
	// Flags
	socketPath := flag.String("socket", defaultSocket(), "Unix socket path")
	tcpBind := flag.String("tcp-bind", "", "TCP bind address (e.g., :8080 or 0.0.0.0:8080)")
	tlsCert := flag.String("tls-cert", "", "TLS certificate file")
	tlsKey := flag.String("tls-key", "", "TLS key file")
	flag.Parse()

	// Build config from flags
	config := &daemon.DaemonConfig{
		UnixSocket: daemon.UnixSocketConfig{
			Enabled: *socketPath != "",
			Path:    *socketPath,
		},
		TCP: daemon.TCPConfig{
			Enabled: *tcpBind != "",
			Bind:    *tcpBind,
			TLS: daemon.TLSConfig{
				Enabled:  *tlsCert != "" && *tlsKey != "",
				CertFile: *tlsCert,
				KeyFile:  *tlsKey,
			},
		},
	}

	// If neither Unix socket nor TCP specified, use Unix socket default
	if !config.UnixSocket.Enabled && !config.TCP.Enabled {
		config.UnixSocket.Enabled = true
		config.UnixSocket.Path = defaultSocket()
	}

	// Initialize ExecLog directory
	// Use /tmp for Lima VM compatibility (read-write filesystem)
	execlogDir := "/tmp/orpheus-execlog"
	if err := os.MkdirAll(execlogDir, 0755); err != nil {
		log.Fatalf("Failed to create execlog directory: %v", err)
	}

	// Run crash recovery
	log.Printf("Running crash recovery...")
	crashed, err := execlog.DetectAndMarkCrashed(execlogDir)
	if err != nil {
		log.Printf("Warning: Crash recovery failed: %v", err)
	} else {
		totalCrashed := 0
		for agentName, requests := range crashed {
			totalCrashed += len(requests)
			log.Printf("  %s: %d crashed requests", agentName, len(requests))
		}
		if totalCrashed > 0 {
			log.Printf("Marked %d total requests as CRASHED", totalCrashed)
			log.Printf("Query with: orpheus execlog crashed")
		}
	}

	// Create server
	server, err := daemon.NewServer(config, version, execlogDir)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

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

	log.Printf("Starting orpheusd %s on %s", version, *socketPath)
	if err := server.ListenAndServe(ctx); err != nil && ctx.Err() == nil {
		log.Fatalf("Server error: %v", err)
	}
	log.Println("Daemon stopped")
}

func defaultSocket() string {
	if goruntime.GOOS == "darwin" {
		// On macOS, daemon runs inside Lima VM
		// For local testing, use /tmp
		return "/tmp/orpheus.sock"
	}
	return "/var/run/orpheus.sock"
}

func printHelp() {
	fmt.Printf("orpheusd %s\n\n", version)
	fmt.Println("Usage:")
	fmt.Println("  orpheusd [serve] [flags]    Run the daemon server")
	fmt.Println("")
	fmt.Println("Server Flags:")
	fmt.Println("  --socket <path>        Unix socket path (default: OS-specific)")
	fmt.Println("  --tcp-bind <addr>      TCP bind address (e.g., :8080)")
	fmt.Println("  --tls-cert <file>      TLS certificate file")
	fmt.Println("  --tls-key <file>       TLS key file")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  # Start on Unix socket (default)")
	fmt.Println("  orpheusd")
	fmt.Println("")
	fmt.Println("  # Start on TCP")
	fmt.Println("  orpheusd --tcp-bind :8080")
	fmt.Println("")
	fmt.Println("  # Start on both")
	fmt.Println("  orpheusd --socket /tmp/test.sock --tcp-bind :8080")
	fmt.Println("")
}
