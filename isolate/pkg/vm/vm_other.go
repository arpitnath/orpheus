//go:build !darwin

package vm

import (
	"io"
)

// LinuxManager implements Manager interface for Linux (no VM needed)
type LinuxManager struct{}

// NewManager creates a new manager - on Linux, VM is not needed
func NewManager() Manager {
	return &LinuxManager{}
}

// IsAvailable returns false on Linux - we don't need VM, we have native namespaces
func (m *LinuxManager) IsAvailable() bool {
	return false
}

// EnsureResources is a no-op on Linux
func (m *LinuxManager) EnsureResources() error {
	return nil
}

// Create returns an error on Linux - VM not needed
func (m *LinuxManager) Create(config *Config) (VM, error) {
	return nil, ErrNotAvailable
}

// Get returns an error on Linux - VM not needed
func (m *LinuxManager) Get() (VM, error) {
	return nil, ErrNotAvailable
}

// NoopVM is a placeholder that should never be used on Linux
type NoopVM struct{}

func (v *NoopVM) Start() error                                                            { return ErrNotAvailable }
func (v *NoopVM) Stop() error                                                             { return ErrNotAvailable }
func (v *NoopVM) IsRunning() bool                                                         { return false }
func (v *NoopVM) Run(command string) (string, error)                                      { return "", ErrNotAvailable }
func (v *NoopVM) RunInteractive(command string, stdin io.Reader, stdout, stderr io.Writer) error { return ErrNotAvailable }
