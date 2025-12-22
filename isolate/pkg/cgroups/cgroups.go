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

// Config holds cgroup resource limits (Agent-Native: Graceful Degradation)
type Config struct {
	// Memory configuration (Agent-Native)
	MemoryTargetMB int  // Soft limit in MB - fast tier (memory.high)
	MemoryLimitMB  int  // Hard limit in MB - with swap (memory.max)
	SwapEnabled    bool // Enable swap for graceful degradation

	// Other limits
	CPUPercent int // CPU limit as percentage (100 = 1 core, 200 = 2 cores)
	MaxPIDs    int // Maximum number of processes (0 = no limit)
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

	// Apply memory limits (Agent-Native: Graceful Degradation)
	if config.MemoryLimitMB > 0 {
		targetMB := config.MemoryTargetMB
		if targetMB == 0 {
			// Default target to limit if not specified (backward compatibility)
			targetMB = config.MemoryLimitMB
		}
		if err := c.setMemoryLimits(targetMB, config.MemoryLimitMB, config.SwapEnabled); err != nil {
			return fmt.Errorf("failed to set memory limits: %w", err)
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

// setMemoryLimits sets memory limits for Agent-Native graceful degradation
// targetMB: Soft limit (memory.high) - fast performance tier
// limitMB: Hard limit (memory.max) - maximum with swap
// swapEnabled: Allow swap for graceful degradation between target and limit
func (c *CGroup) setMemoryLimits(targetMB, limitMB int, swapEnabled bool) error {
	// Convert to bytes
	targetBytes := int64(targetMB) * 1024 * 1024
	limitBytes := int64(limitMB) * 1024 * 1024

	// memory.max - hard limit (OOM kill if exceeded)
	maxFile := filepath.Join(c.path, "memory.max")
	if err := os.WriteFile(maxFile, []byte(strconv.FormatInt(limitBytes, 10)), 0644); err != nil {
		return fmt.Errorf("failed to set memory.max: %w", err)
	}

	// memory.high - soft limit (trigger throttling/reclaim, performance degrades)
	// This is the "target" tier - agent runs fast below this
	highFile := filepath.Join(c.path, "memory.high")
	if err := os.WriteFile(highFile, []byte(strconv.FormatInt(targetBytes, 10)), 0644); err != nil {
		// Non-fatal - continue with hard limit only
		fmt.Printf("[cgroups] Warning: failed to set memory.high (soft limit): %v\n", err)
	}

	// memory.swap.max - swap limit for graceful degradation
	// Allow swap equal to the difference between limit and target
	if swapEnabled && limitMB > targetMB {
		swapBytes := int64(limitMB-targetMB) * 1024 * 1024
		swapFile := filepath.Join(c.path, "memory.swap.max")
		if err := os.WriteFile(swapFile, []byte(strconv.FormatInt(swapBytes, 10)), 0644); err != nil {
			// Swap may not be available on all systems - non-fatal
			fmt.Printf("[cgroups] Warning: swap not available (graceful degradation limited): %v\n", err)
		} else {
			fmt.Printf("[cgroups] Agent-Native memory: target=%dMB (fast), limit=%dMB (with %dMB swap)\n",
				targetMB, limitMB, limitMB-targetMB)
		}
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
