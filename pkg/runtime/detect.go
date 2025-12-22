package runtime

import "os/exec"

// RuncAvailable returns true if runc binary is installed and in PATH
func RuncAvailable() bool {
	runc := NewRunc()
	return runc.Available()
}

// DockerAvailable returns true if Docker daemon is running.
// Used for macOS fallback when runc is not available.
func DockerAvailable() bool {
	// First check if docker binary exists
	_, err := exec.LookPath("docker")
	if err != nil {
		return false
	}

	// Then verify daemon is running
	cmd := exec.Command("docker", "info")
	return cmd.Run() == nil
}
