package downloader

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Downloader manages runtime environment downloads and caching
type Downloader struct {
	cacheDir string
	mu       sync.Mutex
	locks    map[string]*sync.Mutex
}

// Config contains downloader configuration
type Config struct {
	CacheDir string
}

// New creates a new Downloader instance
func New(cfg Config) *Downloader {
	return &Downloader{
		cacheDir: cfg.CacheDir,
		locks:    make(map[string]*sync.Mutex),
	}
}

// EnsureRuntime ensures the specified runtime is available
// Returns the path to the runtime rootfs
func (d *Downloader) EnsureRuntime(ctx context.Context, runtimeSpec string) (string, error) {
	// Parse runtime spec (e.g., "python3.10" -> "python", "3.10")
	language, version, err := parseRuntimeSpec(runtimeSpec)
	if err != nil {
		return "", fmt.Errorf("invalid runtime spec %q: %w", runtimeSpec, err)
	}

	// Get platform and architecture
	platform := runtime.GOOS
	arch := runtime.GOARCH

	// Build runtime ID
	runtimeID := fmt.Sprintf("%s-%s-%s-%s", language, version, platform, arch)
	runtimePath := filepath.Join(d.cacheDir, runtimeID)

	// Check if runtime already exists
	if d.runtimeExists(runtimePath) {
		log.Printf("Runtime %s already cached at %s", runtimeID, runtimePath)
		return runtimePath, nil
	}

	// Acquire lock for this runtime to prevent concurrent builds
	d.mu.Lock()
	if d.locks[runtimeID] == nil {
		d.locks[runtimeID] = &sync.Mutex{}
	}
	runtimeLock := d.locks[runtimeID]
	d.mu.Unlock()

	runtimeLock.Lock()
	defer runtimeLock.Unlock()

	// Check again after acquiring lock (another goroutine might have built it)
	if d.runtimeExists(runtimePath) {
		log.Printf("Runtime %s was built by another process", runtimeID)
		return runtimePath, nil
	}

	// Build the runtime
	log.Printf("Building runtime %s...", runtimeID)
	if err := d.buildRuntime(ctx, language, version, platform, arch, runtimePath); err != nil {
		return "", fmt.Errorf("build runtime: %w", err)
	}

	log.Printf("Runtime %s ready at %s", runtimeID, runtimePath)
	return runtimePath, nil
}

// runtimeExists checks if a runtime directory exists and is valid
func (d *Downloader) runtimeExists(runtimePath string) bool {
	// Check if directory exists
	if _, err := os.Stat(runtimePath); err != nil {
		return false
	}

	// Check for essential directories
	essentialDirs := []string{"usr", "bin"}
	for _, dir := range essentialDirs {
		if _, err := os.Stat(filepath.Join(runtimePath, dir)); err != nil {
			return false
		}
	}

	return true
}

// buildRuntime builds a runtime using Podman
func (d *Downloader) buildRuntime(ctx context.Context, language, version, platform, arch, runtimePath string) error {
	// Get containerfile path
	containerfile, err := getContainerfilePath(language, version)
	if err != nil {
		return err
	}

	// Check if containerfile exists
	if _, err := os.Stat(containerfile); err != nil {
		return fmt.Errorf("containerfile not found: %s", containerfile)
	}

	log.Printf("Using containerfile: %s", containerfile)

	// Create temporary build tag
	buildTag := fmt.Sprintf("orpheus-runtime-%s-%s-%d", language, version, time.Now().Unix())

	// Build image with Podman
	log.Printf("Running: podman build -f %s -t %s", containerfile, buildTag)
	buildCmd := exec.CommandContext(ctx, "podman", "build",
		"-f", containerfile,
		"-t", buildTag,
		"--platform", fmt.Sprintf("%s/%s", platform, normalizeArch(arch)),
	)
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr

	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("podman build failed: %w", err)
	}

	// Create temporary container
	log.Printf("Creating container from image %s", buildTag)
	createCmd := exec.CommandContext(ctx, "podman", "create", buildTag)
	containerID, err := createCmd.Output()
	if err != nil {
		return fmt.Errorf("podman create failed: %w", err)
	}
	containerIDStr := strings.TrimSpace(string(containerID))
	log.Printf("Container ID: %s", containerIDStr)

	// Ensure cleanup
	defer func() {
		log.Printf("Cleaning up container %s", containerIDStr)
		exec.Command("podman", "rm", containerIDStr).Run()
		log.Printf("Cleaning up image %s", buildTag)
		exec.Command("podman", "rmi", buildTag).Run()
	}()

	// Export container to tar
	log.Printf("Exporting container to rootfs...")
	tmpTar := filepath.Join(os.TempDir(), fmt.Sprintf("orpheus-runtime-%d.tar", time.Now().Unix()))
	defer os.Remove(tmpTar)

	exportCmd := exec.CommandContext(ctx, "podman", "export", "-o", tmpTar, containerIDStr)
	if err := exportCmd.Run(); err != nil {
		return fmt.Errorf("podman export failed: %w", err)
	}

	// Create runtime directory
	if err := os.MkdirAll(runtimePath, 0755); err != nil {
		return fmt.Errorf("create runtime directory: %w", err)
	}

	// Extract tar to runtime path
	log.Printf("Extracting rootfs to %s", runtimePath)
	extractCmd := exec.CommandContext(ctx, "tar", "-xf", tmpTar, "-C", runtimePath)
	if err := extractCmd.Run(); err != nil {
		os.RemoveAll(runtimePath) // Cleanup on failure
		return fmt.Errorf("tar extract failed: %w", err)
	}

	log.Printf("Runtime built successfully at %s", runtimePath)
	return nil
}

// parseRuntimeSpec parses a runtime specification like "python3.10" or "nodejs20"
func parseRuntimeSpec(spec string) (language, version string, err error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", "", fmt.Errorf("empty runtime spec")
	}

	// Handle python specs
	if strings.HasPrefix(spec, "python") {
		version = strings.TrimPrefix(spec, "python")
		if version == "" || version == "3" {
			version = "3.12" // Default to 3.12
		}
		return "python", version, nil
	}

	// Handle nodejs specs
	if strings.HasPrefix(spec, "nodejs") || strings.HasPrefix(spec, "node") {
		version = strings.TrimPrefix(strings.TrimPrefix(spec, "nodejs"), "node")
		if version == "" {
			version = "20" // Default to Node 20
		}
		return "nodejs", version, nil
	}

	return "", "", fmt.Errorf("unsupported runtime: %s", spec)
}

// getContainerfilePath returns the path to the containerfile for a runtime
// Structure: runtimes/{language}/{version}.Containerfile
func getContainerfilePath(language, version string) (string, error) {
	// Find the project root (assuming we're in core/pkg/runtime/downloader)
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	// Try to find runtimes directory
	projectRoot := cwd
	for i := 0; i < 5; i++ {
		runtimesDir := filepath.Join(projectRoot, "runtimes")
		if _, err := os.Stat(runtimesDir); err == nil {
			// Found it - build path: runtimes/{language}/{version}.Containerfile
			filename := fmt.Sprintf("%s.Containerfile", version)
			return filepath.Join(runtimesDir, language, filename), nil
		}
		projectRoot = filepath.Dir(projectRoot)
	}

	return "", fmt.Errorf("runtimes directory not found")
}

// normalizeArch converts Go arch to OCI platform arch
// OCI spec uses "amd64" and "arm64", not "x86_64" and "aarch64"
func normalizeArch(arch string) string {
	// Go's runtime.GOARCH already uses OCI-compliant names
	// No conversion needed - amd64 stays amd64, arm64 stays arm64
	return arch
}
