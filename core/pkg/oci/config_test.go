package oci

import (
	"testing"

	"orpheus/daemon/pkg/config"
)

func TestGenerateSpec_Python(t *testing.T) {
	cfg := &config.AgentConfig{
		Name:        "test-agent",
		Runtime:     "python3",
		MemoryLimit: 512,
	}

	spec := GenerateSpec(cfg)

	if spec == nil {
		t.Fatal("GenerateSpec returned nil")
	}

	// Verify Python runtime args
	if len(spec.Process.Args) == 0 {
		t.Fatal("Process args should not be empty for Python runtime")
	}

	// Should contain python3 executable
	foundPython := false
	for _, arg := range spec.Process.Args {
		if arg == "/usr/local/bin/python3.10" || arg == "python3" {
			foundPython = true
			break
		}
	}

	if !foundPython {
		t.Errorf("Python runtime should have python3 in args, got: %v", spec.Process.Args)
	}
}

func TestGenerateSpec_MemoryLimit(t *testing.T) {
	cfg := &config.AgentConfig{
		Name:        "test-agent",
		Runtime:     "python3",
		MemoryLimit: 256, // 256MB
	}

	spec := GenerateSpec(cfg)

	// Verify memory limit set in cgroup resources
	if spec.Linux == nil || spec.Linux.Resources == nil || spec.Linux.Resources.Memory == nil {
		t.Fatal("Memory resources should be configured")
	}

	expectedBytes := int64(256 * 1024 * 1024) // 256MB in bytes
	if spec.Linux.Resources.Memory.Limit == nil {
		t.Fatal("Memory limit should be set")
	}

	if *spec.Linux.Resources.Memory.Limit != expectedBytes {
		t.Errorf("Expected memory limit %d bytes, got %d", expectedBytes, *spec.Linux.Resources.Memory.Limit)
	}
}

func TestGenerateSpec_PIDLimit(t *testing.T) {
	cfg := &config.AgentConfig{
		Name:    "test-agent",
		Runtime: "python3",
	}

	spec := GenerateSpec(cfg)

	// Verify PID limit (fork bomb protection)
	if spec.Linux == nil || spec.Linux.Resources == nil || spec.Linux.Resources.Pids == nil {
		t.Fatal("PID resources should be configured")
	}

	expectedLimit := int64(256)
	if spec.Linux.Resources.Pids.Limit != expectedLimit {
		t.Errorf("Expected PID limit %d, got %d", expectedLimit, spec.Linux.Resources.Pids.Limit)
	}
}

func TestGenerateSpec_Namespaces(t *testing.T) {
	cfg := &config.AgentConfig{
		Name:    "test-agent",
		Runtime: "python3",
	}

	spec := GenerateSpec(cfg)

	// Verify required namespaces
	requiredNamespaces := []string{"pid", "mount", "ipc", "uts"}

	if spec.Linux == nil || len(spec.Linux.Namespaces) == 0 {
		t.Fatal("Namespaces should be configured")
	}

	foundNamespaces := make(map[string]bool)
	for _, ns := range spec.Linux.Namespaces {
		foundNamespaces[ns.Type] = true
	}

	for _, required := range requiredNamespaces {
		if !foundNamespaces[required] {
			t.Errorf("Missing required namespace: %s", required)
		}
	}

	// Network namespace should NOT be present (agents share host network)
	if foundNamespaces["network"] {
		t.Error("Network namespace should not be isolated (agents need host network)")
	}
}

func TestGenerateSpec_NonRoot(t *testing.T) {
	cfg := &config.AgentConfig{
		Name:    "test-agent",
		Runtime: "python3",
	}

	spec := GenerateSpec(cfg)

	// Verify non-root user (uid 1000, gid 1000)
	if spec.Process.User.UID != 1000 {
		t.Errorf("Expected UID 1000 (non-root), got %d", spec.Process.User.UID)
	}

	if spec.Process.User.GID != 1000 {
		t.Errorf("Expected GID 1000 (non-root), got %d", spec.Process.User.GID)
	}
}

func TestGenerateSpec_Capabilities(t *testing.T) {
	cfg := &config.AgentConfig{
		Name:    "test-agent",
		Runtime: "python3",
	}

	spec := GenerateSpec(cfg)

	// Verify all capabilities dropped (security hardening)
	if spec.Process.Capabilities == nil {
		t.Fatal("Capabilities should be configured")
	}

	// Bounding set should be empty (all capabilities dropped)
	if len(spec.Process.Capabilities.Bounding) != 0 {
		t.Errorf("Bounding capabilities should be empty, got: %v", spec.Process.Capabilities.Bounding)
	}

	// Effective set should be empty
	if len(spec.Process.Capabilities.Effective) != 0 {
		t.Errorf("Effective capabilities should be empty, got: %v", spec.Process.Capabilities.Effective)
	}
}

func TestGenerateSpec_Version(t *testing.T) {
	cfg := &config.AgentConfig{
		Name:    "test-agent",
		Runtime: "python3",
	}

	spec := GenerateSpec(cfg)

	// Verify OCI version
	if spec.Version != OCIVersion {
		t.Errorf("Expected OCI version %s, got %s", OCIVersion, spec.Version)
	}
}
