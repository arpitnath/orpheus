package lima

import (
	"os/exec"
	"runtime"
)

// Available returns true if Lima (limactl) is installed and available in PATH.
// Lima is only supported on macOS.
func Available() bool {
	// Lima only works on macOS
	if runtime.GOOS != "darwin" {
		return false
	}

	_, err := exec.LookPath("limactl")
	return err == nil
}

// Running returns true if the AgentScale Lima VM is currently running.
func Running() bool {
	if !Available() {
		return false
	}

	mgr := NewManager()
	return mgr.IsRunning()
}

// SocketExists returns true if the daemon socket exists on the host.
// This indicates the daemon is running inside the VM and the socket is forwarded.
func SocketExists() bool {
	mgr := NewManager()
	socketPath := mgr.SocketPath()

	// Check if socket file exists
	_, err := exec.Command("test", "-S", socketPath).Output()
	return err == nil
}
