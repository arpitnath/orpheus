package service

import (
	"context"
	"os/exec"
	"testing"
)

func TestGetExitCode_CleanExit(t *testing.T) {
	// err = nil means clean exit (code 0)
	code := getExitCode(nil)
	if code != 0 {
		t.Errorf("Expected 0 for nil error, got %d", code)
	}
}

func TestGetExitCode_ExitError(t *testing.T) {
	// Create a command that will fail with specific exit code
	cmd := exec.Command("sh", "-c", "exit 42")
	err := cmd.Run()

	code := getExitCode(err)
	if code != 42 {
		t.Errorf("Expected 42, got %d", code)
	}
}

func TestGetExitCode_ContextCanceled(t *testing.T) {
	// Context cancellation should return -1
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	code := getExitCode(ctx.Err())
	if code != -1 {
		t.Errorf("Expected -1 for context error, got %d", code)
	}
}

func TestIsOOMKill(t *testing.T) {
	tests := []struct {
		exitCode int
		expected bool
	}{
		{137, true},  // SIGKILL from OOM
		{0, false},   // Clean exit
		{1, false},   // Error exit
		{143, false}, // SIGTERM
		{9, false},   // Manual SIGKILL
	}

	for _, tt := range tests {
		result := isOOMKill(tt.exitCode)
		if result != tt.expected {
			t.Errorf("isOOMKill(%d) = %v, expected %v", tt.exitCode, result, tt.expected)
		}
	}
}

func TestIsManualTermination(t *testing.T) {
	tests := []struct {
		exitCode int
		expected bool
	}{
		{143, true},  // SIGTERM
		{0, false},   // Clean exit
		{1, false},   // Error exit
		{137, false}, // OOM SIGKILL
	}

	for _, tt := range tests {
		result := isManualTermination(tt.exitCode)
		if result != tt.expected {
			t.Errorf("isManualTermination(%d) = %v, expected %v", tt.exitCode, result, tt.expected)
		}
	}
}

func TestIsCleanExit(t *testing.T) {
	tests := []struct {
		exitCode int
		expected bool
	}{
		{0, true},    // Clean exit
		{1, false},   // Error exit
		{143, false}, // SIGTERM
		{137, false}, // OOM
	}

	for _, tt := range tests {
		result := isCleanExit(tt.exitCode)
		if result != tt.expected {
			t.Errorf("isCleanExit(%d) = %v, expected %v", tt.exitCode, result, tt.expected)
		}
	}
}

func TestShouldRestartForExitCode(t *testing.T) {
	tests := []struct {
		exitCode int
		expected bool
		reason   string
	}{
		{0, false, "clean exit should not restart"},
		{143, false, "SIGTERM should not restart"},
		{1, true, "error exit should restart"},
		{137, true, "OOM should restart (with extended backoff)"},
		{255, true, "other exits should restart"},
	}

	for _, tt := range tests {
		result := shouldRestartForExitCode(tt.exitCode)
		if result != tt.expected {
			t.Errorf("%s: shouldRestartForExitCode(%d) = %v, expected %v",
				tt.reason, tt.exitCode, result, tt.expected)
		}
	}
}
