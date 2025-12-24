// Package main provides the agentscale-daemon binary.
// The daemon listens on a Unix socket and executes agents via runc.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"syscall"
	"time"

	"agentscale/pkg/auth"
	"agentscale/pkg/daemon"
)

const version = "0.1.0"

func main() {
	// Check for subcommands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "create-key":
			runCreateKey()
			return
		case "list-keys":
			runListKeys()
			return
		case "revoke-key":
			runRevokeKey()
			return
		case "serve", "start", "run":
			// Explicit serve command - continue to server logic
			os.Args = append(os.Args[:1], os.Args[2:]...) // Remove subcommand from args
		case "--help", "-h", "help":
			printHelp()
			return
		case "--version", "-v", "version":
			fmt.Printf("agentscale-daemon %s\n", version)
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

	// Create server
	server, err := daemon.NewServer(config, version)
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

func getAuthStore() (*auth.Store, error) {
	// Determine database path
	dbPath := "/var/lib/agentscale/keys.db"
	if _, err := os.Stat("/var/lib/agentscale"); os.IsNotExist(err) {
		home, _ := os.UserHomeDir()
		dbPath = filepath.Join(home, ".agentscale", "keys.db")
	}

	return auth.NewStore(dbPath)
}

func runCreateKey() {
	fs := flag.NewFlagSet("create-key", flag.ExitOnError)
	name := fs.String("name", "", "Name for this API key (required)")
	rpm := fs.Int("rpm", 100, "Requests per minute limit")
	fs.Parse(os.Args[2:])

	if *name == "" {
		fmt.Println("Error: --name is required")
		fmt.Println("Usage: agentscale-daemon create-key --name <name> [--rpm <limit>]")
		os.Exit(1)
	}

	store, err := getAuthStore()
	if err != nil {
		log.Fatalf("Failed to open auth store: %v", err)
	}
	defer store.Close()

	key, err := store.CreateKey(*name, *rpm)
	if err != nil {
		log.Fatalf("Failed to create key: %v", err)
	}

	fmt.Println("")
	fmt.Println("✓ API Key Created Successfully")
	fmt.Println("")
	fmt.Printf("  Name:       %s\n", key.Name)
	fmt.Printf("  Key:        %s\n", key.Key)
	fmt.Printf("  Rate Limit: %d requests/minute\n", key.RequestsPerMinute)
	fmt.Println("")
	fmt.Println("Share this key securely with the user.")
	fmt.Println("")
}

func runListKeys() {
	store, err := getAuthStore()
	if err != nil {
		log.Fatalf("Failed to open auth store: %v", err)
	}
	defer store.Close()

	keys, err := store.ListKeys()
	if err != nil {
		log.Fatalf("Failed to list keys: %v", err)
	}

	if len(keys) == 0 {
		fmt.Println("No API keys found.")
		fmt.Println("")
		fmt.Println("Create a key with:")
		fmt.Println("  agentscale-daemon create-key --name <name>")
		return
	}

	fmt.Println("")
	fmt.Printf("%-20s %-50s %-8s %-20s\n", "NAME", "KEY", "ACTIVE", "RATE LIMIT")
	fmt.Println(strings.Repeat("-", 100))

	for _, key := range keys {
		active := "Yes"
		if !key.IsActive {
			active = "No"
		}

		keyDisplay := key.Key
		if len(keyDisplay) > 50 {
			keyDisplay = keyDisplay[:47] + "..."
		}

		fmt.Printf("%-20s %-50s %-8s %-20s\n",
			key.Name,
			keyDisplay,
			active,
			fmt.Sprintf("%d req/min", key.RequestsPerMinute),
		)
	}

	fmt.Println("")
	fmt.Printf("Total: %d key(s)\n", len(keys))
	fmt.Println("")
}

func runRevokeKey() {
	if len(os.Args) < 3 {
		fmt.Println("Error: API key required")
		fmt.Println("Usage: agentscale-daemon revoke-key <key>")
		os.Exit(1)
	}

	key := os.Args[2]

	store, err := getAuthStore()
	if err != nil {
		log.Fatalf("Failed to open auth store: %v", err)
	}
	defer store.Close()

	if err := store.RevokeKey(key); err != nil {
		log.Fatalf("Failed to revoke key: %v", err)
	}

	fmt.Println("")
	fmt.Printf("✓ API key revoked: %s\n", key)
	fmt.Println("")
}

func printHelp() {
	fmt.Printf("agentscale-daemon %s\n\n", version)
	fmt.Println("Usage:")
	fmt.Println("  agentscale-daemon [serve] [flags]    Run the daemon server")
	fmt.Println("  agentscale-daemon create-key [flags] Create a new API key")
	fmt.Println("  agentscale-daemon list-keys          List all API keys")
	fmt.Println("  agentscale-daemon revoke-key <key>   Revoke an API key")
	fmt.Println("")
	fmt.Println("Server Flags:")
	fmt.Println("  --socket <path>        Unix socket path (default: OS-specific)")
	fmt.Println("  --tcp-bind <addr>      TCP bind address (e.g., :8080)")
	fmt.Println("  --tls-cert <file>      TLS certificate file")
	fmt.Println("  --tls-key <file>       TLS key file")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  # Start on Unix socket (default)")
	fmt.Println("  agentscale-daemon")
	fmt.Println("")
	fmt.Println("  # Start on TCP")
	fmt.Println("  agentscale-daemon --tcp-bind :8080")
	fmt.Println("")
	fmt.Println("  # Start on both")
	fmt.Println("  agentscale-daemon --socket /tmp/test.sock --tcp-bind :8080")
	fmt.Println("")
	fmt.Println("  # Create API key")
	fmt.Println("  agentscale-daemon create-key --name developer1 --rpm 100")
	fmt.Println("")
}
