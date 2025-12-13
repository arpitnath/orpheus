#!/bin/bash
# AgentScale Isolation - EC2 Test Script
# Run: curl -sL <url> | bash
# Or copy-paste this entire script into EC2

set -e

echo "=== AgentScale Isolation Test Setup ==="

# Install Go if not present
if ! command -v go &> /dev/null; then
    echo "[1/4] Installing Go..."
    sudo snap install go --classic
else
    echo "[1/4] Go already installed"
fi

echo "[2/4] Creating project..."
mkdir -p /tmp/isolation-test
cd /tmp/isolation-test

# Create go.mod
cat > go.mod << 'GOMOD'
module github.com/agentscale/isolation

go 1.21
GOMOD

# Create directory structure
mkdir -p cmd/isolate pkg/cgroups

# Create cgroups.go
cat > pkg/cgroups/cgroups.go << 'CGROUPS'
package cgroups

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

const cgroupBasePath = "/sys/fs/cgroup"

type Config struct {
	MemoryLimitMB int
	CPUPercent    int
	MaxPIDs       int
}

type CGroup struct {
	name string
	path string
}

func New(name string) (*CGroup, error) {
	path := filepath.Join(cgroupBasePath, "agentscale", name)
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cgroup directory: %w", err)
	}
	return &CGroup{name: name, path: path}, nil
}

func (c *CGroup) Apply(config *Config) error {
	if err := c.enableControllers(); err != nil {
		fmt.Printf("[cgroups] Warning: failed to enable controllers: %v\n", err)
	}
	if config.MemoryLimitMB > 0 {
		if err := c.setMemoryLimit(config.MemoryLimitMB); err != nil {
			return fmt.Errorf("failed to set memory limit: %w", err)
		}
	}
	if config.CPUPercent > 0 && config.CPUPercent < 100000 {
		if err := c.setCPULimit(config.CPUPercent); err != nil {
			return fmt.Errorf("failed to set CPU limit: %w", err)
		}
	}
	if config.MaxPIDs > 0 {
		if err := c.setPIDLimit(config.MaxPIDs); err != nil {
			return fmt.Errorf("failed to set PID limit: %w", err)
		}
	}
	return nil
}

func (c *CGroup) AddProcess(pid int) error {
	procsFile := filepath.Join(c.path, "cgroup.procs")
	return os.WriteFile(procsFile, []byte(strconv.Itoa(pid)), 0644)
}

func (c *CGroup) Remove() error {
	return os.RemoveAll(c.path)
}

func (c *CGroup) enableControllers() error {
	parentPath := filepath.Dir(c.path)
	subtreeControl := filepath.Join(parentPath, "cgroup.subtree_control")
	if _, err := os.Stat(subtreeControl); os.IsNotExist(err) {
		if err := os.MkdirAll(parentPath, 0755); err != nil {
			return err
		}
	}
	return os.WriteFile(subtreeControl, []byte("+memory +cpu +pids"), 0644)
}

func (c *CGroup) setMemoryLimit(limitMB int) error {
	limitBytes := int64(limitMB) * 1024 * 1024
	maxFile := filepath.Join(c.path, "memory.max")
	if err := os.WriteFile(maxFile, []byte(strconv.FormatInt(limitBytes, 10)), 0644); err != nil {
		return err
	}
	highBytes := limitBytes * 90 / 100
	highFile := filepath.Join(c.path, "memory.high")
	os.WriteFile(highFile, []byte(strconv.FormatInt(highBytes, 10)), 0644)
	return nil
}

func (c *CGroup) setCPULimit(percent int) error {
	period := 100000
	quota := (percent * period) / 100
	cpuMax := fmt.Sprintf("%d %d", quota, period)
	maxFile := filepath.Join(c.path, "cpu.max")
	return os.WriteFile(maxFile, []byte(cpuMax), 0644)
}

func (c *CGroup) setPIDLimit(max int) error {
	maxFile := filepath.Join(c.path, "pids.max")
	return os.WriteFile(maxFile, []byte(strconv.Itoa(max)), 0644)
}

func IsCgroupV2() bool {
	_, err := os.Stat(filepath.Join(cgroupBasePath, "cgroup.controllers"))
	return err == nil
}
CGROUPS

# Create main.go
cat > cmd/isolate/main.go << 'MAIN'
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	MemoryMB   int
	CPUPercent int
	MaxPIDs    int
	RootFS     string
	Command    string
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		config := parseArgs(os.Args[2:])
		runContainer(config)
	case "child":
		runInsideNamespace()
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("isolate - Lightweight container runtime")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  isolate run [options] <command>    Run a command in isolation")
	fmt.Println("  isolate help                       Show this help")
	fmt.Println("")
	fmt.Println("Run Options:")
	fmt.Println("  --memory=<MB>     Memory limit in MB (default: 512)")
	fmt.Println("  --cpu=<percent>   CPU limit as percentage (default: 100)")
	fmt.Println("  --pids=<max>      Max processes (default: 100)")
	fmt.Println("  --rootfs=<path>   Path to root filesystem")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  isolate run \"echo hello\"")
	fmt.Println("  isolate run --memory=256 --cpu=50 \"python script.py\"")
}

func parseArgs(args []string) *Config {
	config := &Config{
		MemoryMB:   512,
		CPUPercent: 100,
		MaxPIDs:    100,
	}

	for _, arg := range args {
		if strings.HasPrefix(arg, "--memory=") {
			val := strings.TrimPrefix(arg, "--memory=")
			config.MemoryMB, _ = strconv.Atoi(val)
		} else if strings.HasPrefix(arg, "--cpu=") {
			val := strings.TrimPrefix(arg, "--cpu=")
			config.CPUPercent, _ = strconv.Atoi(val)
		} else if strings.HasPrefix(arg, "--pids=") {
			val := strings.TrimPrefix(arg, "--pids=")
			config.MaxPIDs, _ = strconv.Atoi(val)
		} else if strings.HasPrefix(arg, "--rootfs=") {
			config.RootFS = strings.TrimPrefix(arg, "--rootfs=")
		} else if !strings.HasPrefix(arg, "-") {
			config.Command = arg
		}
	}

	return config
}

func runContainer(config *Config) {
	fmt.Println("[isolate] Starting container...")
	fmt.Printf("[isolate] Config: memory=%dMB, cpu=%d%%, pids=%d\n",
		config.MemoryMB, config.CPUPercent, config.MaxPIDs)
	fmt.Printf("[isolate] Command: %s\n", config.Command)

	if err := createNamespacedProcess(config); err != nil {
		fmt.Fprintf(os.Stderr, "[isolate] Container failed: %v\n", err)
		os.Exit(1)
	}
}
MAIN

# Create container_linux.go
cat > cmd/isolate/container_linux.go << 'CONTAINER'
//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/agentscale/isolation/pkg/cgroups"
)

var containerCgroup *cgroups.CGroup

func createNamespacedProcess(config *Config) error {
	containerID := fmt.Sprintf("isolate-%d", os.Getpid())

	if cgroups.IsCgroupV2() {
		cg, err := cgroups.New(containerID)
		if err != nil {
			fmt.Printf("[isolate] Warning: failed to create cgroup: %v\n", err)
		} else {
			cgConfig := &cgroups.Config{
				MemoryLimitMB: config.MemoryMB,
				CPUPercent:    config.CPUPercent,
				MaxPIDs:       config.MaxPIDs,
			}
			if err := cg.Apply(cgConfig); err != nil {
				fmt.Printf("[isolate] Warning: failed to apply cgroup limits: %v\n", err)
			}
			containerCgroup = cg
		}
	} else {
		fmt.Println("[isolate] Warning: cgroups v2 not available")
	}

	cmd := exec.Command("/proc/self/exe", "child", config.Command,
		fmt.Sprintf("%d", config.MemoryMB),
		fmt.Sprintf("%d", config.CPUPercent),
		fmt.Sprintf("%d", config.MaxPIDs),
		config.RootFS,
	)

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNS |
			syscall.CLONE_NEWIPC,
		Unshareflags: syscall.CLONE_NEWNS,
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		cleanup()
		return err
	}

	if containerCgroup != nil {
		if err := containerCgroup.AddProcess(cmd.Process.Pid); err != nil {
			fmt.Printf("[isolate] Warning: failed to add process to cgroup: %v\n", err)
		}
	}

	err := cmd.Wait()
	cleanup()
	return err
}

func cleanup() {
	if containerCgroup != nil {
		containerCgroup.Remove()
		containerCgroup = nil
	}
}

func runInsideNamespace() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "[child] Error: missing command")
		os.Exit(1)
	}

	command := os.Args[2]
	memoryMB, _ := strconv.Atoi(os.Args[3])
	cpuPercent, _ := strconv.Atoi(os.Args[4])
	maxPIDs, _ := strconv.Atoi(os.Args[5])
	rootfs := ""
	if len(os.Args) >= 7 {
		rootfs = os.Args[6]
	}

	fmt.Printf("[container] PID: %d (namespace PID 1)\n", os.Getpid())
	fmt.Printf("[container] Limits: memory=%dMB, cpu=%d%%, pids=%d\n", memoryMB, cpuPercent, maxPIDs)

	syscall.Sethostname([]byte("container"))

	if rootfs != "" {
		fmt.Printf("[container] Setting up rootfs: %s\n", rootfs)
		if err := setupPivotRoot(rootfs); err != nil {
			fmt.Fprintf(os.Stderr, "[container] Failed to setup rootfs: %v\n", err)
			os.Exit(1)
		}
	} else {
		syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, "")
		syscall.Mount("proc", "/proc", "proc", 0, "")
	}

	fmt.Printf("[container] Executing: %s\n", command)

	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "[container] Command failed: %v\n", err)
		os.Exit(1)
	}
}

func setupPivotRoot(rootfs string) error {
	if err := syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("failed to make root private: %w", err)
	}
	if err := syscall.Mount(rootfs, rootfs, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("failed to bind mount rootfs: %w", err)
	}
	oldRoot := rootfs + "/.old_root"
	if err := os.MkdirAll(oldRoot, 0700); err != nil {
		return fmt.Errorf("failed to create old_root: %w", err)
	}
	if err := syscall.PivotRoot(rootfs, oldRoot); err != nil {
		return fmt.Errorf("pivot_root failed: %w", err)
	}
	if err := os.Chdir("/"); err != nil {
		return fmt.Errorf("failed to chdir: %w", err)
	}
	os.MkdirAll("/proc", 0555)
	syscall.Mount("proc", "/proc", "proc", 0, "")
	syscall.Unmount("/.old_root", syscall.MNT_DETACH)
	os.RemoveAll("/.old_root")
	return nil
}
CONTAINER

echo "[3/4] Building..."
go build -o isolate ./cmd/isolate/

echo "[4/4] Running tests..."
echo ""
echo "=== Test 1: Basic Execution ==="
sudo ./isolate run "echo hello from container"

echo ""
echo "=== Test 2: PID Namespace ==="
sudo ./isolate run "echo PID is \$\$"

echo ""
echo "=== Test 3: Hostname Isolation ==="
sudo ./isolate run "hostname"

echo ""
echo "=== Test 4: Process List ==="
sudo ./isolate run "ps aux | head -5"

echo ""
echo "=== All Tests Complete ==="
echo "Binary location: /tmp/isolation-test/isolate"
