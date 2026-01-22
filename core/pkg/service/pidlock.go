package service

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

func acquirePIDLock(lockPath string) (func(), error) {
	if err := checkStaleLock(lockPath); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("lock held by another instance")
	}

	if err := f.Truncate(0); err != nil {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
		return nil, err
	}

	if _, err := fmt.Fprintf(f, "%d\n", os.Getpid()); err != nil {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
		return nil, err
	}

	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
		os.Remove(lockPath)
	}, nil
}

func checkStaleLock(lockPath string) error {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return nil
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		os.Remove(lockPath)
		return nil
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		os.Remove(lockPath)
		return nil
	}

	if err := process.Signal(syscall.Signal(0)); err != nil {
		os.Remove(lockPath)
		return nil
	}

	cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return nil
	}

	if !bytes.Contains(cmdline, []byte("orpheusd")) {
		os.Remove(lockPath)
		return nil
	}

	return fmt.Errorf("valid lock held by orpheusd PID %d", pid)
}
