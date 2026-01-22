package runtime

import (
	"testing"
)

func TestNewRunc(t *testing.T) {
	runc := NewRunc()

	if runc == nil {
		t.Fatal("NewRunc returned nil")
	}

	// BinaryPath should be set to something
	if runc.BinaryPath == "" {
		t.Error("BinaryPath should not be empty")
	}
}

func TestNewRuncWithPath(t *testing.T) {
	runc := NewRuncWithPath("/custom/path/runc")

	if runc == nil {
		t.Fatal("NewRuncWithPath returned nil")
	}

	if runc.BinaryPath != "/custom/path/runc" {
		t.Errorf("Expected BinaryPath=/custom/path/runc, got %s", runc.BinaryPath)
	}
}

func TestRuncRuntime_Available(t *testing.T) {
	// Test with a path that doesn't exist
	runc := NewRuncWithPath("/nonexistent/path/runc")

	if runc.Available() {
		t.Error("Should return false for non-existent path")
	}
}

func TestRuncRuntime_Available_RealPath(t *testing.T) {
	// Test with default runc detection
	runc := NewRunc()

	// This test passes/fails based on whether runc is installed
	// Just verify it doesn't panic
	_ = runc.Available()
}

func TestRuncAvailable(t *testing.T) {
	// Test the package-level helper function
	// Just verify it doesn't panic
	_ = RuncAvailable()
}

func TestIsInterfaceNil_NilInterface(t *testing.T) {
	var nilInterface interface{} = nil

	if !isInterfaceNil(nilInterface) {
		t.Error("Should return true for nil interface")
	}
}

func TestIsInterfaceNil_TypedNilPointer(t *testing.T) {
	var nilPtr *RuncRuntime = nil
	var i interface{} = nilPtr

	if !isInterfaceNil(i) {
		t.Error("Should return true for typed nil pointer in interface")
	}
}

func TestIsInterfaceNil_ValidPointer(t *testing.T) {
	runc := NewRunc()
	var i interface{} = runc

	if isInterfaceNil(i) {
		t.Error("Should return false for valid pointer")
	}
}

func TestIsInterfaceNil_NonPointer(t *testing.T) {
	var i interface{} = "hello"

	if isInterfaceNil(i) {
		t.Error("Should return false for non-nil non-pointer value")
	}
}

// Note: Testing actual Run/RunStreaming requires:
// 1. runc to be installed
// 2. Running as root
// 3. Valid OCI bundle
// These are covered by integration tests, not unit tests

func TestRuncRuntime_BinaryPathDefault(t *testing.T) {
	runc := NewRunc()

	// Should default to one of the expected paths or fallback
	validPaths := map[string]bool{
		"/usr/bin/runc":       true,
		"/usr/local/bin/runc": true,
		"runc":                true,
	}

	if !validPaths[runc.BinaryPath] {
		t.Logf("Unexpected BinaryPath: %s (ok if runc is in non-standard location)", runc.BinaryPath)
	}
}

// mockActivityMonitor for testing interface nil check
type mockActivityMonitor struct{}

func (m *mockActivityMonitor) MonitorReader(r interface{}, w interface{}) interface{} {
	return r
}

func TestIsInterfaceNil_WithMock(t *testing.T) {
	// Non-nil mock
	mock := &mockActivityMonitor{}
	if isInterfaceNil(mock) {
		t.Error("Should return false for valid mock")
	}

	// Typed nil mock
	var nilMock *mockActivityMonitor = nil
	var i interface{} = nilMock
	if !isInterfaceNil(i) {
		t.Error("Should return true for typed nil mock in interface")
	}
}
