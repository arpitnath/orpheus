// Package cgroups handles resource limits using cgroups v2
package cgroups

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

const (
	cgroupBasePath = "/sys/fs/cgroup"
)

// Config holds cgroup resource limits
type Config struct {
	MemoryLimitMB int // Memory limit in MB (0 = no limit)
	CPUPercent    int // CPU limit as percentage (100 = 1 core, 200 = 2 cores)
	MaxPIDs       int // Maximum number of processes (0 = no limit)
}

// CGroup represents a cgroup for a container
type CGroup struct {
	name string
	path string
}

// New creates a new cgroup with the given name
func New(name string) (*CGroup, error) {
	path := filepath.Join(cgroupBasePath, "agentscale", name)

	// Create the cgroup directory
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cgroup directory: %w", err)
	}

	return &CGroup{
		name: name,
		path: path,
	}, nil
}

// Apply applies resource limits to the cgroup
func (c *CGroup) Apply(config *Config) error {
	// Enable controllers
	if err := c.enableControllers(); err != nil {
		// Non-fatal, controllers might already be enabled
		fmt.Printf("[cgroups] Warning: failed to enable controllers: %v\n", err)
	}

	// Apply memory limit
	if config.MemoryLimitMB > 0 {
		if err := c.setMemoryLimit(config.MemoryLimitMB); err != nil {
			return fmt.Errorf("failed to set memory limit: %w", err)
		}
	}

	// Apply CPU limit
	if config.CPUPercent > 0 && config.CPUPercent < 100000 {
		if err := c.setCPULimit(config.CPUPercent); err != nil {
			return fmt.Errorf("failed to set CPU limit: %w", err)
		}
	}

	// Apply PID limit
	if config.MaxPIDs > 0 {
		if err := c.setPIDLimit(config.MaxPIDs); err != nil {
			return fmt.Errorf("failed to set PID limit: %w", err)
		}
	}

	return nil
}

// AddProcess adds a process to this cgroup
func (c *CGroup) AddProcess(pid int) error {
	procsFile := filepath.Join(c.path, "cgroup.procs")
	return os.WriteFile(procsFile, []byte(strconv.Itoa(pid)), 0644)
}

// AddSelf adds the current process to this cgroup
func (c *CGroup) AddSelf() error {
	return c.AddProcess(os.Getpid())
}

// Remove removes the cgroup
func (c *CGroup) Remove() error {
	// First, move all processes out (to parent)
	// Then remove the directory
	return os.RemoveAll(c.path)
}

// Path returns the cgroup path
func (c *CGroup) Path() string {
	return c.path
}

// enableControllers enables memory, cpu, and pids controllers
func (c *CGroup) enableControllers() error {
	// For cgroups v2, we need to enable controllers in the parent
	parentPath := filepath.Dir(c.path)
	subtreeControl := filepath.Join(parentPath, "cgroup.subtree_control")

	// Check if the file exists
	if _, err := os.Stat(subtreeControl); os.IsNotExist(err) {
		// Create parent cgroup if needed
		if err := os.MkdirAll(parentPath, 0755); err != nil {
			return err
		}
	}

	// Enable controllers: +memory +cpu +pids
	controllers := "+memory +cpu +pids"
	if err := os.WriteFile(subtreeControl, []byte(controllers), 0644); err != nil {
		return fmt.Errorf("failed to enable controllers: %w", err)
	}

	return nil
}

// setMemoryLimit sets the memory limit in bytes
func (c *CGroup) setMemoryLimit(limitMB int) error {
	limitBytes := int64(limitMB) * 1024 * 1024

	// memory.max - hard limit
	maxFile := filepath.Join(c.path, "memory.max")
	if err := os.WriteFile(maxFile, []byte(strconv.FormatInt(limitBytes, 10)), 0644); err != nil {
		return err
	}

	// memory.high - soft limit (trigger throttling before OOM)
	// Set to 90% of max
	highBytes := limitBytes * 90 / 100
	highFile := filepath.Join(c.path, "memory.high")
	if err := os.WriteFile(highFile, []byte(strconv.FormatInt(highBytes, 10)), 0644); err != nil {
		// Non-fatal
		fmt.Printf("[cgroups] Warning: failed to set memory.high: %v\n", err)
	}

	return nil
}

// setCPULimit sets the CPU limit as a percentage
// 100 = 1 core, 50 = half a core, 200 = 2 cores
func (c *CGroup) setCPULimit(percent int) error {
	// cpu.max format: "$MAX $PERIOD"
	// MAX is the maximum time in microseconds that the cgroup can use per PERIOD
	// PERIOD is typically 100000 (100ms)

	period := 100000 // 100ms in microseconds
	quota := (percent * period) / 100

	cpuMax := fmt.Sprintf("%d %d", quota, period)
	maxFile := filepath.Join(c.path, "cpu.max")

	return os.WriteFile(maxFile, []byte(cpuMax), 0644)
}

// setPIDLimit sets the maximum number of processes
func (c *CGroup) setPIDLimit(max int) error {
	maxFile := filepath.Join(c.path, "pids.max")
	return os.WriteFile(maxFile, []byte(strconv.Itoa(max)), 0644)
}

// IsCgroupV2 checks if the system uses cgroups v2
func IsCgroupV2() bool {
	// Check for cgroups v2 unified hierarchy
	_, err := os.Stat(filepath.Join(cgroupBasePath, "cgroup.controllers"))
	return err == nil
}

// GetMemoryUsage returns current memory usage in bytes
func (c *CGroup) GetMemoryUsage() (int64, error) {
	currentFile := filepath.Join(c.path, "memory.current")
	data, err := os.ReadFile(currentFile)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(string(data[:len(data)-1]), 10, 64) // Remove newline
}

// GetPIDCount returns current number of processes
func (c *CGroup) GetPIDCount() (int, error) {
	currentFile := filepath.Join(c.path, "pids.current")
	data, err := os.ReadFile(currentFile)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(string(data[:len(data)-1])) // Remove newline
}
