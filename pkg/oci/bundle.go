package oci

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"agentscale/pkg/config"
)

// DefaultBundleDir is the base directory for OCI bundles
const DefaultBundleDir = "/tmp/agentscale-bundles"

// BundleGenerator creates OCI bundles from agent configurations
type BundleGenerator struct {
	// BundleDir is the base directory for bundle storage
	BundleDir string
}

// NewBundleGenerator creates a new generator with default settings
func NewBundleGenerator() *BundleGenerator {
	return &BundleGenerator{
		BundleDir: DefaultBundleDir,
	}
}

// NewBundleGeneratorWithDir creates a generator with a custom bundle directory
func NewBundleGeneratorWithDir(bundleDir string) *BundleGenerator {
	return &BundleGenerator{
		BundleDir: bundleDir,
	}
}

// Bundle represents a generated OCI bundle ready for runc
type Bundle struct {
	// ID is the unique container identifier (UUID-based)
	ID string

	// Path is the bundle directory path
	Path string

	// ConfigPath is the path to config.json
	ConfigPath string

	// RootfsPath is the path to the rootfs directory (symlink)
	RootfsPath string
}

// GenerateBundle creates an OCI bundle from agent configuration.
//
// Parameters:
//   - cfg: Agent configuration containing memory, timeout, and runtime settings
//   - rootfsSource: Path to the agent image directory (will be symlinked to bundle/rootfs)
//   - containerID: Unique identifier for this container instance (should be UUID-based)
//
// Returns:
//   - *Bundle: The generated bundle with paths to config.json and rootfs
//   - error: If bundle creation fails
//
// The bundle structure:
//
//	{BundleDir}/{containerID}/
//	├── config.json          # OCI runtime specification
//	└── rootfs/              # Symlink to agent image
//	    → {rootfsSource}
func (g *BundleGenerator) GenerateBundle(cfg *config.AgentConfig, rootfsSource string, containerID string) (*Bundle, error) {
	// Create bundle directory
	bundlePath := filepath.Join(g.BundleDir, containerID)
	if err := os.MkdirAll(bundlePath, 0755); err != nil {
		return nil, fmt.Errorf("create bundle directory: %w", err)
	}

	// Generate OCI config.json
	spec := GenerateSpec(cfg)
	configPath := filepath.Join(bundlePath, "config.json")
	configData, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		os.RemoveAll(bundlePath)
		return nil, fmt.Errorf("marshal config.json: %w", err)
	}
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		os.RemoveAll(bundlePath)
		return nil, fmt.Errorf("write config.json: %w", err)
	}

	// Create rootfs symlink to agent image
	rootfsPath := filepath.Join(bundlePath, "rootfs")
	if err := os.Symlink(rootfsSource, rootfsPath); err != nil {
		os.RemoveAll(bundlePath)
		return nil, fmt.Errorf("create rootfs symlink: %w", err)
	}

	return &Bundle{
		ID:         containerID,
		Path:       bundlePath,
		ConfigPath: configPath,
		RootfsPath: rootfsPath,
	}, nil
}

// GenerateBundleWithOptions creates a bundle with custom spec options
func (g *BundleGenerator) GenerateBundleWithOptions(cfg *config.AgentConfig, rootfsSource string, containerID string, opts *SpecOptions) (*Bundle, error) {
	// Create bundle directory
	bundlePath := filepath.Join(g.BundleDir, containerID)
	if err := os.MkdirAll(bundlePath, 0755); err != nil {
		return nil, fmt.Errorf("create bundle directory: %w", err)
	}

	// Generate OCI config.json with options
	spec := GenerateSpecWithOptions(cfg, opts)
	configPath := filepath.Join(bundlePath, "config.json")
	configData, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		os.RemoveAll(bundlePath)
		return nil, fmt.Errorf("marshal config.json: %w", err)
	}
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		os.RemoveAll(bundlePath)
		return nil, fmt.Errorf("write config.json: %w", err)
	}

	// Create rootfs symlink to agent image
	rootfsPath := filepath.Join(bundlePath, "rootfs")
	if err := os.Symlink(rootfsSource, rootfsPath); err != nil {
		os.RemoveAll(bundlePath)
		return nil, fmt.Errorf("create rootfs symlink: %w", err)
	}

	return &Bundle{
		ID:         containerID,
		Path:       bundlePath,
		ConfigPath: configPath,
		RootfsPath: rootfsPath,
	}, nil
}

// Cleanup removes the bundle directory and all its contents
func (b *Bundle) Cleanup() error {
	if b.Path == "" {
		return nil
	}
	return os.RemoveAll(b.Path)
}

// EnsureBundleDir creates the bundle base directory if it doesn't exist
func (g *BundleGenerator) EnsureBundleDir() error {
	return os.MkdirAll(g.BundleDir, 0755)
}

// CleanupAllBundles removes all bundle directories (for cleanup on server shutdown)
func (g *BundleGenerator) CleanupAllBundles() error {
	return os.RemoveAll(g.BundleDir)
}

// ListBundles returns all bundle directories in the bundle dir
func (g *BundleGenerator) ListBundles() ([]string, error) {
	entries, err := os.ReadDir(g.BundleDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var bundles []string
	for _, entry := range entries {
		if entry.IsDir() {
			bundles = append(bundles, entry.Name())
		}
	}
	return bundles, nil
}
