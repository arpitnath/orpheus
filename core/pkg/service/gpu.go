package service

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// hasNVIDIAGPU checks if NVIDIA GPU is available on the system
func hasNVIDIAGPU() bool {
	if _, err := os.Stat("/dev/nvidia0"); err == nil {
		return true
	}

	cmd := exec.Command("nvidia-smi", "--list-gpus")
	if err := cmd.Run(); err == nil {
		return true
	}

	return false
}

// getGPUMemory returns total GPU memory in GB for the first GPU
func getGPUMemory() (int, error) {
	cmd := exec.Command("nvidia-smi",
		"--query-gpu=memory.total",
		"--format=csv,noheader,nounits")

	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	memMB, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0, err
	}

	return memMB / 1024, nil
}

// getGPUCount returns the number of NVIDIA GPUs available
func getGPUCount() int {
	cmd := exec.Command("nvidia-smi", "--query-gpu=count", "--format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	count, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0
	}

	return count
}
