package service

import (
	"os/exec"
	"syscall"
)

func getExitCode(err error) int {
	if err == nil {
		return 0
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus()
		}
		return exitErr.ExitCode()
	}

	return -1
}

func isOOMKill(exitCode int) bool {
	return exitCode == 137
}

func isManualTermination(exitCode int) bool {
	return exitCode == 143
}

func isCleanExit(exitCode int) bool {
	return exitCode == 0
}
