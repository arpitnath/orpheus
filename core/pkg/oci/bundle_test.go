package oci

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"orpheus/daemon/pkg/config"
)

func TestNewBundleGenerator(t *testing.T) {
	gen := NewBundleGenerator()

	if gen == nil {
		t.Fatal("NewBundleGenerator returned nil")
	}

	if gen.BundleDir != DefaultBundleDir {
		t.Errorf("Expected BundleDir=%s, got %s", DefaultBundleDir, gen.BundleDir)
	}
}

func TestNewBundleGeneratorWithDir(t *testing.T) {
	customDir := "/custom/bundle/dir"
	gen := NewBundleGeneratorWithDir(customDir)

	if gen == nil {
		t.Fatal("NewBundleGeneratorWithDir returned nil")
	}

	if gen.BundleDir != customDir {
		t.Errorf("Expected BundleDir=%s, got %s", customDir, gen.BundleDir)
	}
}

func TestGenerateBundle_Creates(t *testing.T) {
	tmpDir := t.TempDir()
	gen := NewBundleGeneratorWithDir(tmpDir)

	// Create a fake rootfs directory
	rootfsDir := filepath.Join(tmpDir, "fake-rootfs")
	if err := os.MkdirAll(rootfsDir, 0755); err != nil {
		t.Fatalf("Failed to create fake rootfs: %v", err)
	}

	cfg := &config.AgentConfig{
		Name:    "test-agent",
		Runtime: "python3",
		Memory:  256,
	}

	bundle, err := gen.GenerateBundle(cfg, rootfsDir, "container-123")
	if err != nil {
		t.Fatalf("GenerateBundle failed: %v", err)
	}
	defer bundle.Cleanup()

	// Verify bundle directory was created
	if _, err := os.Stat(bundle.Path); os.IsNotExist(err) {
		t.Error("Bundle path should exist")
	}

	// Verify config.json was created
	if _, err := os.Stat(bundle.ConfigPath); os.IsNotExist(err) {
		t.Error("ConfigPath should exist")
	}

	// Verify ID is set
	if bundle.ID != "container-123" {
		t.Errorf("Expected ID=container-123, got %s", bundle.ID)
	}
}

func TestGenerateBundle_ConfigJSON(t *testing.T) {
	tmpDir := t.TempDir()
	gen := NewBundleGeneratorWithDir(tmpDir)

	// Create a fake rootfs directory
	rootfsDir := filepath.Join(tmpDir, "fake-rootfs")
	if err := os.MkdirAll(rootfsDir, 0755); err != nil {
		t.Fatalf("Failed to create fake rootfs: %v", err)
	}

	cfg := &config.AgentConfig{
		Name:    "test-agent",
		Runtime: "python3",
		Memory:  512,
	}

	bundle, err := gen.GenerateBundle(cfg, rootfsDir, "container-456")
	if err != nil {
		t.Fatalf("GenerateBundle failed: %v", err)
	}
	defer bundle.Cleanup()

	// Read and parse config.json
	configData, err := os.ReadFile(bundle.ConfigPath)
	if err != nil {
		t.Fatalf("Failed to read config.json: %v", err)
	}

	var spec Spec
	if err := json.Unmarshal(configData, &spec); err != nil {
		t.Fatalf("Failed to parse config.json: %v", err)
	}

	// Verify OCI version is set
	if spec.Version == "" {
		t.Error("OCI version should be set")
	}

	// Verify process is configured
	if spec.Process == nil {
		t.Error("Process should be configured")
	}
}

func TestBundle_Cleanup(t *testing.T) {
	tmpDir := t.TempDir()
	gen := NewBundleGeneratorWithDir(tmpDir)

	// Create a fake rootfs directory
	rootfsDir := filepath.Join(tmpDir, "fake-rootfs")
	if err := os.MkdirAll(rootfsDir, 0755); err != nil {
		t.Fatalf("Failed to create fake rootfs: %v", err)
	}

	cfg := &config.AgentConfig{
		Name:    "test-agent",
		Runtime: "python3",
	}

	bundle, err := gen.GenerateBundle(cfg, rootfsDir, "cleanup-test")
	if err != nil {
		t.Fatalf("GenerateBundle failed: %v", err)
	}

	bundlePath := bundle.Path

	// Verify path exists before cleanup
	if _, err := os.Stat(bundlePath); os.IsNotExist(err) {
		t.Fatal("Bundle should exist before cleanup")
	}

	// Cleanup
	if err := bundle.Cleanup(); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Verify path removed after cleanup
	if _, err := os.Stat(bundlePath); !os.IsNotExist(err) {
		t.Error("Bundle should be removed after cleanup")
	}
}

func TestBundle_Cleanup_EmptyPath(t *testing.T) {
	bundle := &Bundle{Path: ""}

	// Should not error on empty path
	err := bundle.Cleanup()
	if err != nil {
		t.Errorf("Cleanup on empty path should not error, got: %v", err)
	}
}

func TestBundle_Cleanup_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	gen := NewBundleGeneratorWithDir(tmpDir)

	rootfsDir := filepath.Join(tmpDir, "fake-rootfs")
	os.MkdirAll(rootfsDir, 0755)

	cfg := &config.AgentConfig{Name: "test", Runtime: "python3"}
	bundle, _ := gen.GenerateBundle(cfg, rootfsDir, "idem-test")

	// First cleanup
	bundle.Cleanup()

	// Second cleanup should not error
	err := bundle.Cleanup()
	if err != nil {
		t.Errorf("Second cleanup should not error, got: %v", err)
	}
}

func TestEnsureBundleDir(t *testing.T) {
	tmpDir := t.TempDir()
	bundleDir := filepath.Join(tmpDir, "new-bundle-dir")

	gen := NewBundleGeneratorWithDir(bundleDir)

	// Should not exist yet
	if _, err := os.Stat(bundleDir); !os.IsNotExist(err) {
		t.Fatal("Bundle dir should not exist yet")
	}

	// Create it
	if err := gen.EnsureBundleDir(); err != nil {
		t.Fatalf("EnsureBundleDir failed: %v", err)
	}

	// Should exist now
	if _, err := os.Stat(bundleDir); os.IsNotExist(err) {
		t.Error("Bundle dir should exist after EnsureBundleDir")
	}
}

func TestCleanupAllBundles(t *testing.T) {
	tmpDir := t.TempDir()
	bundleDir := filepath.Join(tmpDir, "bundles")
	gen := NewBundleGeneratorWithDir(bundleDir)

	// Create rootfs outside bundle dir
	rootfsDir := filepath.Join(tmpDir, "fake-rootfs")
	os.MkdirAll(rootfsDir, 0755)

	cfg := &config.AgentConfig{Name: "test", Runtime: "python3"}

	// Create multiple bundles
	gen.GenerateBundle(cfg, rootfsDir, "bundle-1")
	gen.GenerateBundle(cfg, rootfsDir, "bundle-2")
	gen.GenerateBundle(cfg, rootfsDir, "bundle-3")

	// Verify bundles exist
	bundles, _ := gen.ListBundles()
	if len(bundles) != 3 {
		t.Fatalf("Expected 3 bundles, got %d", len(bundles))
	}

	// Cleanup all
	if err := gen.CleanupAllBundles(); err != nil {
		t.Fatalf("CleanupAllBundles failed: %v", err)
	}

	// Verify bundle dir removed
	if _, err := os.Stat(bundleDir); !os.IsNotExist(err) {
		t.Error("Bundle dir should be removed after CleanupAllBundles")
	}
}

func TestListBundles(t *testing.T) {
	tmpDir := t.TempDir()
	bundleDir := filepath.Join(tmpDir, "bundles")
	gen := NewBundleGeneratorWithDir(bundleDir)

	// Create rootfs outside bundle dir
	rootfsDir := filepath.Join(tmpDir, "fake-rootfs")
	os.MkdirAll(rootfsDir, 0755)

	cfg := &config.AgentConfig{Name: "test", Runtime: "python3"}

	// Create bundles
	b1, _ := gen.GenerateBundle(cfg, rootfsDir, "alpha")
	defer b1.Cleanup()
	b2, _ := gen.GenerateBundle(cfg, rootfsDir, "beta")
	defer b2.Cleanup()

	// List bundles
	bundles, err := gen.ListBundles()
	if err != nil {
		t.Fatalf("ListBundles failed: %v", err)
	}

	if len(bundles) != 2 {
		t.Errorf("Expected 2 bundles, got %d", len(bundles))
	}

	// Should contain both bundle IDs
	found := map[string]bool{}
	for _, b := range bundles {
		found[b] = true
	}

	if !found["alpha"] {
		t.Error("Should contain 'alpha' bundle")
	}
	if !found["beta"] {
		t.Error("Should contain 'beta' bundle")
	}
}

func TestListBundles_NonExistentDir(t *testing.T) {
	gen := NewBundleGeneratorWithDir("/nonexistent/path")

	bundles, err := gen.ListBundles()
	if err != nil {
		t.Errorf("ListBundles on non-existent dir should not error, got: %v", err)
	}

	if bundles != nil {
		t.Errorf("Expected nil bundles for non-existent dir, got: %v", bundles)
	}
}

func TestGenerateBundle_RootfsPath(t *testing.T) {
	tmpDir := t.TempDir()
	gen := NewBundleGeneratorWithDir(tmpDir)

	// Create a fake rootfs with relative-looking path
	rootfsDir := filepath.Join(tmpDir, "relative", "rootfs")
	os.MkdirAll(rootfsDir, 0755)

	cfg := &config.AgentConfig{Name: "test", Runtime: "python3"}

	bundle, err := gen.GenerateBundle(cfg, rootfsDir, "rootfs-test")
	if err != nil {
		t.Fatalf("GenerateBundle failed: %v", err)
	}
	defer bundle.Cleanup()

	// RootfsPath should be absolute
	if !filepath.IsAbs(bundle.RootfsPath) {
		t.Errorf("RootfsPath should be absolute, got: %s", bundle.RootfsPath)
	}
}

func TestGenerateBundleWithOptions(t *testing.T) {
	tmpDir := t.TempDir()
	gen := NewBundleGeneratorWithDir(tmpDir)

	rootfsDir := filepath.Join(tmpDir, "fake-rootfs")
	os.MkdirAll(rootfsDir, 0755)

	cfg := &config.AgentConfig{
		Name:    "test-agent",
		Runtime: "python3",
		Memory:  256,
	}

	opts := &SpecOptions{
		// Custom options can be added here
	}

	bundle, err := gen.GenerateBundleWithOptions(cfg, rootfsDir, "opts-test", opts)
	if err != nil {
		t.Fatalf("GenerateBundleWithOptions failed: %v", err)
	}
	defer bundle.Cleanup()

	// Verify bundle created
	if _, err := os.Stat(bundle.Path); os.IsNotExist(err) {
		t.Error("Bundle should exist")
	}
}

func TestGenerateBundleWithOptions_NilOpts(t *testing.T) {
	tmpDir := t.TempDir()
	gen := NewBundleGeneratorWithDir(tmpDir)

	rootfsDir := filepath.Join(tmpDir, "fake-rootfs")
	os.MkdirAll(rootfsDir, 0755)

	cfg := &config.AgentConfig{Name: "test", Runtime: "python3"}

	// nil options should work
	bundle, err := gen.GenerateBundleWithOptions(cfg, rootfsDir, "nil-opts", nil)
	if err != nil {
		t.Fatalf("GenerateBundleWithOptions with nil opts failed: %v", err)
	}
	defer bundle.Cleanup()

	if bundle.ID != "nil-opts" {
		t.Errorf("Expected ID=nil-opts, got %s", bundle.ID)
	}
}
