package service

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
)

func killProcessGracefully(process *os.Process, timeout time.Duration) error {
	if process == nil {
		return nil
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() {
		_, err := process.Wait()
		done <- err
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return process.Kill()
	}
}

func isPortOpen(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func waitForPortFree(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if !isPortOpen(port) {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}

	return false
}

// restartWithDoubleKill will be integrated in manager.go (Task 7)
