// Package lima provides Lima VM management for macOS.
// Lima runs a lightweight Linux VM using Apple's Virtualization.framework,
// allowing runc-based container execution on macOS.
package lima

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DefaultInstanceName is the default Lima VM instance name
const DefaultInstanceName = "agentscale"

// VMStatus represents the status of a Lima VM
type VMStatus string

const (
	StatusRunning VMStatus = "Running"
	StatusStopped VMStatus = "Stopped"
	StatusUnknown VMStatus = "Unknown"
)

// LimaInstance represents a Lima VM instance from limactl list --json
type LimaInstance struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Dir    string `json:"dir"`
	Arch   string `json:"arch"`
	CPUs   int    `json:"cpus"`
	Memory int64  `json:"memory"`
	Disk   int64  `json:"disk"`
}

// Manager manages Lima VM lifecycle
type Manager struct {
	InstanceName string
	TemplatePath string // Path to agentscale.yaml template
}

// NewManager creates a new Lima manager with default settings
func NewManager() *Manager {
	return &Manager{
		InstanceName: DefaultInstanceName,
	}
}

// NewManagerWithTemplate creates a new Lima manager with a custom template path
func NewManagerWithTemplate(templatePath string) *Manager {
	return &Manager{
		InstanceName: DefaultInstanceName,
		TemplatePath: templatePath,
	}
}

// Start starts the Lima VM.
// If the VM doesn't exist and TemplatePath is set, it creates and starts it.
// Uses --tty=false for non-interactive operation.
func (m *Manager) Start() error {
	// Check if instance exists
	status, err := m.Status()
	if err != nil {
		// Instance doesn't exist - create it if we have a template
		if m.TemplatePath != "" {
			return m.create()
		}
		return fmt.Errorf("lima instance '%s' not found and no template provided", m.InstanceName)
	}

	// Already running
	if status == StatusRunning {
		return nil
	}

	// Start existing instance
	cmd := exec.Command("limactl", "start", "--tty=false", m.InstanceName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// create creates a new Lima VM from the template
func (m *Manager) create() error {
	if m.TemplatePath == "" {
		return fmt.Errorf("no template path provided")
	}

	// Check template exists
	if _, err := os.Stat(m.TemplatePath); os.IsNotExist(err) {
		return fmt.Errorf("template not found: %s", m.TemplatePath)
	}

	cmd := exec.Command("limactl", "start", "--tty=false", "--name="+m.InstanceName, m.TemplatePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Stop stops the Lima VM
func (m *Manager) Stop() error {
	cmd := exec.Command("limactl", "stop", m.InstanceName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Status returns the current status of the Lima VM
func (m *Manager) Status() (VMStatus, error) {
	instance, err := m.getInstance()
	if err != nil {
		return StatusUnknown, err
	}

	switch instance.Status {
	case "Running":
		return StatusRunning, nil
	case "Stopped":
		return StatusStopped, nil
	default:
		return StatusUnknown, nil
	}
}

// getInstance gets the Lima instance info from limactl list --json
func (m *Manager) getInstance() (*LimaInstance, error) {
	cmd := exec.Command("limactl", "list", "--json")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("limactl list failed: %w", err)
	}

	var instances []LimaInstance
	if err := json.Unmarshal(stdout.Bytes(), &instances); err != nil {
		return nil, fmt.Errorf("parse limactl output: %w", err)
	}

	for _, inst := range instances {
		if inst.Name == m.InstanceName {
			return &inst, nil
		}
	}

	return nil, fmt.Errorf("instance '%s' not found", m.InstanceName)
}

// SocketPath returns the path to the forwarded daemon socket on the host.
// The socket is forwarded from /var/run/agentscale.sock inside the VM
// to ~/.lima/{instance}/sock/agentscale.sock on the host.
func (m *Manager) SocketPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".lima", m.InstanceName, "sock", "agentscale.sock")
}

// InstanceDir returns the Lima instance directory
func (m *Manager) InstanceDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".lima", m.InstanceName)
}

// Shell executes a command inside the Lima VM
func (m *Manager) Shell(command string) error {
	args := []string{"shell", m.InstanceName, "--"}
	args = append(args, strings.Split(command, " ")...)

	cmd := exec.Command("limactl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// ShellOutput executes a command inside the Lima VM and returns the output
func (m *Manager) ShellOutput(command string) (string, error) {
	args := []string{"shell", m.InstanceName, "--"}
	args = append(args, strings.Split(command, " ")...)

	cmd := exec.Command("limactl", args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", err
	}

	return strings.TrimSpace(stdout.String()), nil
}

// Delete deletes the Lima VM instance
func (m *Manager) Delete(force bool) error {
	args := []string{"delete", m.InstanceName}
	if force {
		args = append(args, "--force")
	}

	cmd := exec.Command("limactl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Info returns detailed information about the Lima instance
func (m *Manager) Info() (*LimaInstance, error) {
	return m.getInstance()
}

// IsRunning returns true if the VM is running
func (m *Manager) IsRunning() bool {
	status, err := m.Status()
	return err == nil && status == StatusRunning
}
