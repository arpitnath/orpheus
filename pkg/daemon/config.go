package daemon

import (
	"fmt"
	"os"
)

// DaemonConfig configures the daemon server's listening modes.
// The daemon can listen on Unix socket, TCP, or both simultaneously.
type DaemonConfig struct {
	// Unix socket configuration (for local development)
	UnixSocket UnixSocketConfig `yaml:"unix_socket"`

	// TCP configuration (for network access)
	TCP TCPConfig `yaml:"tcp"`
}

// UnixSocketConfig configures Unix socket listener.
type UnixSocketConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

// TCPConfig configures TCP listener with optional TLS.
type TCPConfig struct {
	Enabled bool      `yaml:"enabled"`
	Bind    string    `yaml:"bind"` // e.g., "0.0.0.0:8080" or "127.0.0.1:8080"
	TLS     TLSConfig `yaml:"tls"`
}

// TLSConfig configures TLS/HTTPS support.
type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// Validate checks that at least one listener mode is enabled.
func (c *DaemonConfig) Validate() error {
	if !c.UnixSocket.Enabled && !c.TCP.Enabled {
		return fmt.Errorf("at least one listener mode must be enabled (unix_socket or tcp)")
	}

	// Validate Unix socket config
	if c.UnixSocket.Enabled {
		if c.UnixSocket.Path == "" {
			return fmt.Errorf("unix_socket.path is required when unix_socket.enabled=true")
		}
	}

	// Validate TCP config
	if c.TCP.Enabled {
		if c.TCP.Bind == "" {
			return fmt.Errorf("tcp.bind is required when tcp.enabled=true")
		}

		// Validate TLS config if enabled
		if c.TCP.TLS.Enabled {
			if c.TCP.TLS.CertFile == "" {
				return fmt.Errorf("tcp.tls.cert_file is required when tcp.tls.enabled=true")
			}
			if c.TCP.TLS.KeyFile == "" {
				return fmt.Errorf("tcp.tls.key_file is required when tcp.tls.enabled=true")
			}

			// Check cert files exist
			if _, err := os.Stat(c.TCP.TLS.CertFile); os.IsNotExist(err) {
				return fmt.Errorf("tls cert file not found: %s", c.TCP.TLS.CertFile)
			}
			if _, err := os.Stat(c.TCP.TLS.KeyFile); os.IsNotExist(err) {
				return fmt.Errorf("tls key file not found: %s", c.TCP.TLS.KeyFile)
			}
		}
	}

	return nil
}

// DefaultDaemonConfig returns a config with sensible defaults.
// Unix socket enabled (for backwards compatibility), TCP disabled.
func DefaultDaemonConfig() *DaemonConfig {
	return &DaemonConfig{
		UnixSocket: UnixSocketConfig{
			Enabled: true,
			Path:    defaultUnixSocketPath(),
		},
		TCP: TCPConfig{
			Enabled: false,
			Bind:    "127.0.0.1:8080", // Localhost only when enabled
			TLS: TLSConfig{
				Enabled: false,
			},
		},
	}
}

// defaultUnixSocketPath returns the default Unix socket path based on OS.
func defaultUnixSocketPath() string {
	// On macOS (inside Lima VM) or Linux
	return "/var/run/agentscale.sock"
}
